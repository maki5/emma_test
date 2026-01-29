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

func TestPostgresArticleRepository_BulkInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	userRepo := repository.NewPostgresUserRepository(testDB.Pool)
	articleRepo := repository.NewPostgresArticleRepository(testDB.Pool)
	ctx := context.Background()

	// Helper to create a test user
	createTestUser := func(t *testing.T) domain.User {
		user := domain.User{
			ID:        uuid.New().String(),
			Email:     uuid.New().String() + "@example.com",
			Name:      "Test Author",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result := userRepo.BulkInsert(ctx, []domain.User{user})
		require.Equal(t, 1, result.SuccessCount)
		return user
	}

	t.Run("insert single article successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     "test-article",
				Title:    "Test Article",
				Body:     "This is the article body",
				AuthorID: author.ID,
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("insert article with all fields", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		description := "Article description"
		articles := []domain.Article{
			{
				ID:          uuid.New().String(),
				Slug:        "full-article",
				Title:       "Full Article",
				Description: &description,
				Body:        "Article body content",
				Tags:        []string{"go", "testing", "api"},
				AuthorID:    author.ID,
				Status:      "published",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
	})

	t.Run("insert multiple articles successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		articles := make([]domain.Article, 5)
		for i := 0; i < 5; i++ {
			articles[i] = domain.Article{
				ID:       uuid.New().String(),
				Slug:     uuid.New().String(),
				Title:    "Article " + uuid.New().String(),
				Body:     "Body content",
				AuthorID: author.ID,
				Status:   "draft",
			}
		}

		result := articleRepo.BulkInsert(ctx, articles)

		assert.Equal(t, 5, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
	})

	t.Run("handle duplicate slugs in same batch", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		duplicateSlug := "duplicate-slug"
		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     duplicateSlug,
				Title:    "First Article",
				Body:     "Body",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     duplicateSlug, // Duplicate - should be detected as in-batch duplicate
				Title:    "Second Article",
				Body:     "Body",
				AuthorID: author.ID,
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		// First row succeeds, second row fails with duplicate_slug_in_batch
		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		if len(result.Errors) > 0 {
			assert.Equal(t, "duplicate_slug_in_batch", result.Errors[0].Reason)
			assert.Equal(t, 2, result.Errors[0].Row)
		}

		// Verify only one article was actually inserted
		var count int
		err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM articles").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Only one article should be in the database")
	})

	t.Run("handle invalid author_id (FK violation)", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")

		nonExistentAuthorID := uuid.New().String()
		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     "orphan-article",
				Title:    "Orphan Article",
				Body:     "Body",
				AuthorID: nonExistentAuthorID,
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Reason, "author_not_found")
	})

	t.Run("mixed valid and invalid articles", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     "valid-article",
				Title:    "Valid Article",
				Body:     "Body",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "invalid-author-article",
				Title:    "Invalid Author Article",
				Body:     "Body",
				AuthorID: uuid.New().String(), // Non-existent author
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
	})

	t.Run("empty articles slice", func(t *testing.T) {
		result := articleRepo.BulkInsert(ctx, []domain.Article{})

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("context cancellation", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     "cancelled-article",
				Title:    "Cancelled Article",
				Body:     "Body",
				AuthorID: author.ID,
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(cancelCtx, articles)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
	})
}

