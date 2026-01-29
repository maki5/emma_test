package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/repository"
)

const (
	// DefaultExportTimeout is the timeout for export processing
	DefaultExportTimeout = 30 * time.Minute

	// ExportQueueSendTimeout is the timeout for sending tasks to the queue
	ExportQueueSendTimeout = 5 * time.Second
)

// ExportService handles bulk export operations.
type ExportService struct {
	userRepo    repository.UserRepository
	articleRepo repository.ArticleRepository
	commentRepo repository.CommentRepository
	jobRepo     repository.JobRepository

	exportDir   string
	workerCount int
	timeout     time.Duration

	jobQueue chan exportTask
	stopChan chan struct{}
	wg       sync.WaitGroup
	closed   bool
	mu       sync.RWMutex
}

type exportTask struct {
	job       *domain.ExportJob
	requestID string
}

// NewExportService creates a new ExportService with worker pool.
func NewExportService(
	userRepo repository.UserRepository,
	articleRepo repository.ArticleRepository,
	commentRepo repository.CommentRepository,
	jobRepo repository.JobRepository,
	exportDir string,
	workerCount int,
) (*ExportService, error) {
	s := &ExportService{
		userRepo:    userRepo,
		articleRepo: articleRepo,
		commentRepo: commentRepo,
		jobRepo:     jobRepo,
		exportDir:   exportDir,
		workerCount: workerCount,
		timeout:     DefaultExportTimeout,
		jobQueue:    make(chan exportTask, workerCount*2),
		stopChan:    make(chan struct{}),
	}

	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return nil, fmt.Errorf("create export directory: %w", err)
	}

	for i := 0; i < workerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}

	return s, nil
}

func (s *ExportService) worker() {
	defer s.wg.Done()

	for {
		select {
		case task, ok := <-s.jobQueue:
			if !ok {
				return
			}
			s.processExport(task)
		case <-s.stopChan:
			return
		}
	}
}

// Close shuts down the worker pool immediately.
func (s *ExportService) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	close(s.stopChan)
	close(s.jobQueue)
	s.wg.Wait()
}

// StartExport creates an export job and queues it for processing.
func (s *ExportService) StartExport(ctx context.Context, resourceType, format, idempotencyToken, requestID string) (*domain.ExportJob, error) {
	log.Printf("[request_id=%s] Starting export for resource_type=%s, format=%s", requestID, resourceType, format)

	existingJob, err := s.jobRepo.GetExportJobByIdempotencyToken(ctx, idempotencyToken)
	if err != nil {
		return nil, fmt.Errorf("check idempotency token: %w", err)
	}
	if existingJob != nil {
		log.Printf("[request_id=%s] Returning existing job %s for idempotency token", requestID, existingJob.ID)
		return existingJob, nil
	}

	now := time.Now()
	job := &domain.ExportJob{
		ID:               uuid.New().String(),
		ResourceType:     resourceType,
		Format:           format,
		Status:           domain.JobStatusPending,
		TotalRecords:     0,
		IdempotencyToken: idempotencyToken,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.jobRepo.CreateExportJob(ctx, job); err != nil {
		return nil, fmt.Errorf("create export job: %w", err)
	}

	// Check if service is shutting down
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("export service is shutting down")
	}
	s.mu.RUnlock()

	// Send to queue with timeout to prevent blocking
	select {
	case s.jobQueue <- exportTask{job: job, requestID: requestID}:
		log.Printf("[request_id=%s] Job %s queued for processing", requestID, job.ID)
	case <-time.After(ExportQueueSendTimeout):
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
			case s.jobQueue <- exportTask{job: job, requestID: requestID}:
			case <-s.stopChan:
			}
		}()
	}

	return job, nil
}

