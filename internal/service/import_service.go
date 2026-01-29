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

	// ErrorSlicePreallocSize is the initial capacity for error slices
	// Pre-allocating avoids expensive slice reallocations during import
	ErrorSlicePreallocSize = 1000
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
	closed   bool
	mu       sync.RWMutex
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
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

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
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("import service is shutting down")
	}
	s.mu.RUnlock()

	select {
	case s.jobQueue <- importTask{
		job:       job,
		data:      data,
		requestID: requestID,
	}:
		log.Printf("[request_id=%s] Job %s queued for processing", requestID, job.ID)
	case <-time.After(QueueSendTimeout):
		log.Printf("[request_id=%s] Warning: queue full, job %s will be processed when capacity available", requestID, job.ID)
		// Try to send in a goroutine, but check if closed first
		go func() {
			s.mu.RLock()
			if s.closed {
				s.mu.RUnlock()
				return
			}
			s.mu.RUnlock()

			select {
			case s.jobQueue <- importTask{
				job:       job,
				data:      data,
				requestID: requestID,
			}:
			case <-s.stopChan:
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
	startTime := time.Now()

	log.Printf("[request_id=%s] Processing import job %s: resource_type=%s", requestID, job.ID, job.ResourceType)

	job.Status = domain.JobStatusProcessing
	job.UpdatedAt = time.Now()
	if err := s.jobRepo.UpdateImportJob(ctx, job); err != nil {
		log.Printf("[request_id=%s] Failed to update import job status to processing: %v", requestID, err)
	}

	// Create a reader from the buffered data
	reader := bytes.NewReader(task.data)
	log.Printf("[request_id=%s] Starting %s import processing, data size: %d bytes", requestID, job.ResourceType, len(task.data))

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

	elapsed := time.Since(startTime)
	log.Printf("[request_id=%s] Import job %s completed: status=%s, total=%d, success=%d, failed=%d, elapsed=%s",
		requestID, job.ID, job.Status, job.TotalRecords, job.SuccessCount, job.FailureCount, elapsed.Round(time.Millisecond))
}

func (s *ImportService) importUsers(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{
		Errors: make([]domain.RecordError, 0, ErrorSlicePreallocSize),
	}
	log.Printf("[job_id=%s] Starting CSV parsing for users", job.ID)

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
	log.Printf("[job_id=%s] CSV header parsed: %v", job.ID, header)

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
	parseStart := time.Now()
	var totalValidationTime time.Duration
	var totalDBTime time.Duration
	var lastProgressTime time.Time

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

		// Log progress every 1000 records
		if result.TotalRecords%1000 == 0 {
			now := time.Now()
			var sinceLast time.Duration
			if !lastProgressTime.IsZero() {
				sinceLast = now.Sub(lastProgressTime)
			}
			log.Printf("[job_id=%s] Parsing progress: %d records parsed in %v (last 1000 in %v, validation=%v, db=%v)",
				job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
				sinceLast.Round(time.Millisecond), totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))
			lastProgressTime = now
		}

		user := domain.User{
			Active: true,
		}

		if idx, ok := colMap["id"]; ok && idx < len(record) {
			user.ID = strings.TrimSpace(record[idx])
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
		if idx, ok := colMap["active"]; ok && idx < len(record) {
			val := strings.ToLower(strings.TrimSpace(record[idx]))
			user.Active = val == "true" || val == "1" || val == "yes"
		}

		validateStart := time.Now()
		if err := s.validator.ValidateUser(&user); err != nil {
			totalValidationTime += time.Since(validateStart)
			validator.AppendValidationErrors(&result.Errors, rowNum, err)
			result.FailureCount++
			continue
		}
		totalValidationTime += time.Since(validateStart)

		batch = append(batch, userBatchItem{user: user, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			dbStart := time.Now()
			s.processBatchUsers(ctx, job, batch, &result)
			totalDBTime += time.Since(dbStart)
			batch = batch[:0]
		}
	}

	log.Printf("[job_id=%s] CSV parsing complete: %d records parsed in %v (valid=%d, invalid=%d, validation_time=%v, db_time=%v)",
		job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
		result.TotalRecords-result.FailureCount, result.FailureCount,
		totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))

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
	batchNum := (result.ProcessedRecords / s.batchSize) + 1
	batchStart := time.Now()
	log.Printf("[job_id=%s] Users import: processing batch %d (%d records)", job.ID, batchNum, len(batch))

	// Create row number mapping: batch index -> absolute row number
	prepStart := time.Now()
	rowMapping := make(map[int]int)
	users := make([]domain.User, len(batch))
	for i, item := range batch {
		users[i] = item.user
		rowMapping[i+1] = item.rowNum // BulkInsert uses 1-based row numbers
	}
	prepDuration := time.Since(prepStart)

	dbStart := time.Now()
	batchResult := s.userRepo.BulkInsert(ctx, users)
	dbDuration := time.Since(dbStart)

	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	log.Printf("[job_id=%s] Users import: batch %d done - progress: %d/%d records (success=%d, failed=%d) [prep=%v, db=%v, total=%v]",
		job.ID, batchNum, result.ProcessedRecords, result.TotalRecords, result.SuccessCount, result.FailureCount,
		prepDuration.Round(time.Microsecond), dbDuration.Round(time.Millisecond), time.Since(batchStart).Round(time.Millisecond))

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		// Log first few errors as samples
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			log.Printf("[job_id=%s] Batch %d error sample: row=%d, field=%s, reason=%s",
				job.ID, batchNum, err.Row, err.Field, err.Reason)
		}
		if len(batchResult.Errors) > maxSamples {
			log.Printf("[job_id=%s] Batch %d: ... and %d more errors",
				job.ID, batchNum, len(batchResult.Errors)-maxSamples)
		}
	}

	// Map batch row numbers to absolute row numbers
	for i := range batchResult.Errors {
		if absRow, ok := rowMapping[batchResult.Errors[i].Row]; ok {
			batchResult.Errors[i].Row = absRow
		}
	}
	result.Errors = append(result.Errors, batchResult.Errors...)
}

