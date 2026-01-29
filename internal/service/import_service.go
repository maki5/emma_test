package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/logger"
	"bulk-import-export/internal/metrics"
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
	logger.WithRequestID(requestID).Info("Starting import",
		slog.String("resource_type", resourceType))

	existingJob, err := s.jobRepo.GetImportJobByIdempotencyToken(ctx, idempotencyToken)
	if err != nil {
		return nil, fmt.Errorf("check idempotency token: %w", err)
	}
	if existingJob != nil {
		logger.WithRequestID(requestID).Info("Returning existing job for idempotency token",
			slog.String("job_id", existingJob.ID))
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
		logger.WithRequestID(requestID).Info("Job queued for processing",
			slog.String("job_id", job.ID))
	case <-time.After(QueueSendTimeout):
		logger.WithRequestID(requestID).Warn("Queue full, job will be processed when capacity available",
			slog.String("job_id", job.ID))
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

	// Track job in metrics
	metrics.StartJob("import", job.ResourceType)
	defer metrics.EndJob("import", job.ResourceType)

	logger.WithRequestID(requestID).Info("Processing import job",
		slog.String("job_id", job.ID),
		slog.String("resource_type", job.ResourceType))

	job.Status = domain.JobStatusProcessing
	job.UpdatedAt = time.Now()
	if err := s.jobRepo.UpdateImportJob(ctx, job); err != nil {
		logger.WithRequestID(requestID).Error("Failed to update import job status to processing",
			slog.String("error", err.Error()))
	}

	// Create a reader from the buffered data
	reader := bytes.NewReader(task.data)
	logger.WithRequestID(requestID).Info("Starting import processing",
		slog.String("resource_type", job.ResourceType),
		slog.Int("data_size_bytes", len(task.data)))

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
			logger.WithJobID(job.ID).Error("Failed to update import job",
				slog.String("error", err.Error()))
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
		logger.WithRequestID(requestID).Error("Failed to update import job",
			slog.String("error", err.Error()))
	}

	elapsed := time.Since(startTime)
	logger.WithRequestID(requestID).Info("Import job completed",
		slog.String("job_id", job.ID),
		slog.String("status", string(job.Status)),
		slog.Int("total_records", job.TotalRecords),
		slog.Int("success_count", job.SuccessCount),
		slog.Int("failure_count", job.FailureCount),
		slog.Duration("elapsed", elapsed))

	// Record job completion metrics
	metrics.ObserveJobCompletion("import", job.ResourceType, string(job.Status), elapsed.Seconds(), job.SuccessCount, job.FailureCount)
}