func (s *ExportService) processExport(task exportTask) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	job := task.job
	requestID := task.requestID
	startTime := time.Now()

	log.Printf("[request_id=%s] Processing export job %s: resource_type=%s, format=%s", requestID, job.ID, job.ResourceType, job.Format)

	job.Status = domain.JobStatusProcessing
	job.UpdatedAt = time.Now()
	if err := s.jobRepo.UpdateExportJob(ctx, job); err != nil {
		log.Printf("[request_id=%s] Failed to update export job status to processing: %v", requestID, err)
	}

	var err error
	var filePath string
	var totalRecords int

	switch job.ResourceType {
	case "users":
		filePath, totalRecords, err = s.exportUsers(ctx, job)
	case "articles":
		filePath, totalRecords, err = s.exportArticles(ctx, job)
	case "comments":
		filePath, totalRecords, err = s.exportComments(ctx, job)
	default:
		job.Status = domain.JobStatusFailed
		errMsg := fmt.Sprintf("unsupported resource type: %s", job.ResourceType)
		job.ErrorMessage = &errMsg
		job.UpdatedAt = time.Now()
		if err := s.jobRepo.UpdateExportJob(ctx, job); err != nil {
			log.Printf("[request_id=%s] Failed to update export job: %v", requestID, err)
		}
		return
	}

	now := time.Now()
	job.TotalRecords = totalRecords
	job.UpdatedAt = now
	job.CompletedAt = &now

	if err != nil {
		job.Status = domain.JobStatusFailed
		errMsg := err.Error()
		job.ErrorMessage = &errMsg
	} else {
		job.Status = domain.JobStatusCompleted
		job.FilePath = filePath
	}

	if err := s.jobRepo.UpdateExportJob(ctx, job); err != nil {
		log.Printf("[request_id=%s] Failed to update export job: %v", requestID, err)
	}

	elapsed := time.Since(startTime)
	log.Printf("[request_id=%s] Export job %s completed: status=%s, total=%d, elapsed=%s",
		requestID, job.ID, job.Status, job.TotalRecords, elapsed.Round(time.Millisecond))
}

func (s *ExportService) exportUsers(ctx context.Context, job *domain.ExportJob) (string, int, error) {
	ext := ".csv"
	if job.Format == "ndjson" {
		ext = ".ndjson"
	}

	filename := fmt.Sprintf("users_%s%s", job.ID, ext)
	filePath := filepath.Join(s.exportDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}

	var exportErr error
	defer func() {
		// Sync to ensure data is flushed to disk
		if syncErr := file.Sync(); syncErr != nil && exportErr == nil {
			log.Printf("[export] Warning: failed to sync file %s: %v", filePath, syncErr)
		}
		file.Close()
		// Clean up partial file on error
		if exportErr != nil {
			if removeErr := os.Remove(filePath); removeErr != nil {
				log.Printf("[export] Warning: failed to remove partial file %s: %v", filePath, removeErr)
			}
		}
	}()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)

		if err := writer.Write([]string{"id", "email", "name", "role", "is_active", "created_at", "updated_at"}); err != nil {
			exportErr = fmt.Errorf("write header: %w", err)
			return "", 0, exportErr
		}

		err = s.userRepo.StreamAll(ctx, func(user domain.User) error {
			record := []string{
				user.ID,
				user.Email,
				user.Name,
				user.Role,
				fmt.Sprintf("%v", user.Active),
				user.CreatedAt.Format(time.RFC3339),
				user.UpdatedAt.Format(time.RFC3339),
			}
			if err := writer.Write(record); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream users: %w", err)
			return "", 0, exportErr
		}

		// Flush and check for errors
		writer.Flush()
		if err := writer.Error(); err != nil {
			exportErr = fmt.Errorf("flush csv writer: %w", err)
			return "", 0, exportErr
		}
	} else {
		encoder := json.NewEncoder(file)

		err = s.userRepo.StreamAll(ctx, func(user domain.User) error {
			if err := encoder.Encode(user); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream users: %w", err)
			return "", 0, exportErr
		}
	}

	return filePath, count, nil
}

