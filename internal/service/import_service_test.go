package service_test

import (
	"bytes"
	"context"
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

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, "test-token").
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.AnythingOfType("*domain.ImportJob")).
			Return(nil)

		// Allow async worker calls (may or may not happen before Close)
		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			Return(nil).
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

		// Wait for async worker to complete before closing
		time.Sleep(500 * time.Millisecond)
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

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		// Expect job status updates
		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			Return(nil).
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

		// Wait for async processing to complete
		time.Sleep(500 * time.Millisecond)

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

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			Return(nil).
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

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)

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

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			Return(nil).
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

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)

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

		mockJobRepo.EXPECT().
			GetImportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateImportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateImportJob(mock.Anything, mock.Anything).
			Return(nil).
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

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)

		mockJobRepo.AssertExpectations(t)
	})
}
