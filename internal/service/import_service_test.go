package service_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/mocks"
	"bulk-import-export/internal/service"
	"bulk-import-export/internal/validator"
)

func TestImportService_StartImport(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new import job successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal when job processing completes
		done := make(chan struct{})

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, "test-token").
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.AnythingOfType("*domain.ImportJob")).
			Return(nil)

		// Track UpdateImportJob calls - signal done when job completes
		updateCount := 0
		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				updateCount++
				// Signal done when we see a terminal status (second update)
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					close(done)
				}
				return nil
			}).
			Maybe()

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100, // batchSize
			1,   // workerCount
		)

		csvData := bytes.NewReader([]byte("email,name,role\ntest@example.com,Test User,user"))

		job, err := svc.StartImport(ctx, "users", "test-token", "users.csv", "req-123", csvData)

		require.NoError(t, err)
		require.NotNil(t, job)
		// Basic checks only - don't read fields that async worker will modify
		assert.NotEmpty(t, job.ID)

		// Wait for async worker to complete with timeout
		select {
		case <-done:
			// Job completed
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}
		svc.Close()
	})

	t.Run("returns existing job for duplicate idempotency token", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		existingJob := &domain.ImportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Status:           domain.JobStatusCompleted,
			IdempotencyToken: "existing-token",
		}

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, "existing-token").
			Return(existingJob, nil)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		csvData := bytes.NewReader([]byte("email,name,role\ntest@example.com,Test User,user"))

		job, err := svc.StartImport(ctx, "users", "existing-token", "users.csv", "req-123", csvData)

		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, existingJob.ID, job.ID)
		assert.Equal(t, domain.JobStatusCompleted, job.Status)
	})

	t.Run("returns error when service is closed", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)

		// Close the service immediately
		svc.Close()

		csvData := bytes.NewReader([]byte("email,name,role\ntest@example.com,Test User,user"))

		_, err := svc.StartImport(ctx, "users", "token", "users.csv", "req-123", csvData)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "shutting down")
	})
}

func TestImportService_GetImportJob(t *testing.T) {
	ctx := context.Background()

	t.Run("returns job when found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		expectedJob := &domain.ImportJob{
			ID:           uuid.New().String(),
			ResourceType: "users",
			Status:       domain.JobStatusCompleted,
		}

		mockJobRepo.EXPECT().
			GetImportJob(mock.Anything, expectedJob.ID).
			Return(expectedJob, nil)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		job, err := svc.GetImportJob(ctx, expectedJob.ID)

		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, expectedJob.ID, job.ID)
	})

	t.Run("returns nil when job not found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		mockJobRepo.EXPECT().
			GetImportJob(mock.Anything, "non-existent").
			Return(nil, nil)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		job, err := svc.GetImportJob(ctx, "non-existent")

		require.NoError(t, err)
		assert.Nil(t, job)
	})
}