func (s *ExportService) exportArticles(ctx context.Context, job *domain.ExportJob) (string, int, error) {
	ext := ".csv"
	if job.Format == "ndjson" {
		ext = ".ndjson"
	}

	filename := fmt.Sprintf("articles_%s%s", job.ID, ext)
	filePath := filepath.Join(s.exportDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}

	var exportErr error
	defer func() {
		if syncErr := file.Sync(); syncErr != nil && exportErr == nil {
			log.Printf("[export] Warning: failed to sync file %s: %v", filePath, syncErr)
		}
		file.Close()
		if exportErr != nil {
			if removeErr := os.Remove(filePath); removeErr != nil {
				log.Printf("[export] Warning: failed to remove partial file %s: %v", filePath, removeErr)
			}
		}
	}()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)

		if err := writer.Write([]string{"id", "title", "slug", "body", "author_id", "status", "published_at", "created_at", "updated_at"}); err != nil {
			exportErr = fmt.Errorf("write header: %w", err)
			return "", 0, exportErr
		}

		err = s.articleRepo.StreamAll(ctx, func(article domain.Article) error {
			publishedAt := ""
			if article.PublishedAt != nil {
				publishedAt = article.PublishedAt.Format(time.RFC3339)
			}

			record := []string{
				article.ID,
				article.Title,
				article.Slug,
				article.Body,
				article.AuthorID,
				article.Status,
				publishedAt,
				article.CreatedAt.Format(time.RFC3339),
				article.UpdatedAt.Format(time.RFC3339),
			}
			if err := writer.Write(record); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream articles: %w", err)
			return "", 0, exportErr
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			exportErr = fmt.Errorf("flush csv writer: %w", err)
			return "", 0, exportErr
		}
	} else {
		encoder := json.NewEncoder(file)

		err = s.articleRepo.StreamAll(ctx, func(article domain.Article) error {
			if err := encoder.Encode(article); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream articles: %w", err)
			return "", 0, exportErr
		}
	}

	return filePath, count, nil
}

func (s *ExportService) exportComments(ctx context.Context, job *domain.ExportJob) (string, int, error) {
	ext := ".csv"
	if job.Format == "ndjson" {
		ext = ".ndjson"
	}

	filename := fmt.Sprintf("comments_%s%s", job.ID, ext)
	filePath := filepath.Join(s.exportDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}

	var exportErr error
	defer func() {
		if syncErr := file.Sync(); syncErr != nil && exportErr == nil {
			log.Printf("[export] Warning: failed to sync file %s: %v", filePath, syncErr)
		}
		file.Close()
		if exportErr != nil {
			if removeErr := os.Remove(filePath); removeErr != nil {
				log.Printf("[export] Warning: failed to remove partial file %s: %v", filePath, removeErr)
			}
		}
	}()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)

		if err := writer.Write([]string{"id", "article_id", "user_id", "body", "created_at"}); err != nil {
			exportErr = fmt.Errorf("write header: %w", err)
			return "", 0, exportErr
		}

		err = s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			// Add cm_ prefix back to comment ID for export
			exportID := "cm_" + comment.ID
			record := []string{
				exportID,
				comment.ArticleID,
				comment.UserID,
				comment.Body,
				comment.CreatedAt.Format(time.RFC3339),
			}
			if err := writer.Write(record); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream comments: %w", err)
			return "", 0, exportErr
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			exportErr = fmt.Errorf("flush csv writer: %w", err)
			return "", 0, exportErr
		}
	} else {
		encoder := json.NewEncoder(file)

		err = s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			// Add cm_ prefix back to comment ID for export
			comment.ID = "cm_" + comment.ID
			if err := encoder.Encode(comment); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})

		if err != nil {
			exportErr = fmt.Errorf("stream comments: %w", err)
			return "", 0, exportErr
		}
	}

	return filePath, count, nil
}

// GetExportJob retrieves an export job by ID.
func (s *ExportService) GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error) {
	return s.jobRepo.GetExportJob(ctx, id)
}

