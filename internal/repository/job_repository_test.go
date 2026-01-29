package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/repository"
)

func TestPostgresJobRepository_ImportJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	repo := repository.NewPostgresJobRepository(testDB.Pool)
	ctx := context.Background()

	t.Run("create and get import job", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		now := time.Now()
		job := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			TotalRecords:     100,
			ProcessedRecords: 0,
			SuccessCount:     0,
			FailureCount:     0,
			IdempotencyToken: uuid.New().String(),
			Metadata:         map[string]interface{}{"filename": "users.csv"},
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateImportJob(ctx, job)
		require.NoError(t, err)

		// Get by ID
		retrieved, err := repo.GetImportJob(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, job.ID, retrieved.ID)
		assert.Equal(t, job.ResourceType, retrieved.ResourceType)
		assert.Equal(t, job.Status, retrieved.Status)
		assert.Equal(t, job.TotalRecords, retrieved.TotalRecords)
		assert.Equal(t, job.IdempotencyToken, retrieved.IdempotencyToken)
	})

	t.Run("get import job by idempotency token", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		now := time.Now()
		token := uuid.New().String()
		job := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "articles",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateImportJob(ctx, job)
		require.NoError(t, err)

		// Get by token
		retrieved, err := repo.GetImportJobByIdempotencyToken(ctx, token)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, job.ID, retrieved.ID)
		assert.Equal(t, token, retrieved.IdempotencyToken)
	})

	t.Run("get non-existent import job returns nil", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		retrieved, err := repo.GetImportJob(ctx, uuid.New().String())
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("get by non-existent token returns nil", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		retrieved, err := repo.GetImportJobByIdempotencyToken(ctx, "non-existent-token")
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("update import job", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		now := time.Now()
		job := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			TotalRecords:     100,
			ProcessedRecords: 0,
			SuccessCount:     0,
			FailureCount:     0,
			IdempotencyToken: uuid.New().String(),
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateImportJob(ctx, job)
		require.NoError(t, err)

		// Update job
		completedAt := time.Now()
		job.Status = domain.JobStatusCompleted
		job.ProcessedRecords = 100
		job.SuccessCount = 95
		job.FailureCount = 5
		job.UpdatedAt = completedAt
		job.CompletedAt = &completedAt

		err = repo.UpdateImportJob(ctx, job)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetImportJob(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, domain.JobStatusCompleted, retrieved.Status)
		assert.Equal(t, 100, retrieved.ProcessedRecords)
		assert.Equal(t, 95, retrieved.SuccessCount)
		assert.Equal(t, 5, retrieved.FailureCount)
		assert.NotNil(t, retrieved.CompletedAt)
	})

	t.Run("duplicate idempotency token returns existing job", func(t *testing.T) {
		testDB.TruncateTables(t, "import_jobs")

		now := time.Now()
		token := uuid.New().String()

		job1 := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateImportJob(ctx, job1)
		require.NoError(t, err)

		// Try to create with same token
		job2 := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token, // Same token
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err = repo.CreateImportJob(ctx, job2)
		require.NoError(t, err)

		// job2 should now contain job1's data
		assert.Equal(t, job1.ID, job2.ID)
	})
}

func TestPostgresJobRepository_ExportJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	repo := repository.NewPostgresJobRepository(testDB.Pool)
	ctx := context.Background()

	t.Run("create and get export job", func(t *testing.T) {
		testDB.TruncateTables(t, "export_jobs")

		now := time.Now()
		job := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Format:           "csv",
			Status:           domain.JobStatusPending,
			TotalRecords:     0,
			IdempotencyToken: uuid.New().String(),
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateExportJob(ctx, job)
		require.NoError(t, err)

		// Get by ID
		retrieved, err := repo.GetExportJob(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, job.ID, retrieved.ID)
		assert.Equal(t, job.ResourceType, retrieved.ResourceType)
		assert.Equal(t, job.Format, retrieved.Format)
		assert.Equal(t, job.Status, retrieved.Status)
		assert.Equal(t, job.IdempotencyToken, retrieved.IdempotencyToken)
	})

	t.Run("get export job by idempotency token", func(t *testing.T) {
		testDB.TruncateTables(t, "export_jobs")

		now := time.Now()
		token := uuid.New().String()
		job := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "articles",
			Format:           "ndjson",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateExportJob(ctx, job)
		require.NoError(t, err)

		// Get by token
		retrieved, err := repo.GetExportJobByIdempotencyToken(ctx, token)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, job.ID, retrieved.ID)
		assert.Equal(t, token, retrieved.IdempotencyToken)
	})

	t.Run("get non-existent export job returns nil", func(t *testing.T) {
		testDB.TruncateTables(t, "export_jobs")

		retrieved, err := repo.GetExportJob(ctx, uuid.New().String())
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("update export job", func(t *testing.T) {
		testDB.TruncateTables(t, "export_jobs")

		now := time.Now()
		job := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "comments",
			Format:           "csv",
			Status:           domain.JobStatusPending,
			TotalRecords:     0,
			IdempotencyToken: uuid.New().String(),
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateExportJob(ctx, job)
		require.NoError(t, err)

		// Update job
		completedAt := time.Now()
		job.Status = domain.JobStatusCompleted
		job.TotalRecords = 500
		job.FilePath = "/exports/comments_123.csv"
		job.UpdatedAt = completedAt
		job.CompletedAt = &completedAt

		err = repo.UpdateExportJob(ctx, job)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetExportJob(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, domain.JobStatusCompleted, retrieved.Status)
		assert.Equal(t, 500, retrieved.TotalRecords)
		assert.Equal(t, "/exports/comments_123.csv", retrieved.FilePath)
		assert.NotNil(t, retrieved.CompletedAt)
	})

	t.Run("duplicate idempotency token returns existing job", func(t *testing.T) {
		testDB.TruncateTables(t, "export_jobs")

		now := time.Now()
		token := uuid.New().String()

		job1 := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Format:           "csv",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err := repo.CreateExportJob(ctx, job1)
		require.NoError(t, err)

		// Try to create with same token
		job2 := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Format:           "csv",
			Status:           domain.JobStatusPending,
			IdempotencyToken: token, // Same token
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		err = repo.CreateExportJob(ctx, job2)
		require.NoError(t, err)

		// job2 should now contain job1's data
		assert.Equal(t, job1.ID, job2.ID)
	})
}
