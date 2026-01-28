package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/repository"
	"bulk-import-export/internal/validator"
)

const (
	// DefaultImportTimeout is the timeout for import processing
	DefaultImportTimeout = 30 * time.Minute

	// ScannerBufferSize is the initial buffer size for NDJSON scanner
	ScannerBufferSize = 64 * 1024 // 64KB
	// ScannerMaxBufferSize is the maximum buffer size for NDJSON scanner
	ScannerMaxBufferSize = 1024 * 1024 // 1MB

	// QueueSendTimeout is the timeout for sending tasks to the queue
	QueueSendTimeout = 5 * time.Second
)

// ImportService handles bulk import operations.
type ImportService struct {
	userRepo    repository.UserRepository
	articleRepo repository.ArticleRepository
	commentRepo repository.CommentRepository
	jobRepo     repository.JobRepository
	validator   *validator.Validator

	batchSize   int
	workerCount int
	timeout     time.Duration

	jobQueue chan importTask
	stopChan chan struct{}
	wg       sync.WaitGroup
}

type importTask struct {
	job       *domain.ImportJob
	data      []byte // Buffered file content
	requestID string // Request ID for tracing
}

// NewImportService creates a new ImportService with worker pool.
func NewImportService(
	userRepo repository.UserRepository,
	articleRepo repository.ArticleRepository,
	commentRepo repository.CommentRepository,
	jobRepo repository.JobRepository,
	v *validator.Validator,
	batchSize int,
	workerCount int,
) *ImportService {
	s := &ImportService{
		userRepo:    userRepo,
		articleRepo: articleRepo,
		commentRepo: commentRepo,
		jobRepo:     jobRepo,
		validator:   v,
		batchSize:   batchSize,
		workerCount: workerCount,
		timeout:     DefaultImportTimeout,
		jobQueue:    make(chan importTask, workerCount*2),
		stopChan:    make(chan struct{}),
	}

	for i := 0; i < workerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}

	return s
}

func (s *ImportService) worker() {
	defer s.wg.Done()

	for {
		select {
		case task, ok := <-s.jobQueue:
			if !ok {
				return
			}
			s.processImport(task)
		case <-s.stopChan:
			return
		}
	}
}

// Close shuts down the worker pool immediately.
func (s *ImportService) Close() {
	close(s.stopChan)
	close(s.jobQueue)
	s.wg.Wait()
}

// StartImport creates an import job and queues it for processing.
func (s *ImportService) StartImport(ctx context.Context, resourceType, idempotencyToken, filename, requestID string, reader io.Reader) (*domain.ImportJob, error) {
	log.Printf("[request_id=%s] Starting import for resource_type=%s", requestID, resourceType)

	existingJob, err := s.jobRepo.GetImportJobByIdempotencyToken(ctx, idempotencyToken)
	if err != nil {
		return nil, fmt.Errorf("check idempotency token: %w", err)
	}
	if existingJob != nil {
		log.Printf("[request_id=%s] Returning existing job %s for idempotency token", requestID, existingJob.ID)
		return existingJob, nil
	}

	// Buffer the file content before returning - the reader will be closed after this function returns
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read file content: %w", err)
	}

	now := time.Now()
	job := &domain.ImportJob{
		ID:               uuid.New().String(),
		ResourceType:     resourceType,
		Status:           domain.JobStatusPending,
		TotalRecords:     0,
		ProcessedRecords: 0,
		SuccessCount:     0,
		FailureCount:     0,
		IdempotencyToken: idempotencyToken,
		Metadata: map[string]interface{}{
			"filename": filename,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.jobRepo.CreateImportJob(ctx, job); err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}

	// Send to queue with timeout to prevent blocking
	select {
	case s.jobQueue <- importTask{
		job:       job,
		data:      data,
		requestID: requestID,
	}:
		log.Printf("[request_id=%s] Job %s queued for processing", requestID, job.ID)
	case <-time.After(QueueSendTimeout):
		log.Printf("[request_id=%s] Warning: queue full, job %s will be processed when capacity available", requestID, job.ID)
		// Still try to send, but don't block indefinitely
		go func() {
			s.jobQueue <- importTask{
				job:       job,
				data:      data,
				requestID: requestID,
			}
		}()
	}

	return job, nil
}