func TestPostgresArticleRepository_StreamAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	userRepo := repository.NewPostgresUserRepository(testDB.Pool)
	articleRepo := repository.NewPostgresArticleRepository(testDB.Pool)
	ctx := context.Background()

	// Helper to create a test user
	createTestUser := func(t *testing.T) domain.User {
		user := domain.User{
			ID:        uuid.New().String(),
			Email:     uuid.New().String() + "@example.com",
			Name:      "Test Author",
			Role:      "user",
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		result := userRepo.BulkInsert(ctx, []domain.User{user})
		require.Equal(t, 1, result.SuccessCount)
		return user
	}

	t.Run("stream empty table", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")

		var articles []domain.Article
		err := articleRepo.StreamAll(ctx, func(article domain.Article) error {
			articles = append(articles, article)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, articles)
	})

	t.Run("stream all articles", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		// Insert test articles
		insertArticles := make([]domain.Article, 3)
		for i := 0; i < 3; i++ {
			insertArticles[i] = domain.Article{
				ID:       uuid.New().String(),
				Slug:     uuid.New().String(),
				Title:    "Article " + uuid.New().String(),
				Body:     "Body content",
				AuthorID: author.ID,
				Status:   "draft",
			}
		}
		result := articleRepo.BulkInsert(ctx, insertArticles)
		require.Equal(t, 3, result.SuccessCount)

		// Stream articles
		var streamedArticles []domain.Article
		err := articleRepo.StreamAll(ctx, func(article domain.Article) error {
			streamedArticles = append(streamedArticles, article)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, streamedArticles, 3)

		// Verify fields are populated
		for _, article := range streamedArticles {
			assert.NotEmpty(t, article.ID)
			assert.NotEmpty(t, article.Slug)
			assert.NotEmpty(t, article.Title)
			assert.NotEmpty(t, article.Body)
			assert.NotEmpty(t, article.AuthorID)
			assert.NotEmpty(t, article.Status)
		}
	})

	t.Run("callback error stops streaming", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		// Insert test articles
		insertArticles := make([]domain.Article, 5)
		for i := 0; i < 5; i++ {
			insertArticles[i] = domain.Article{
				ID:       uuid.New().String(),
				Slug:     uuid.New().String(),
				Title:    "Article " + uuid.New().String(),
				Body:     "Body content",
				AuthorID: author.ID,
				Status:   "draft",
			}
		}
		result := articleRepo.BulkInsert(ctx, insertArticles)
		require.Equal(t, 5, result.SuccessCount)

		// Stream with error after 2 articles
		callbackErr := assert.AnError
		var count int
		err := articleRepo.StreamAll(ctx, func(article domain.Article) error {
			count++
			if count >= 2 {
				return callbackErr
			}
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("bulk insert with many duplicates in batch", func(t *testing.T) {
		testDB.TruncateTables(t, "comments", "articles", "users")
		author := createTestUser(t)

		// 8 articles: 4 unique slugs, 4 duplicates
		// Rows: article-a(1), article-b(2), article-c(3), article-d(4), article-a(5), article-b(6), article-c(7), article-d(8)
		// Expected: 4 success (rows 1-4), 4 failures with duplicate_slug_in_batch (rows 5-8)
		articles := []domain.Article{
			{
				ID:       uuid.New().String(),
				Slug:     "article-a",
				Title:    "Article A",
				Body:     "Body A",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-b",
				Title:    "Article B",
				Body:     "Body B",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-c",
				Title:    "Article C",
				Body:     "Body C",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-d",
				Title:    "Article D",
				Body:     "Body D",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-a", // duplicate
				Title:    "Article A Duplicate",
				Body:     "Body A Dup",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-b", // duplicate
				Title:    "Article B Duplicate",
				Body:     "Body B Dup",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-c", // duplicate
				Title:    "Article C Duplicate",
				Body:     "Body C Dup",
				AuthorID: author.ID,
				Status:   "draft",
			},
			{
				ID:       uuid.New().String(),
				Slug:     "article-d", // duplicate
				Title:    "Article D Duplicate",
				Body:     "Body D Dup",
				AuthorID: author.ID,
				Status:   "draft",
			},
		}

		result := articleRepo.BulkInsert(ctx, articles)

		// Verify counts
		assert.Equal(t, 4, result.SuccessCount, "Expected 4 successful inserts")
		assert.Equal(t, 4, result.FailedCount, "Expected 4 failed inserts due to duplicates")
		assert.Len(t, result.Errors, 4, "Expected 4 error entries for duplicates")

		// Verify all errors are duplicate_slug_in_batch
		for _, err := range result.Errors {
			assert.Contains(t, err.Reason, "duplicate_slug_in_batch", "Expected duplicate_slug_in_batch error")
		}

		// Verify only 4 unique articles in DB
		var count int
		err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM articles").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 4, count, "Expected exactly 4 articles in database")

		// Verify correct slugs are in DB
		var slugs []string
		rows, err := testDB.Pool.Query(ctx, "SELECT slug FROM articles ORDER BY slug")
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var slug string
			require.NoError(t, rows.Scan(&slug))
			slugs = append(slugs, slug)
		}
		require.NoError(t, rows.Err())

		expectedSlugs := []string{"article-a", "article-b", "article-c", "article-d"}
		assert.Equal(t, expectedSlugs, slugs, "Expected correct unique slugs in database")
	})
}