func TestImportService_ProcessImport_Users(t *testing.T) {
	t.Run("processes valid CSV users successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		// Expect job status updates - signal done on terminal status
		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2) // processing + completed

		// Expect bulk insert - use mock.MatchedBy to accept any []domain.User
		mockUserRepo.EXPECT().
			BulkInsert(mock.Anything, mock.MatchedBy(func(users []domain.User) bool {
				return len(users) == 2
			})).
			Return(domain.BatchResult{
				SuccessCount: 2,
				FailedCount:  0,
				Errors:       nil,
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)

		csvData := bytes.NewReader([]byte(`id,email,name,role
550e8400-e29b-41d4-a716-446655440001,user1@example.com,User One,user
550e8400-e29b-41d4-a716-446655440002,user2@example.com,User Two,admin`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing to complete with timeout
		select {
		case <-done:
			// Job completed
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		// Assert expectations before closing
		mockJobRepo.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)

		// Close service after assertions
		svc.Close()
	})
}

func TestImportService_ProcessImport_Articles(t *testing.T) {
	t.Run("processes valid NDJSON articles successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		mockArticleRepo.EXPECT().
			BulkInsert(mock.Anything, mock.AnythingOfType("[]domain.Article")).
			Return(domain.BatchResult{
				SuccessCount: 1,
				FailedCount:  0,
				Errors:       nil,
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		ndjsonData := bytes.NewReader([]byte(`{"id":"` + uuid.New().String() + `","slug":"test-article","title":"Test Article","body":"This is the body content here","author_id":"` + uuid.New().String() + `","status":"draft"}`))

		job, err := svc.StartImport(context.Background(), "articles", uuid.New().String(), "articles.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing with timeout
		select {
		case <-done:
			// Job completed
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockArticleRepo.AssertExpectations(t)
	})
}

func TestImportService_ProcessImport_Comments(t *testing.T) {
	t.Run("processes valid NDJSON comments successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		mockCommentRepo.EXPECT().
			BulkInsert(mock.Anything, mock.AnythingOfType("[]domain.Comment")).
			Return(domain.BatchResult{
				SuccessCount: 1,
				FailedCount:  0,
				Errors:       nil,
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		ndjsonData := bytes.NewReader([]byte(`{"id":"cm_` + uuid.New().String() + `","body":"Test comment","article_id":"` + uuid.New().String() + `","user_id":"` + uuid.New().String() + `"}`))

		job, err := svc.StartImport(context.Background(), "comments", uuid.New().String(), "comments.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing with timeout
		select {
		case <-done:
			// Job completed
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockCommentRepo.AssertExpectations(t)
	})
}

func TestImportService_ValidationErrors(t *testing.T) {
	t.Run("fails validation for invalid user data", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		// No BulkInsert call expected - all records fail validation

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		// Invalid email
		csvData := bytes.NewReader([]byte(`email,name,role
invalid-email,User One,user`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing with timeout
		select {
		case <-done:
			// Job completed
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})
}

func TestImportService_ProcessImport_ArticlesWithErrors(t *testing.T) {
	t.Run("handles invalid JSON in articles NDJSON", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		// Invalid JSON
		ndjsonData := bytes.NewReader([]byte(`{"id":"invalid-uuid","slug":"test","title":}`))

		job, err := svc.StartImport(context.Background(), "articles", uuid.New().String(), "articles.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})

	t.Run("handles validation errors in articles", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		// Article with missing required fields
		ndjsonData := bytes.NewReader([]byte(`{"id":"` + uuid.New().String() + `","slug":"","title":"","body":"","author_id":"","status":"draft"}`))

		job, err := svc.StartImport(context.Background(), "articles", uuid.New().String(), "articles.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})
}

func TestImportService_ProcessImport_CommentsWithErrors(t *testing.T) {
	t.Run("handles invalid JSON in comments NDJSON", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		// Invalid JSON
		ndjsonData := bytes.NewReader([]byte(`{"id":"cm_123","body":}`))

		job, err := svc.StartImport(context.Background(), "comments", uuid.New().String(), "comments.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})

	t.Run("handles validation errors in comments", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		// Comment with missing required fields
		ndjsonData := bytes.NewReader([]byte(`{"id":"cm_` + uuid.New().String() + `","body":"","article_id":"","user_id":""}`))

		job, err := svc.StartImport(context.Background(), "comments", uuid.New().String(), "comments.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})
}

func TestImportService_LargeBatchProcessing(t *testing.T) {
	t.Run("processes multiple batches of users", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		// Expect 2 batch inserts (with batchSize=2 and 3 users)
		mockUserRepo.EXPECT().
			BulkInsert(mock.Anything, mock.Anything).
			Return(domain.BatchResult{
				SuccessCount: 2,
				FailedCount:  0,
				Errors:       nil,
			}).Once()

		mockUserRepo.EXPECT().
			BulkInsert(mock.Anything, mock.Anything).
			Return(domain.BatchResult{
				SuccessCount: 1,
				FailedCount:  0,
				Errors:       nil,
			}).Once()

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			2, // Small batch size to trigger multiple batches
			1,
		)
		defer svc.Close()

		// 3 users - should trigger 2 batches
		csvData := bytes.NewReader([]byte(`id,email,name,role
550e8400-e29b-41d4-a716-446655440001,user1@example.com,User One,user
550e8400-e29b-41d4-a716-446655440002,user2@example.com,User Two,admin
550e8400-e29b-41d4-a716-446655440003,user3@example.com,User Three,user`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockUserRepo.AssertExpectations(t)
	})
}

func TestImportService_BulkInsertErrors(t *testing.T) {
	t.Run("handles bulk insert partial failures", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed || job.Status == domain.JobStatusCompletedWithErrors {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		// Return partial failure
		mockUserRepo.EXPECT().
			BulkInsert(mock.Anything, mock.Anything).
			Return(domain.BatchResult{
				SuccessCount: 1,
				FailedCount:  1,
				Errors: []domain.RecordError{
					{Row: 2, Field: "email", Reason: "duplicate email"},
				},
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		csvData := bytes.NewReader([]byte(`id,email,name,role
550e8400-e29b-41d4-a716-446655440001,user1@example.com,User One,user
550e8400-e29b-41d4-a716-446655440002,user2@example.com,User Two,admin`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
	})
}

func TestImportService_StartImport_Errors(t *testing.T) {
	t.Run("returns error when GetImportJobByIdempotencyToken fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, errors.New("database error"))

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		csvData := bytes.NewReader([]byte(`id,email,name,role
550e8400-e29b-41d4-a716-446655440001,user1@example.com,User One,user`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		assert.Error(t, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "idempotency token")
	})

	t.Run("returns error when CreateImportJob fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(errors.New("database connection error"))

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		csvData := bytes.NewReader([]byte(`id,email,name,role
550e8400-e29b-41d4-a716-446655440001,user1@example.com,User One,user`))

		job, err := svc.StartImport(context.Background(), "users", uuid.New().String(), "users.csv", "req-123", csvData)
		assert.Error(t, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "database connection error")
	})
}

func TestImportService_ProcessBatchArticles(t *testing.T) {
	t.Run("processes batch of articles successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed || job.Status == domain.JobStatusCompletedWithErrors {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		mockArticleRepo.EXPECT().
			BulkInsert(mock.Anything, mock.AnythingOfType("[]domain.Article")).
			Return(domain.BatchResult{
				SuccessCount: 2,
				FailedCount:  0,
				Errors:       nil,
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		articleID := uuid.New().String()
		userID := uuid.New().String()
		ndjsonData := bytes.NewReader([]byte(`{"id":"` + articleID + `","slug":"test-article","title":"Test Article","body":"Content here","author_id":"` + userID + `","status":"draft"}
{"id":"` + uuid.New().String() + `","slug":"another-article","title":"Another Article","body":"More content","author_id":"` + userID + `","status":"published"}`))

		job, err := svc.StartImport(context.Background(), "articles", uuid.New().String(), "articles.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockArticleRepo.AssertExpectations(t)
	})
}

func TestImportService_ProcessBatchComments(t *testing.T) {
	t.Run("processes batch of comments successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)
		v := validator.NewValidator()

		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ImportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed || job.Status == domain.JobStatusCompletedWithErrors {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		mockCommentRepo.EXPECT().
			BulkInsert(mock.Anything, mock.AnythingOfType("[]domain.Comment")).
			Return(domain.BatchResult{
				SuccessCount: 2,
				FailedCount:  0,
				Errors:       nil,
			})

		svc := service.NewImportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			v,
			100,
			1,
		)
		defer svc.Close()

		articleID := uuid.New().String()
		userID := uuid.New().String()
		ndjsonData := bytes.NewReader([]byte(`{"id":"cm_` + uuid.New().String() + `","body":"Test comment","article_id":"` + articleID + `","user_id":"` + userID + `"}
{"id":"cm_` + uuid.New().String() + `","body":"Another comment","article_id":"` + articleID + `","user_id":"` + userID + `"}`))

		job, err := svc.StartImport(context.Background(), "comments", uuid.New().String(), "comments.ndjson", "req-123", ndjsonData)
		require.NoError(t, err)
		require.NotNil(t, job)

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockCommentRepo.AssertExpectations(t)
	})
}
