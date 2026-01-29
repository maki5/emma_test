package service_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
)

func TestExportService_StartExport(t *testing.T) {
	ctx := context.Background()

	// Create temp dir for exports
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("creates new export job successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, "test-token").
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.AnythingOfType("*domain.ExportJob")).
			Return(nil)

		// Track UpdateExportJob calls - signal done on terminal status
		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Maybe()
		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Return(nil).
			Maybe()

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1, // workerCount
		)
		require.NoError(t, err)

		job, err := svc.StartExport(ctx, "users", "csv", "test-token", "req-123")

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

		existingJob := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Format:           "csv",
			Status:           domain.JobStatusCompleted,
			IdempotencyToken: "existing-token",
		}

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, "existing-token").
			Return(existingJob, nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(ctx, "users", "csv", "existing-token", "req-123")

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

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)

		// Close the service immediately
		svc.Close()

		_, err = svc.StartExport(ctx, "users", "csv", "token", "req-123")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "shutting down")
	})
}

func TestExportService_GetExportJob(t *testing.T) {
	ctx := context.Background()

	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("returns job when found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		expectedJob := &domain.ExportJob{
			ID:           uuid.New().String(),
			ResourceType: "users",
			Format:       "csv",
			Status:       domain.JobStatusCompleted,
		}

		mockJobRepo.EXPECT().
			GetExportJob(mock.Anything, expectedJob.ID).
			Return(expectedJob, nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.GetExportJob(ctx, expectedJob.ID)

		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, expectedJob.ID, job.ID)
	})

	t.Run("returns nil when job not found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockJobRepo.EXPECT().
			GetExportJob(mock.Anything, "non-existent").
			Return(nil, nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.GetExportJob(ctx, "non-existent")

		require.NoError(t, err)
		assert.Nil(t, job)
	})

	t.Run("returns error when repository fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockJobRepo.EXPECT().
			GetExportJob(mock.Anything, "some-id").
			Return(nil, errors.New("database error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.GetExportJob(ctx, "some-id")

		assert.Error(t, err)
		assert.Nil(t, job)
	})
}

func TestExportService_ProcessExport_Users(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("exports users to CSV successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		// Expect job status updates - signal done on terminal status
		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2) // processing + completed

		// Simulate streaming users
		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.User) error) {
				now := time.Now()
				users := []domain.User{
					{ID: uuid.New().String(), Email: "user1@example.com", Name: "User One", Role: "user", Active: true, CreatedAt: now, UpdatedAt: now},
					{ID: uuid.New().String(), Email: "user2@example.com", Name: "User Two", Role: "admin", Active: true, CreatedAt: now, UpdatedAt: now},
				}
				for _, u := range users {
					if err := callback(u); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "users", "csv", uuid.New().String(), "req-123")
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
		mockUserRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "users_*.csv"))
		assert.GreaterOrEqual(t, len(files), 1)
	})

	t.Run("exports users to NDJSON successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.User) error) {
				now := time.Now()
				user := domain.User{ID: uuid.New().String(), Email: "user@example.com", Name: "User", Role: "user", Active: true, CreatedAt: now, UpdatedAt: now}
				callback(user)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "users", "ndjson", uuid.New().String(), "req-123")
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
		mockUserRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "users_*.ndjson"))
		assert.GreaterOrEqual(t, len(files), 1)
	})
}

func TestExportService_StreamUsers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("streams users to writer in CSV format", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.User) error) {
				users := []domain.User{
					{ID: "user-1", Email: "user1@example.com", Name: "User One", Role: "user", Active: true, CreatedAt: now, UpdatedAt: now},
					{ID: "user-2", Email: "user2@example.com", Name: "User Two", Role: "admin", Active: false, CreatedAt: now, UpdatedAt: now},
				}
				for _, u := range users {
					if err := callback(u); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamUsers(context.Background(), "csv", writer)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.Contains(t, buf.String(), "id,email,name,role")
		assert.Contains(t, buf.String(), "user1@example.com")
		assert.Contains(t, buf.String(), "user2@example.com")
	})

	t.Run("streams users to writer in NDJSON format", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.User) error) {
				user := domain.User{ID: "user-1", Email: "user@example.com", Name: "User", Role: "user", Active: true, CreatedAt: now, UpdatedAt: now}
				callback(user)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamUsers(context.Background(), "ndjson", writer)

		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Contains(t, buf.String(), `"email":"user@example.com"`)
	})

	t.Run("returns error when repository fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockUserRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Return(errors.New("database connection error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamUsers(context.Background(), "csv", writer)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "database connection error")
	})
}

