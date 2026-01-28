package service

import (
	"context"
	"io"

	"bulk-import-export/internal/domain"
)

// StreamWriter interface for streaming export data.
type StreamWriter interface {
	Write(data []byte) error
	Flush()
}

// ImportServiceInterface defines the interface for import operations.
// Used for dependency injection and mocking in tests.
type ImportServiceInterface interface {
	// StartImport starts a new import job.
	StartImport(ctx context.Context, resourceType, idempotencyToken, filename, requestID string, reader io.Reader) (*domain.ImportJob, error)
	// GetImportJob retrieves an import job by ID.
	GetImportJob(ctx context.Context, id string) (*domain.ImportJob, error)
	// Close shuts down the import service workers.
	Close()
}

// ExportServiceInterface defines the interface for export operations.
// Used for dependency injection and mocking in tests.
type ExportServiceInterface interface {
	// StartExport starts a new async export job.
	StartExport(ctx context.Context, resourceType, format, idempotencyToken, requestID string) (*domain.ExportJob, error)
	// GetExportJob retrieves an export job by ID.
	GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error)
	// StreamUsers streams users directly to the writer.
	StreamUsers(ctx context.Context, format string, writer StreamWriter) (int, error)
	// StreamArticles streams articles directly to the writer.
	StreamArticles(ctx context.Context, format string, writer StreamWriter) (int, error)
	// StreamComments streams comments directly to the writer.
	StreamComments(ctx context.Context, format string, writer StreamWriter) (int, error)
	// Close shuts down the export service workers.
	Close()
}
