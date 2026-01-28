package repository

import (
	"context"

	"bulk-import-export/internal/domain"
)

// UserRepository defines methods for user data access.
type UserRepository interface {
	BulkInsert(ctx context.Context, users []domain.User) domain.BatchResult
	StreamAll(ctx context.Context, callback func(domain.User) error) error
}

// ArticleRepository defines methods for article data access.
type ArticleRepository interface {
	BulkInsert(ctx context.Context, articles []domain.Article) domain.BatchResult
	StreamAll(ctx context.Context, callback func(domain.Article) error) error
}

// CommentRepository defines methods for comment data access.
type CommentRepository interface {
	BulkInsert(ctx context.Context, comments []domain.Comment) domain.BatchResult
	StreamAll(ctx context.Context, callback func(domain.Comment) error) error
}

// ImportJobRepository defines methods for import job data access.
type ImportJobRepository interface {
	CreateImportJob(ctx context.Context, job *domain.ImportJob) error
	GetImportJob(ctx context.Context, id string) (*domain.ImportJob, error)
	GetImportJobByIdempotencyToken(ctx context.Context, token string) (*domain.ImportJob, error)
	UpdateImportJob(ctx context.Context, job *domain.ImportJob) error
}

// ExportJobRepository defines methods for export job data access.
type ExportJobRepository interface {
	CreateExportJob(ctx context.Context, job *domain.ExportJob) error
	GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error)
	GetExportJobByIdempotencyToken(ctx context.Context, token string) (*domain.ExportJob, error)
	UpdateExportJob(ctx context.Context, job *domain.ExportJob) error
}

// JobRepository combines all job-related repository interfaces.
type JobRepository interface {
	ImportJobRepository
	ExportJobRepository
}