// StreamUsers streams users directly to the writer in the specified format.
func (s *ExportService) StreamUsers(ctx context.Context, format string, writer StreamWriter) (int, error) {
	var count int

	if format == "csv" {
		// Write CSV header
		if err := writer.Write([]byte("id,email,name,role,is_active,created_at,updated_at\n")); err != nil {
			return 0, fmt.Errorf("write header: %w", err)
		}

		err := s.userRepo.StreamAll(ctx, func(user domain.User) error {
			// Use csv.Writer to properly escape fields
			var buf bytes.Buffer
			csvWriter := csv.NewWriter(&buf)
			record := []string{
				user.ID, user.Email, user.Name, user.Role,
				fmt.Sprintf("%v", user.Active),
				user.CreatedAt.Format(time.RFC3339),
				user.UpdatedAt.Format(time.RFC3339),
			}
			if err := csvWriter.Write(record); err != nil {
				return fmt.Errorf("encode csv: %w", err)
			}
			csvWriter.Flush()
			if err := writer.Write(buf.Bytes()); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream users: %w", err)
		}
	} else {
		err := s.userRepo.StreamAll(ctx, func(user domain.User) error {
			data, err := json.Marshal(user)
			if err != nil {
				return fmt.Errorf("marshal user: %w", err)
			}
			if err := writer.Write(append(data, '\n')); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream users: %w", err)
		}
	}

	writer.Flush()
	return count, nil
}

// StreamArticles streams articles directly to the writer in the specified format.
func (s *ExportService) StreamArticles(ctx context.Context, format string, writer StreamWriter) (int, error) {
	var count int

	if format == "csv" {
		// Write CSV header
		if err := writer.Write([]byte("id,title,slug,body,author_id,status,published_at,created_at,updated_at\n")); err != nil {
			return 0, fmt.Errorf("write header: %w", err)
		}

		err := s.articleRepo.StreamAll(ctx, func(article domain.Article) error {
			publishedAt := ""
			if article.PublishedAt != nil {
				publishedAt = article.PublishedAt.Format(time.RFC3339)
			}
			// Use csv.Writer to properly escape fields
			var buf bytes.Buffer
			csvWriter := csv.NewWriter(&buf)
			record := []string{
				article.ID, article.Title, article.Slug, article.Body, article.AuthorID,
				article.Status, publishedAt,
				article.CreatedAt.Format(time.RFC3339), article.UpdatedAt.Format(time.RFC3339),
			}
			if err := csvWriter.Write(record); err != nil {
				return fmt.Errorf("encode csv: %w", err)
			}
			csvWriter.Flush()
			if err := writer.Write(buf.Bytes()); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream articles: %w", err)
		}
	} else {
		err := s.articleRepo.StreamAll(ctx, func(article domain.Article) error {
			data, err := json.Marshal(article)
			if err != nil {
				return fmt.Errorf("marshal article: %w", err)
			}
			if err := writer.Write(append(data, '\n')); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream articles: %w", err)
		}
	}

	writer.Flush()
	return count, nil
}

// StreamComments streams comments directly to the writer in the specified format.
func (s *ExportService) StreamComments(ctx context.Context, format string, writer StreamWriter) (int, error) {
	var count int

	if format == "csv" {
		// Write CSV header
		if err := writer.Write([]byte("id,article_id,user_id,body,created_at\n")); err != nil {
			return 0, fmt.Errorf("write header: %w", err)
		}

		err := s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			// Add cm_ prefix back to comment ID for export
			exportID := "cm_" + comment.ID
			// Use csv.Writer to properly escape fields
			var buf bytes.Buffer
			csvWriter := csv.NewWriter(&buf)
			record := []string{
				exportID, comment.ArticleID, comment.UserID, comment.Body,
				comment.CreatedAt.Format(time.RFC3339),
			}
			if err := csvWriter.Write(record); err != nil {
				return fmt.Errorf("encode csv: %w", err)
			}
			csvWriter.Flush()
			if err := writer.Write(buf.Bytes()); err != nil {
				return fmt.Errorf("write row: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream comments: %w", err)
		}
	} else {
		err := s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			// Add cm_ prefix back to comment ID for export
			comment.ID = "cm_" + comment.ID
			data, err := json.Marshal(comment)
			if err != nil {
				return fmt.Errorf("marshal comment: %w", err)
			}
			if err := writer.Write(append(data, '\n')); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("stream comments: %w", err)
		}
	}

	writer.Flush()
	return count, nil
}