func (s *ImportService) processImport(task importTask) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	job := task.job
	requestID := task.requestID

	log.Printf("[request_id=%s] Processing import job %s", requestID, job.ID)

	job.Status = domain.JobStatusProcessing
	job.UpdatedAt = time.Now()
	if err := s.jobRepo.UpdateImportJob(ctx, job); err != nil {
		log.Printf("[request_id=%s] Failed to update import job status to processing: %v", requestID, err)
	}

	// Create a reader from the buffered data
	reader := bytes.NewReader(task.data)

	var result domain.ImportResult

	switch job.ResourceType {
	case "users":
		result = s.importUsers(ctx, job, reader)
	case "articles":
		result = s.importArticles(ctx, job, reader)
	case "comments":
		result = s.importComments(ctx, job, reader)
	default:
		job.Status = domain.JobStatusFailed
		errMsg := fmt.Sprintf("unsupported resource type: %s", job.ResourceType)
		job.ErrorMessage = &errMsg
		job.UpdatedAt = time.Now()
		if err := s.jobRepo.UpdateImportJob(ctx, job); err != nil {
			log.Printf("[request_id=%s] Failed to update import job: %v", requestID, err)
		}
		return
	}

	now := time.Now()
	job.TotalRecords = result.TotalRecords
	job.ProcessedRecords = result.ProcessedRecords
	job.SuccessCount = result.SuccessCount
	job.FailureCount = result.FailureCount
	job.UpdatedAt = now
	job.CompletedAt = &now

	if result.FailureCount == result.TotalRecords && result.TotalRecords > 0 {
		job.Status = domain.JobStatusFailed
	} else if result.FailureCount > 0 {
		job.Status = domain.JobStatusCompletedWithErrors
	} else {
		job.Status = domain.JobStatusCompleted
	}

	if err := s.jobRepo.UpdateImportJob(ctx, job); err != nil {
		log.Printf("[request_id=%s] Failed to update import job: %v", requestID, err)
	}

	log.Printf("[request_id=%s] Import job %s completed: status=%s, total=%d, success=%d, failed=%d",
		requestID, job.ID, job.Status, job.TotalRecords, job.SuccessCount, job.FailureCount)
}