func TestExportService_StreamArticles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("streams articles to writer in NDJSON format", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockArticleRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Article) error) {
				article := domain.Article{
					ID:        "article-1",
					Slug:      "test-article",
					Title:     "Test Article",
					Body:      "Body content",
					AuthorID:  "author-1",
					Status:    "published",
					CreatedAt: now,
					UpdatedAt: now,
				}
				callback(article)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamArticles(context.Background(), "ndjson", writer)

		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Contains(t, buf.String(), `"slug":"test-article"`)
	})

	t.Run("streams articles to writer in CSV format", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockArticleRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Article) error) {
				articles := []domain.Article{
					{ID: "article-1", Slug: "article-one", Title: "Article One", Body: "Body 1", AuthorID: "author-1", Status: "published", CreatedAt: now, UpdatedAt: now},
					{ID: "article-2", Slug: "article-two", Title: "Article Two", Body: "Body 2", AuthorID: "author-2", Status: "draft", CreatedAt: now, UpdatedAt: now},
				}
				for _, a := range articles {
					if err := callback(a); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamArticles(context.Background(), "csv", writer)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
		// Verify CSV contains article data
		assert.Contains(t, buf.String(), "article-one")
		assert.Contains(t, buf.String(), "article-two")
	})

	t.Run("returns error when repository fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockArticleRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Return(errors.New("database error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamArticles(context.Background(), "csv", writer)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestExportService_StreamComments(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("streams comments to writer with cm_ prefix", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockCommentRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Comment) error) {
				comment := domain.Comment{
					ID:        "comment-1",
					Body:      "Test comment",
					ArticleID: "article-1",
					UserID:    "user-1",
					CreatedAt: now,
				}
				callback(comment)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamComments(context.Background(), "ndjson", writer)

		require.NoError(t, err)
		assert.Equal(t, 1, count)
		// Verify cm_ prefix is added
		assert.Contains(t, buf.String(), `"id":"cm_comment-1"`)
	})

	t.Run("streams comments to CSV format", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		now := time.Now()
		mockCommentRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Comment) error) {
				comments := []domain.Comment{
					{ID: "comment-1", Body: "First comment", ArticleID: "article-1", UserID: "user-1", CreatedAt: now},
					{ID: "comment-2", Body: "Second comment", ArticleID: "article-1", UserID: "user-2", CreatedAt: now},
				}
				for _, c := range comments {
					if err := callback(c); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamComments(context.Background(), "csv", writer)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
		// Verify CSV format with cm_ prefix
		assert.Contains(t, buf.String(), "cm_comment-1")
		assert.Contains(t, buf.String(), "cm_comment-2")
	})

	t.Run("returns error when repository fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockCommentRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Return(errors.New("database error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		var buf bytes.Buffer
		writer := &testStreamWriter{buf: &buf}

		count, err := svc.StreamComments(context.Background(), "csv", writer)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestExportService_ProcessExport_Articles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("exports articles to NDJSON successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		now := time.Now()
		mockArticleRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Article) error) {
				articles := []domain.Article{
					{ID: uuid.New().String(), Slug: "article-1", Title: "Article One", Body: "Body one", AuthorID: uuid.New().String(), Status: "published", CreatedAt: now, UpdatedAt: now},
					{ID: uuid.New().String(), Slug: "article-2", Title: "Article Two", Body: "Body two", AuthorID: uuid.New().String(), Status: "draft", CreatedAt: now, UpdatedAt: now},
				}
				for _, a := range articles {
					if err := callback(a); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "articles", "ndjson", uuid.New().String(), "req-123")
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockArticleRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "articles_*.ndjson"))
		assert.GreaterOrEqual(t, len(files), 1)
	})

	t.Run("exports articles to CSV successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		now := time.Now()
		mockArticleRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Article) error) {
				article := domain.Article{ID: uuid.New().String(), Slug: "test-article", Title: "Test", Body: "Body", AuthorID: uuid.New().String(), Status: "published", CreatedAt: now, UpdatedAt: now}
				callback(article)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "articles", "csv", uuid.New().String(), "req-123")
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "articles_*.csv"))
		assert.GreaterOrEqual(t, len(files), 1)
	})
}