func (s *ImportService) importUsers(ctx context.Context, job *domain.ImportJob, reader io.Reader) domain.ImportResult {
	result := domain.ImportResult{
		Errors: make([]domain.RecordError, 0, ErrorSlicePreallocSize),
	}
	logger.WithJobID(job.ID).Info("Starting CSV parsing for users")

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
	logger.WithJobID(job.ID).Debug("CSV header parsed",
		slog.Any("header", header))

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
			logger.WithJobID(job.ID).Info("Parsing progress",
				slog.Int("parsed_records", result.TotalRecords),
				slog.Duration("elapsed", time.Since(parseStart)),
				slog.Duration("since_last", sinceLast),
				slog.Duration("validation_time", totalValidationTime),
				slog.Duration("db_time", totalDBTime))
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

	logger.WithJobID(job.ID).Info("CSV parsing complete",
		slog.Int("total_records", result.TotalRecords),
		slog.Duration("parse_time", time.Since(parseStart)),
		slog.Int("valid_records", result.TotalRecords-result.FailureCount),
		slog.Int("invalid_records", result.FailureCount),
		slog.Duration("validation_time", totalValidationTime),
		slog.Duration("db_time", totalDBTime))

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
	logger.WithJobID(job.ID).Info("Processing users batch",
		slog.Int("batch_number", batchNum),
		slog.Int("batch_size", len(batch)))

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

	logger.WithJobID(job.ID).Info("Batch processing complete",
		slog.Int("batch_number", batchNum),
		slog.Int("processed_records", result.ProcessedRecords),
		slog.Int("total_records", result.TotalRecords),
		slog.Int("success_count", result.SuccessCount),
		slog.Int("failure_count", result.FailureCount),
		slog.Duration("prep_time", prepDuration),
		slog.Duration("db_time", dbDuration),
		slog.Duration("total_time", time.Since(batchStart)))

	// Record batch metrics
	metrics.ObserveBatchDuration("import", "users", "db", dbDuration.Seconds())

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		// Log first few errors as samples
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			logger.WithJobID(job.ID).Warn("Batch error sample",
				slog.Int("batch_number", batchNum),
				slog.Int("row", err.Row),
				slog.String("field", err.Field),
				slog.String("reason", err.Reason))
		}
		if len(batchResult.Errors) > maxSamples {
			logger.WithJobID(job.ID).Warn("Additional batch errors",
				slog.Int("batch_number", batchNum),
				slog.Int("additional_errors", len(batchResult.Errors)-maxSamples))
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
	logger.WithJobID(job.ID).Info("Starting NDJSON parsing for articles")
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
			logger.WithJobID(job.ID).Info("Parsing progress",
				slog.Int("parsed_records", result.TotalRecords),
				slog.Duration("elapsed", time.Since(parseStart)),
				slog.Duration("since_last", sinceLast),
				slog.Duration("validation_time", totalValidationTime),
				slog.Duration("db_time", totalDBTime))
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
		logger.WithJobID(job.ID).Error("Scanner error during articles import", slog.String("error", err.Error()))
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: fmt.Sprintf("scanner error: %v", err),
		})
		result.FailureCount++
	}

	logger.WithJobID(job.ID).Info("NDJSON parsing complete",
		slog.Int("total_records", result.TotalRecords),
		slog.Duration("parse_time", time.Since(parseStart)),
		slog.Int("valid_records", result.TotalRecords-result.FailureCount),
		slog.Int("invalid_records", result.FailureCount),
		slog.Duration("validation_time", totalValidationTime),
		slog.Duration("db_time", totalDBTime))

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
	logger.WithJobID(job.ID).Info("Articles import: processing batch",
		slog.Int("batch_num", batchNum),
		slog.Int("records", len(batch)))

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

	logger.WithJobID(job.ID).Info("Articles import: batch done",
		slog.Int("batch_num", batchNum),
		slog.Int("processed_records", result.ProcessedRecords),
		slog.Int("total_records", result.TotalRecords),
		slog.Int("success_count", result.SuccessCount),
		slog.Int("failure_count", result.FailureCount),
		slog.Duration("db_time", dbDuration),
		slog.Duration("batch_time", time.Since(batchStart)))

	// Record batch metrics
	metrics.ObserveBatchDuration("import", "articles", "db", dbDuration.Seconds())

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			logger.WithJobID(job.ID).Error("Batch error sample",
				slog.Int("batch_num", batchNum),
				slog.Int("row", err.Row),
				slog.String("field", err.Field),
				slog.String("reason", err.Reason))
		}
		if len(batchResult.Errors) > maxSamples {
			logger.WithJobID(job.ID).Error("Batch has more errors",
				slog.Int("batch_num", batchNum),
				slog.Int("additional_errors", len(batchResult.Errors)-maxSamples))
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
	logger.WithJobID(job.ID).Info("Starting NDJSON parsing for comments")
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
			logger.WithJobID(job.ID).Info("Parsing progress",
				slog.Int("parsed_records", result.TotalRecords),
				slog.Duration("elapsed", time.Since(parseStart)),
				slog.Duration("since_last", sinceLast),
				slog.Duration("validation_time", totalValidationTime),
				slog.Duration("db_time", totalDBTime))
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
		logger.WithJobID(job.ID).Error("Scanner error during articles import", slog.String("error", err.Error()))
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    rowNum,
			Field:  "scanner",
			Reason: fmt.Sprintf("scanner error: %v", err),
		})
		result.FailureCount++
	}

	logger.WithJobID(job.ID).Info("NDJSON parsing complete",
		slog.Int("total_records", result.TotalRecords),
		slog.Duration("parse_time", time.Since(parseStart)),
		slog.Int("valid_records", result.TotalRecords-result.FailureCount),
		slog.Int("invalid_records", result.FailureCount),
		slog.Duration("validation_time", totalValidationTime),
		slog.Duration("db_time", totalDBTime))

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
	logger.WithJobID(job.ID).Info("Comments import: processing batch",
		slog.Int("batch_num", batchNum),
		slog.Int("records", len(batch)))

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

	logger.WithJobID(job.ID).Info("Comments import: batch done",
		slog.Int("batch_num", batchNum),
		slog.Int("processed_records", result.ProcessedRecords),
		slog.Int("total_records", result.TotalRecords),
		slog.Int("success_count", result.SuccessCount),
		slog.Int("failure_count", result.FailureCount),
		slog.Duration("db_time", dbDuration),
		slog.Duration("batch_time", time.Since(batchStart)))

	// Record batch metrics
	metrics.ObserveBatchDuration("import", "comments", "db", dbDuration.Seconds())

	// Log batch errors if any
	if batchResult.FailedCount > 0 && len(batchResult.Errors) > 0 {
		maxSamples := 3
		if len(batchResult.Errors) < maxSamples {
			maxSamples = len(batchResult.Errors)
		}
		for i := 0; i < maxSamples; i++ {
			err := batchResult.Errors[i]
			logger.WithJobID(job.ID).Error("Batch error sample",
				slog.Int("batch_num", batchNum),
				slog.Int("row", err.Row),
				slog.String("field", err.Field),
				slog.String("reason", err.Reason))
		}
		if len(batchResult.Errors) > maxSamples {
			logger.WithJobID(job.ID).Error("Batch has more errors",
				slog.Int("batch_num", batchNum),
				slog.Int("additional_errors", len(batchResult.Errors)-maxSamples))
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
