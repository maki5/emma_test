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

func TestPostgresCommentRepository_BulkInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	userRepo := repository.NewPostgresUserRepository(testDB.Pool)
	articleRepo := repository.NewPostgresArticleRepository(testDB.Pool)
	commentRepo := repository.NewPostgresCommentRepository(testDB.Pool)
	ctx := context.Background()

	// Helper to create test fixtures
	createTestFixtures := func(t *testing.T) (domain.User, domain.Article) {
		user := domain.User{
			ID:        uuid.New().String(),
			Email:     uuid.New().String() + "@example.com",
			Name:      "Test User",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		userResult := userRepo.BulkInsert(ctx, []domain.User{user})
		require.Equal(t, 1, userResult.SuccessCount)

		article := domain.Article{
			ID:       uuid.New().String(),
			Slug:     uuid.New().String(),
			Title:    "Test Article",
			Body:     "Article body",
			AuthorID: user.ID,
			Status:   "published",
		}
		articleResult := articleRepo.BulkInsert(ctx, []domain.Article{article})
		require.Equal(t, 1, articleResult.SuccessCount)

		return user, article
	}

	t.Run("insert single comment successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		comments := []domain.Comment{
			{
				ID:        uuid.New().String(),
				Body:      "This is a comment",
				ArticleID: article.ID,
				UserID:    user.ID,
			},
		}

		result := commentRepo.BulkInsert(ctx, comments)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("insert multiple comments successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		comments := make([]domain.Comment, 5)
		for i := 0; i < 5; i++ {
			comments[i] = domain.Comment{
				ID:        uuid.New().String(),
				Body:      "Comment " + uuid.New().String(),
				ArticleID: article.ID,
				UserID:    user.ID,
			}
		}

		result := commentRepo.BulkInsert(ctx, comments)

		assert.Equal(t, 5, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
	})

	t.Run("handle invalid article_id (FK violation)", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, _ := createTestFixtures(t)

		nonExistentArticleID := uuid.New().String()
		comments := []domain.Comment{
			{
				ID:        uuid.New().String(),
				Body:      "Orphan comment",
				ArticleID: nonExistentArticleID,
				UserID:    user.ID,
			},
		}

		result := commentRepo.BulkInsert(ctx, comments)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Reason, "article_not_found")
	})

	t.Run("handle invalid user_id (FK violation)", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		_, article := createTestFixtures(t)

		nonExistentUserID := uuid.New().String()
		comments := []domain.Comment{
			{
				ID:        uuid.New().String(),
				Body:      "Comment with invalid user",
				ArticleID: article.ID,
				UserID:    nonExistentUserID,
			},
		}

		result := commentRepo.BulkInsert(ctx, comments)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Reason, "user_not_found")
	})

	t.Run("mixed valid and invalid comments", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		comments := []domain.Comment{
			{
				ID:        uuid.New().String(),
				Body:      "Valid comment",
				ArticleID: article.ID,
				UserID:    user.ID,
			},
			{
				ID:        uuid.New().String(),
				Body:      "Invalid article comment",
				ArticleID: uuid.New().String(), // Non-existent
				UserID:    user.ID,
			},
			{
				ID:        uuid.New().String(),
				Body:      "Invalid user comment",
				ArticleID: article.ID,
				UserID:    uuid.New().String(), // Non-existent
			},
		}

		result := commentRepo.BulkInsert(ctx, comments)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 2, result.FailedCount)
		assert.Len(t, result.Errors, 2)
	})

	t.Run("empty comments slice", func(t *testing.T) {
		result := commentRepo.BulkInsert(ctx, []domain.Comment{})

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("context cancellation", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		comments := []domain.Comment{
			{
				ID:        uuid.New().String(),
				Body:      "Cancelled comment",
				ArticleID: article.ID,
				UserID:    user.ID,
			},
		}

		result := commentRepo.BulkInsert(cancelCtx, comments)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
	})
}

func TestPostgresCommentRepository_StreamAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	userRepo := repository.NewPostgresUserRepository(testDB.Pool)
	articleRepo := repository.NewPostgresArticleRepository(testDB.Pool)
	commentRepo := repository.NewPostgresCommentRepository(testDB.Pool)
	ctx := context.Background()

	// Helper to create test fixtures
	createTestFixtures := func(t *testing.T) (domain.User, domain.Article) {
		user := domain.User{
			ID:        uuid.New().String(),
			Email:     uuid.New().String() + "@example.com",
			Name:      "Test User",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		userResult := userRepo.BulkInsert(ctx, []domain.User{user})
		require.Equal(t, 1, userResult.SuccessCount)

		article := domain.Article{
			ID:       uuid.New().String(),
			Slug:     uuid.New().String(),
			Title:    "Test Article",
			Body:     "Article body",
			AuthorID: user.ID,
			Status:   "published",
		}
		articleResult := articleRepo.BulkInsert(ctx, []domain.Article{article})
		require.Equal(t, 1, articleResult.SuccessCount)

		return user, article
	}

	t.Run("stream empty table", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")

		var comments []domain.Comment
		err := commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			comments = append(comments, comment)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, comments)
	})

	t.Run("stream all comments", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		// Insert test comments
		insertComments := make([]domain.Comment, 3)
		for i := 0; i < 3; i++ {
			insertComments[i] = domain.Comment{
				ID:        uuid.New().String(),
				Body:      "Comment " + uuid.New().String(),
				ArticleID: article.ID,
				UserID:    user.ID,
			}
		}
		result := commentRepo.BulkInsert(ctx, insertComments)
		require.Equal(t, 3, result.SuccessCount)

		// Stream comments
		var streamedComments []domain.Comment
		err := commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			streamedComments = append(streamedComments, comment)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, streamedComments, 3)

		// Verify fields are populated
		for _, comment := range streamedComments {
			assert.NotEmpty(t, comment.ID)
			assert.NotEmpty(t, comment.Body)
			assert.NotEmpty(t, comment.ArticleID)
			assert.NotEmpty(t, comment.UserID)
			assert.False(t, comment.CreatedAt.IsZero())
		}
	})

	t.Run("callback error stops streaming", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		user, article := createTestFixtures(t)

		// Insert test comments
		insertComments := make([]domain.Comment, 5)
		for i := 0; i < 5; i++ {
			insertComments[i] = domain.Comment{
				ID:        uuid.New().String(),
				Body:      "Comment " + uuid.New().String(),
				ArticleID: article.ID,
				UserID:    user.ID,
			}
		}
		result := commentRepo.BulkInsert(ctx, insertComments)
		require.Equal(t, 5, result.SuccessCount)

		// Stream with error after 2 comments
		callbackErr := assert.AnError
		var count int
		err := commentRepo.StreamAll(ctx, func(comment domain.Comment) error {
			count++
			if count >= 2 {
				return callbackErr
			}
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, 2, count)
	})
}