func TestExportService_ProcessExport_Comments(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("exports comments to NDJSON successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		now := time.Now()
		mockCommentRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Comment) error) {
				comments := []domain.Comment{
					{ID: "comment-1", Body: "Comment one", ArticleID: uuid.New().String(), UserID: uuid.New().String(), CreatedAt: now},
					{ID: "comment-2", Body: "Comment two", ArticleID: uuid.New().String(), UserID: uuid.New().String(), CreatedAt: now},
				}
				for _, c := range comments {
					if err := callback(c); err != nil {
						return
					}
				}
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "comments", "ndjson", uuid.New().String(), "req-123")
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)
		mockCommentRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "comments_*.ndjson"))
		assert.GreaterOrEqual(t, len(files), 1)
	})

	t.Run("exports comments to CSV successfully", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		// Channel to signal job completion
		done := make(chan struct{})
		var once sync.Once

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(nil)

		mockJobRepo.EXPECT().
			UpdateExportJob(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, job *domain.ExportJob) error {
				if job.Status == domain.JobStatusCompleted || job.Status == domain.JobStatusFailed {
					once.Do(func() { close(done) })
				}
				return nil
			}).
			Times(2)

		now := time.Now()
		mockCommentRepo.EXPECT().
			StreamAll(mock.Anything, mock.Anything).
			Run(func(ctx context.Context, callback func(domain.Comment) error) {
				comment := domain.Comment{ID: "comment-1", Body: "Test comment", ArticleID: uuid.New().String(), UserID: uuid.New().String(), CreatedAt: now}
				callback(comment)
			}).
			Return(nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "comments", "csv", uuid.New().String(), "req-123")
		require.NoError(t, err)
		require.NotNil(t, job)

		// Wait for async processing
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for job completion")
		}

		mockJobRepo.AssertExpectations(t)

		// Verify file was created
		files, _ := filepath.Glob(filepath.Join(tempDir, "comments_*.csv"))
		assert.GreaterOrEqual(t, len(files), 1)
	})
}

func TestExportService_StartExport_Errors(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("returns existing job if idempotency token matches", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		existingJob := &domain.ExportJob{
			ID:               uuid.New().String(),
			ResourceType:     "users",
			Format:           "csv",
			Status:           domain.JobStatusCompleted,
			IdempotencyToken: "token-123",
		}

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, "token-123").
			Return(existingJob, nil)

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "users", "csv", "token-123", "req-123")
		require.NoError(t, err)
		assert.Equal(t, existingJob.ID, job.ID)
	})

	t.Run("returns error when CreateExportJob fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, nil)

		mockJobRepo.EXPECT().
			CreateExportJob(mock.Anything, mock.Anything).
			Return(errors.New("database connection error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "users", "csv", uuid.New().String(), "req-123")
		assert.Error(t, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "database connection error")
	})

	t.Run("returns error when GetExportJobByIdempotencyToken fails", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepository(t)
		mockArticleRepo := mocks.NewMockArticleRepository(t)
		mockCommentRepo := mocks.NewMockCommentRepository(t)
		mockJobRepo := mocks.NewMockJobRepository(t)

		mockJobRepo.EXPECT().
			GetExportJobByIdempotencyToken(mock.Anything, mock.Anything).
			Return(nil, errors.New("database error"))

		svc, err := service.NewExportService(
			mockUserRepo,
			mockArticleRepo,
			mockCommentRepo,
			mockJobRepo,
			tempDir,
			1,
		)
		require.NoError(t, err)
		defer svc.Close()

		job, err := svc.StartExport(context.Background(), "users", "csv", uuid.New().String(), "req-123")
		assert.Error(t, err)
		assert.Nil(t, job)
		assert.Contains(t, err.Error(), "idempotency token")
	})
}

// testStreamWriter implements StreamWriter for testing
type testStreamWriter struct {
	buf *bytes.Buffer
}

func (w *testStreamWriter) Write(data []byte) error {
	_, err := w.buf.Write(data)
	return err
}

func (w *testStreamWriter) Flush() {}
