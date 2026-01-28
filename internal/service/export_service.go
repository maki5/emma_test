package service

import (
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

	// Send to queue with timeout to prevent blocking
	select {
	case s.jobQueue <- exportTask{job: job, requestID: requestID}:
		log.Printf("[request_id=%s] Job %s queued for processing", requestID, job.ID)
	case <-time.After(ExportQueueSendTimeout):
		log.Printf("[request_id=%s] Warning: queue full, job %s will be processed when capacity available", requestID, job.ID)
		// Still try to send, but don't block indefinitely
		go func() {
			s.jobQueue <- exportTask{job: job, requestID: requestID}
		}()
	}

	return job, nil
}

func (s *ExportService) processExport(task exportTask) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	job := task.job
	requestID := task.requestID

	log.Printf("[request_id=%s] Processing export job %s", requestID, job.ID)

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

	log.Printf("[request_id=%s] Export job %s completed: status=%s, total=%d",
		requestID, job.ID, job.Status, job.TotalRecords)
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
	defer file.Close()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)
		defer writer.Flush()

		if err := writer.Write([]string{"id", "email", "name", "role", "is_active", "created_at", "updated_at"}); err != nil {
			return "", 0, fmt.Errorf("write header: %w", err)
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
	} else {
		encoder := json.NewEncoder(file)

		err = s.userRepo.StreamAll(ctx, func(user domain.User) error {
			if err := encoder.Encode(user); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
	}

	if err != nil {
		return "", 0, fmt.Errorf("stream users: %w", err)
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
	defer file.Close()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)
		defer writer.Flush()

		if err := writer.Write([]string{"id", "title", "slug", "body", "author_id", "status", "published_at", "created_at", "updated_at"}); err != nil {
			return "", 0, fmt.Errorf("write header: %w", err)
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
	} else {
		encoder := json.NewEncoder(file)

		err = s.articleRepo.StreamAll(ctx, func(article domain.Article) error {
			if err := encoder.Encode(article); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
	}

	if err != nil {
		return "", 0, fmt.Errorf("stream articles: %w", err)
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
	defer file.Close()

	var count int

	if job.Format == "csv" {
		writer := csv.NewWriter(file)
		defer writer.Flush()

		if err := writer.Write([]string{"id", "article_id", "user_id", "body", "created_at"}); err != nil {
			return "", 0, fmt.Errorf("write header: %w", err)
		}

		err = s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			record := []string{
				comment.ID,
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
	} else {
		encoder := json.NewEncoder(file)

		err = s.commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			if err := encoder.Encode(comment); err != nil {
				return fmt.Errorf("write json: %w", err)
			}
			count++
			return nil
		})
	}

	if err != nil {
		return "", 0, fmt.Errorf("stream comments: %w", err)
	}

	return filePath, count, nil
}

// GetExportJob retrieves an export job by ID.
func (s *ExportService) GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error) {
	return s.jobRepo.GetExportJob(ctx, id)
}

// ListExportJobs lists all export jobs.
func (s *ExportService) ListExportJobs(ctx context.Context) ([]domain.ExportJob, error) {
	return s.jobRepo.ListExportJobs(ctx)
}

// GetExportFilePath returns the file path for a completed export job.
func (s *ExportService) GetExportFilePath(ctx context.Context, id string) (string, error) {
	job, err := s.jobRepo.GetExportJob(ctx, id)
	if err != nil {
		return "", err
	}
	if job == nil {
		return "", fmt.Errorf("export job not found")
	}

	if job.Status != domain.JobStatusCompleted {
		return "", fmt.Errorf("export job not completed")
	}

	if job.FilePath == "" {
		return "", fmt.Errorf("export file not available")
	}

	return job.FilePath, nil
}
