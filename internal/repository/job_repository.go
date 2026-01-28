package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"bulk-import-export/internal/domain"
)

// PostgresJobRepository implements JobRepository using PostgreSQL.
type PostgresJobRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresJobRepository creates a new PostgresJobRepository.
func NewPostgresJobRepository(pool *pgxpool.Pool) *PostgresJobRepository {
	return &PostgresJobRepository{pool: pool}
}

// CreateImportJob creates a new import job.
func (r *PostgresJobRepository) CreateImportJob(ctx context.Context, job *domain.ImportJob) error {
	metadata, err := json.Marshal(job.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO import_jobs (id, resource_type, status, total_records, processed_records, 
			success_count, failure_count, idempotency_token, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, job.ID, job.ResourceType, job.Status, job.TotalRecords, job.ProcessedRecords,
		job.SuccessCount, job.FailureCount, job.IdempotencyToken, metadata, job.CreatedAt, job.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			strings.Contains(pgErr.ConstraintName, "idempotency_token") {
			existingJob, fetchErr := r.GetImportJobByIdempotencyToken(ctx, job.IdempotencyToken)
			if fetchErr != nil {
				return fmt.Errorf("fetch existing job after race: %w", fetchErr)
			}
			if existingJob != nil {
				*job = *existingJob
				return nil
			}
		}
		return fmt.Errorf("insert import job: %w", err)
	}

	return nil
}

// GetImportJob retrieves an import job by ID.
func (r *PostgresJobRepository) GetImportJob(ctx context.Context, id string) (*domain.ImportJob, error) {
	var job domain.ImportJob
	var metadata []byte
	var completedAt *time.Time
	var errorMsg *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_type, status, total_records, processed_records,
			success_count, failure_count, idempotency_token, metadata, error_message,
			created_at, updated_at, completed_at
		FROM import_jobs
		WHERE id = $1
	`, id).Scan(&job.ID, &job.ResourceType, &job.Status, &job.TotalRecords, &job.ProcessedRecords,
		&job.SuccessCount, &job.FailureCount, &job.IdempotencyToken, &metadata, &errorMsg,
		&job.CreatedAt, &job.UpdatedAt, &completedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get import job: %w", err)
	}

	if metadata != nil {
		if err := json.Unmarshal(metadata, &job.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	job.CompletedAt = completedAt
	job.ErrorMessage = errorMsg

	return &job, nil
}

// GetImportJobByIdempotencyToken retrieves an import job by idempotency token.
func (r *PostgresJobRepository) GetImportJobByIdempotencyToken(ctx context.Context, token string) (*domain.ImportJob, error) {
	var job domain.ImportJob
	var metadata []byte
	var completedAt *time.Time
	var errorMsg *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_type, status, total_records, processed_records,
			success_count, failure_count, idempotency_token, metadata, error_message,
			created_at, updated_at, completed_at
		FROM import_jobs
		WHERE idempotency_token = $1
	`, token).Scan(&job.ID, &job.ResourceType, &job.Status, &job.TotalRecords, &job.ProcessedRecords,
		&job.SuccessCount, &job.FailureCount, &job.IdempotencyToken, &metadata, &errorMsg,
		&job.CreatedAt, &job.UpdatedAt, &completedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get import job by token: %w", err)
	}

	if metadata != nil {
		if err := json.Unmarshal(metadata, &job.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	job.CompletedAt = completedAt
	job.ErrorMessage = errorMsg

	return &job, nil
}

// UpdateImportJob updates an existing import job.
func (r *PostgresJobRepository) UpdateImportJob(ctx context.Context, job *domain.ImportJob) error {
	metadata, err := json.Marshal(job.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE import_jobs
		SET status = $2, total_records = $3, processed_records = $4,
			success_count = $5, failure_count = $6, metadata = $7, error_message = $8,
			updated_at = $9, completed_at = $10
		WHERE id = $1
	`, job.ID, job.Status, job.TotalRecords, job.ProcessedRecords,
		job.SuccessCount, job.FailureCount, metadata, job.ErrorMessage,
		job.UpdatedAt, job.CompletedAt)

	if err != nil {
		return fmt.Errorf("update import job: %w", err)
	}

	return nil
}

// CreateExportJob creates a new export job.
func (r *PostgresJobRepository) CreateExportJob(ctx context.Context, job *domain.ExportJob) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO export_jobs (id, resource_type, format, status, total_records,
			file_path, idempotency_token, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, job.ID, job.ResourceType, job.Format, job.Status, job.TotalRecords,
		job.FilePath, job.IdempotencyToken, job.CreatedAt, job.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			strings.Contains(pgErr.ConstraintName, "idempotency_token") {
			existingJob, fetchErr := r.GetExportJobByIdempotencyToken(ctx, job.IdempotencyToken)
			if fetchErr != nil {
				return fmt.Errorf("fetch existing job after race: %w", fetchErr)
			}
			if existingJob != nil {
				*job = *existingJob
				return nil
			}
		}
		return fmt.Errorf("insert export job: %w", err)
	}

	return nil
}

// GetExportJob retrieves an export job by ID.
func (r *PostgresJobRepository) GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error) {
	var job domain.ExportJob
	var completedAt *time.Time
	var errorMsg *string
	var filePath, format *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_type, format, status, total_records,
			file_path, idempotency_token, error_message,
			created_at, updated_at, completed_at
		FROM export_jobs
		WHERE id = $1
	`, id).Scan(&job.ID, &job.ResourceType, &format, &job.Status, &job.TotalRecords,
		&filePath, &job.IdempotencyToken, &errorMsg,
		&job.CreatedAt, &job.UpdatedAt, &completedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get export job: %w", err)
	}

	if filePath != nil {
		job.FilePath = *filePath
	}
	if format != nil {
		job.Format = *format
	}
	job.CompletedAt = completedAt
	job.ErrorMessage = errorMsg

	return &job, nil
}

// GetExportJobByIdempotencyToken retrieves an export job by idempotency token.
func (r *PostgresJobRepository) GetExportJobByIdempotencyToken(ctx context.Context, token string) (*domain.ExportJob, error) {
	var job domain.ExportJob
	var completedAt *time.Time
	var errorMsg *string
	var filePath, format *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, resource_type, format, status, total_records,
			file_path, idempotency_token, error_message,
			created_at, updated_at, completed_at
		FROM export_jobs
		WHERE idempotency_token = $1
	`, token).Scan(&job.ID, &job.ResourceType, &format, &job.Status, &job.TotalRecords,
		&filePath, &job.IdempotencyToken, &errorMsg,
		&job.CreatedAt, &job.UpdatedAt, &completedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get export job by token: %w", err)
	}

	if filePath != nil {
		job.FilePath = *filePath
	}
	if format != nil {
		job.Format = *format
	}
	job.CompletedAt = completedAt
	job.ErrorMessage = errorMsg

	return &job, nil
}

// UpdateExportJob updates an existing export job.
func (r *PostgresJobRepository) UpdateExportJob(ctx context.Context, job *domain.ExportJob) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = $2, total_records = $3, file_path = $4,
			error_message = $5, updated_at = $6, completed_at = $7
		WHERE id = $1
	`, job.ID, job.Status, job.TotalRecords, job.FilePath,
		job.ErrorMessage, job.UpdatedAt, job.CompletedAt)

	if err != nil {
		return fmt.Errorf("update export job: %w", err)
	}

	return nil
}