func (s *ImportService) importUsers(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{}

	csvReader := csv.NewReader(reader)

	header, err := csvReader.Read()
	if err != nil {
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "header",
			Reason: fmt.Sprintf("failed to read CSV header: %v", err),
		})
		return result
	}

	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Validate required columns exist
	requiredColumns := []string{"email", "name"}
	var missingColumns []string
	for _, col := range requiredColumns {
		if _, ok := colMap[col]; !ok {
			missingColumns = append(missingColumns, col)
		}
	}
	if len(missingColumns) > 0 {
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "header",
			Reason: fmt.Sprintf("missing required columns: %s", strings.Join(missingColumns, ", ")),
		})
		return result
	}

	var batch []userBatchItem
	rowNum := 1 // Start at 1 because row 0 is the header

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, domain.RecordError{
				Row:    rowNum,
				Field:  "csv",
				Reason: err.Error(),
			})
			rowNum++
			continue
		}

		rowNum++
		result.TotalRecords++

		user := domain.User{
			ID:     uuid.New().String(),
			Active: true,
		}

		if idx, ok := colMap["email"]; ok && idx < len(record) {
			user.Email = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["name"]; ok && idx < len(record) {
			user.Name = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["role"]; ok && idx < len(record) {
			user.Role = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["is_active"]; ok && idx < len(record) {
			val := strings.ToLower(strings.TrimSpace(record[idx]))
			user.Active = val == "true" || val == "1" || val == "yes"
		}

		if err := s.validator.ValidateUser(&user); err != nil {
			errs := validator.ConvertValidationErrors(rowNum, err)
			result.Errors = append(result.Errors, errs...)
			result.FailureCount++
			continue
		}

		batch = append(batch, userBatchItem{user: user, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			s.processBatchUsers(ctx, job, batch, &result)
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		s.processBatchUsers(ctx, job, batch, &result)
	}

	return result
}

type userBatchItem struct {
	user   domain.User
	rowNum int
}

// processBatchUsers handles a batch of users with proper row number mapping
func (s *ImportService) processBatchUsers(ctx context.Context, job *domain.ImportJob, batch []userBatchItem, result *domain.ImportResult) {
	// Create row number mapping: batch index -> absolute row number
	rowMapping := make(map[int]int)
	users := make([]domain.User, len(batch))
	for i, item := range batch {
		users[i] = item.user
		rowMapping[i+1] = item.rowNum // BulkInsert uses 1-based row numbers
	}

	batchResult := s.userRepo.BulkInsert(ctx, users)
	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	// Map batch row numbers to absolute row numbers
	for i := range batchResult.Errors {
		if absRow, ok := rowMapping[batchResult.Errors[i].Row]; ok {
			batchResult.Errors[i].Row = absRow
		}
	}
	result.Errors = append(result.Errors, batchResult.Errors...)
}

func (s *ImportService) importArticles(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{}

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, ScannerBufferSize)
	scanner.Buffer(buf, ScannerMaxBufferSize)

	var batch []articleBatchItem
	rowNum := 0

	for scanner.Scan() {
		rowNum++

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			// Blank lines don't count as records
			continue
		}

		result.TotalRecords++

		var article domain.Article
		if err := json.Unmarshal([]byte(line), &article); err != nil {
			result.Errors = append(result.Errors, domain.RecordError{
				Row:    rowNum,
				Field:  "record",
				Reason: fmt.Sprintf("invalid JSON: %v", err),
			})
			result.FailureCount++
			continue
		}

		if article.ID == "" {
			article.ID = uuid.New().String()
		}

		if err := s.validator.ValidateArticle(&article); err != nil {
			errs := validator.ConvertValidationErrors(rowNum, err)
			result.Errors = append(result.Errors, errs...)
			result.FailureCount++
			continue
		}

		batch = append(batch, articleBatchItem{article: article, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			s.processBatchArticles(ctx, job, batch, &result)
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: err.Error(),
		})
	}

	if len(batch) > 0 {
		s.processBatchArticles(ctx, job, batch, &result)
	}

	return result
}

type articleBatchItem struct {
	article domain.Article
	rowNum  int
}

// processBatchArticles handles a batch of articles with proper row number mapping
func (s *ImportService) processBatchArticles(ctx context.Context, job *domain.ImportJob, batch []articleBatchItem, result *domain.ImportResult) {
	rowMapping := make(map[int]int)
	articles := make([]domain.Article, len(batch))
	for i, item := range batch {
		articles[i] = item.article
		rowMapping[i+1] = item.rowNum
	}

	batchResult := s.articleRepo.BulkInsert(ctx, articles)
	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	for i := range batchResult.Errors {
		if absRow, ok := rowMapping[batchResult.Errors[i].Row]; ok {
			batchResult.Errors[i].Row = absRow
		}
	}
	result.Errors = append(result.Errors, batchResult.Errors...)
}

func (s *ImportService) importComments(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{}

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, ScannerBufferSize)
	scanner.Buffer(buf, ScannerMaxBufferSize)

	var batch []commentBatchItem
	rowNum := 0

	for scanner.Scan() {
		rowNum++

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			// Blank lines don't count as records
			continue
		}

		result.TotalRecords++

		var comment domain.Comment
		if err := json.Unmarshal([]byte(line), &comment); err != nil {
			result.Errors = append(result.Errors, domain.RecordError{
				Row:    rowNum,
				Field:  "record",
				Reason: fmt.Sprintf("invalid JSON: %v", err),
			})
			result.FailureCount++
			continue
		}

		if comment.ID == "" {
			comment.ID = uuid.New().String()
		}

		if err := s.validator.ValidateComment(&comment); err != nil {
			errs := validator.ConvertValidationErrors(rowNum, err)
			result.Errors = append(result.Errors, errs...)
			result.FailureCount++
			continue
		}

		batch = append(batch, commentBatchItem{comment: comment, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			s.processBatchComments(ctx, job, batch, &result)
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: err.Error(),
		})
	}

	if len(batch) > 0 {
		s.processBatchComments(ctx, job, batch, &result)
	}

	return result
}

type commentBatchItem struct {
	comment domain.Comment
	rowNum  int
}

// processBatchComments handles a batch of comments with proper row number mapping
func (s *ImportService) processBatchComments(ctx context.Context, job *domain.ImportJob, batch []commentBatchItem, result *domain.ImportResult) {
	rowMapping := make(map[int]int)
	comments := make([]domain.Comment, len(batch))
	for i, item := range batch {
		comments[i] = item.comment
		rowMapping[i+1] = item.rowNum
	}

	batchResult := s.commentRepo.BulkInsert(ctx, comments)
	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	for i := range batchResult.Errors {
		if absRow, ok := rowMapping[batchResult.Errors[i].Row]; ok {
			batchResult.Errors[i].Row = absRow
		}
	}
	result.Errors = append(result.Errors, batchResult.Errors...)
}

// GetImportJob retrieves an import job by ID.
func (s *ImportService) GetImportJob(ctx context.Context, id string) (*domain.ImportJob, error) {
	return s.jobRepo.GetImportJob(ctx, id)
}

// ListImportJobs lists all import jobs.
func (s *ImportService) ListImportJobs(ctx context.Context) ([]domain.ImportJob, error) {
	return s.jobRepo.ListImportJobs(ctx)
}