func (s *ImportService) importArticles(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{
		Errors: make([]domain.RecordError, 0, ErrorSlicePreallocSize),
	}
	log.Printf("[job_id=%s] Starting NDJSON parsing for articles", job.ID)
	parseStart := time.Now()
	var totalValidationTime time.Duration
	var totalDBTime time.Duration
	var lastProgressTime time.Time

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

		// Log progress every 1000 records
		if result.TotalRecords%1000 == 0 {
			now := time.Now()
			var sinceLast time.Duration
			if !lastProgressTime.IsZero() {
				sinceLast = now.Sub(lastProgressTime)
			}
			log.Printf("[job_id=%s] Parsing progress: %d records parsed in %v (last 1000 in %v, validation=%v, db=%v)",
				job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
				sinceLast.Round(time.Millisecond), totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))
			lastProgressTime = now
		}

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

		validateStart := time.Now()
		if err := s.validator.ValidateArticle(&article); err != nil {
			totalValidationTime += time.Since(validateStart)
			validator.AppendValidationErrors(&result.Errors, rowNum, err)
			result.FailureCount++
			continue
		}
		totalValidationTime += time.Since(validateStart)

		batch = append(batch, articleBatchItem{article: article, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			dbStart := time.Now()
			s.processBatchArticles(ctx, job, batch, &result)
			totalDBTime += time.Since(dbStart)
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[job_id=%s] Scanner error during articles import: %v", job.ID, err)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: fmt.Sprintf("scanner error: %v", err),
		})
		result.FailureCount++
	}

	log.Printf("[job_id=%s] NDJSON parsing complete: %d records parsed in %v (valid=%d, invalid=%d, validation_time=%v, db_time=%v)",
		job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
		result.TotalRecords-result.FailureCount, result.FailureCount,
		totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))

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
	batchNum := (result.ProcessedRecords / s.batchSize) + 1
	batchStart := time.Now()
	log.Printf("[job_id=%s] Articles import: processing batch %d (%d records)", job.ID, batchNum, len(batch))

	rowMapping := make(map[int]int)
	articles := make([]domain.Article, len(batch))
	for i, item := range batch {
		articles[i] = item.article
		rowMapping[i+1] = item.rowNum
	}

	dbStart := time.Now()
	batchResult := s.articleRepo.BulkInsert(ctx, articles)
	dbDuration := time.Since(dbStart)

	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	log.Printf("[job_id=%s] Articles import: batch %d done - progress: %d/%d records (success=%d, failed=%d) [db=%v, total=%v]",
		job.ID, batchNum, result.ProcessedRecords, result.TotalRecords, result.SuccessCount, result.FailureCount,
		dbDuration.Round(time.Millisecond), time.Since(batchStart).Round(time.Millisecond))

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			log.Printf("[job_id=%s] Batch %d error sample: row=%d, field=%s, reason=%s",
				job.ID, batchNum, err.Row, err.Field, err.Reason)
		}
		if len(batchResult.Errors) > maxSamples {
			log.Printf("[job_id=%s] Batch %d: ... and %d more errors",
				job.ID, batchNum, len(batchResult.Errors)-maxSamples)
		}
	}

	for i := range batchResult.Errors {
		if absRow, ok := rowMapping[batchResult.Errors[i].Row]; ok {
			batchResult.Errors[i].Row = absRow
		}
	}
	result.Errors = append(result.Errors, batchResult.Errors...)
}

func (s *ImportService) importComments(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{
		Errors: make([]domain.RecordError, 0, ErrorSlicePreallocSize),
	}
	log.Printf("[job_id=%s] Starting NDJSON parsing for comments", job.ID)
	parseStart := time.Now()
	var totalValidationTime time.Duration
	var totalDBTime time.Duration
	var lastProgressTime time.Time

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

		// Log progress every 1000 records
		if result.TotalRecords%1000 == 0 {
			now := time.Now()
			var sinceLast time.Duration
			if !lastProgressTime.IsZero() {
				sinceLast = now.Sub(lastProgressTime)
			}
			log.Printf("[job_id=%s] Parsing progress: %d records parsed in %v (last 1000 in %v, validation=%v, db=%v)",
				job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
				sinceLast.Round(time.Millisecond), totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))
			lastProgressTime = now
		}

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

		validateStart := time.Now()
		if err := s.validator.ValidateComment(&comment); err != nil {
			totalValidationTime += time.Since(validateStart)
			validator.AppendValidationErrors(&result.Errors, rowNum, err)
			result.FailureCount++
			continue
		}
		totalValidationTime += time.Since(validateStart)

		batch = append(batch, commentBatchItem{comment: comment, rowNum: rowNum})

		if len(batch) >= s.batchSize {
			dbStart := time.Now()
			s.processBatchComments(ctx, job, batch, &result)
			totalDBTime += time.Since(dbStart)
			batch = batch[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[job_id=%s] Scanner error during comments import: %v", job.ID, err)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: fmt.Sprintf("scanner error: %v", err),
		})
		result.FailureCount++
	}

	log.Printf("[job_id=%s] NDJSON parsing complete: %d records parsed in %v (valid=%d, invalid=%d, validation_time=%v, db_time=%v)",
		job.ID, result.TotalRecords, time.Since(parseStart).Round(time.Millisecond),
		result.TotalRecords-result.FailureCount, result.FailureCount,
		totalValidationTime.Round(time.Millisecond), totalDBTime.Round(time.Millisecond))

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
	batchNum := (result.ProcessedRecords / s.batchSize) + 1
	batchStart := time.Now()
	log.Printf("[job_id=%s] Comments import: processing batch %d (%d records)", job.ID, batchNum, len(batch))

	rowMapping := make(map[int]int)
	comments := make([]domain.Comment, len(batch))
	for i, item := range batch {
		comment := item.comment
		// Strip cm_ prefix from ID before inserting into database (DB expects UUID format)
		comment.ID = strings.TrimPrefix(comment.ID, "cm_")
		comments[i] = comment
		rowMapping[i+1] = item.rowNum
	}

	dbStart := time.Now()
	batchResult := s.commentRepo.BulkInsert(ctx, comments)
	dbDuration := time.Since(dbStart)

	result.ProcessedRecords += batchResult.SuccessCount + batchResult.FailedCount
	result.SuccessCount += batchResult.SuccessCount
	result.FailureCount += batchResult.FailedCount

	log.Printf("[job_id=%s] Comments import: batch %d done - progress: %d/%d records (success=%d, failed=%d) [db=%v, total=%v]",
		job.ID, batchNum, result.ProcessedRecords, result.TotalRecords, result.SuccessCount, result.FailureCount,
		dbDuration.Round(time.Millisecond), time.Since(batchStart).Round(time.Millisecond))

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			log.Printf("[job_id=%s] Batch %d error sample: row=%d, field=%s, reason=%s",
				job.ID, batchNum, err.Row, err.Field, err.Reason)
		}
		if len(batchResult.Errors) > maxSamples {
			log.Printf("[job_id=%s] Batch %d: ... and %d more errors",
				job.ID, batchNum, len(batchResult.Errors)-maxSamples)
		}
	}

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
