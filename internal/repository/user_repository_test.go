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

func TestPostgresUserRepository_BulkInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	repo := repository.NewPostgresUserRepository(testDB.Pool)
	ctx := context.Background()

	t.Run("insert single user successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		users := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     "test@example.com",
				Name:      "Test User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		result := repo.BulkInsert(ctx, users)

		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("insert multiple users successfully", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		users := make([]domain.User, 10)
		for i := 0; i < 10; i++ {
			users[i] = domain.User{
				ID:        uuid.New().String(),
				Email:     uuid.New().String() + "@example.com",
				Name:      "User " + uuid.New().String(),
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}

		result := repo.BulkInsert(ctx, users)

		assert.Equal(t, 10, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("handle duplicate emails in same batch", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		duplicateEmail := "duplicate@example.com"
		users := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     duplicateEmail,
				Name:      "First User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        uuid.New().String(),
				Email:     duplicateEmail, // Duplicate - should be detected as in-batch duplicate
				Name:      "Second User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		result := repo.BulkInsert(ctx, users)

		// First row succeeds, second row fails with duplicate_email_in_batch
		assert.Equal(t, 1, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		if len(result.Errors) > 0 {
			assert.Equal(t, "duplicate_email_in_batch", result.Errors[0].Reason)
			assert.Equal(t, 2, result.Errors[0].Row)
		}

		// Verify only one user was actually inserted
		var count int
		err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Only one user should be in the database")
	})

	t.Run("handle pre-existing duplicate email", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		existingEmail := "existing@example.com"

		// Insert first user
		firstUsers := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     existingEmail,
				Name:      "Existing User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}
		result := repo.BulkInsert(ctx, firstUsers)
		require.Equal(t, 1, result.SuccessCount)

		// Try to insert user with same email
		duplicateUsers := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     existingEmail, // Already exists
				Name:      "Duplicate User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		result = repo.BulkInsert(ctx, duplicateUsers)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Reason, "duplicate")
	})

	t.Run("empty users slice", func(t *testing.T) {
		result := repo.BulkInsert(ctx, []domain.User{})

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
		assert.Empty(t, result.Errors)
	})

	t.Run("insert users with different roles", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		users := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     "admin@example.com",
				Name:      "Admin User",
				Role:      "admin",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        uuid.New().String(),
				Email:     "moderator@example.com",
				Name:      "Moderator User",
				Role:      "moderator",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        uuid.New().String(),
				Email:     "user@example.com",
				Name:      "Regular User",
				Role:      "user",
				Active:    false,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		result := repo.BulkInsert(ctx, users)

		assert.Equal(t, 3, result.SuccessCount)
		assert.Equal(t, 0, result.FailedCount)
	})

	t.Run("context cancellation", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		users := []domain.User{
			{
				ID:        uuid.New().String(),
				Email:     "cancelled@example.com",
				Name:      "Cancelled User",
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		result := repo.BulkInsert(cancelCtx, users)

		assert.Equal(t, 0, result.SuccessCount)
		assert.Equal(t, 1, result.FailedCount)
		assert.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0].Reason, "cancel")
	})

	t.Run("bulk insert with many duplicates in batch", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		// Create a batch with multiple duplicates:
		// alice@example.com appears 3 times (rows 1, 4, 7)
		// bob@example.com appears 2 times (rows 2, 5)
		// charlie@example.com appears 1 time (row 3)
		// diana@example.com appears 2 times (rows 6, 8)
		users := []domain.User{
			{ID: uuid.New().String(), Email: "alice@example.com", Name: "Alice 1", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},   // Row 1 - first occurrence, should succeed
			{ID: uuid.New().String(), Email: "bob@example.com", Name: "Bob 1", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},       // Row 2 - first occurrence, should succeed
			{ID: uuid.New().String(), Email: "charlie@example.com", Name: "Charlie", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, // Row 3 - unique, should succeed
			{ID: uuid.New().String(), Email: "alice@example.com", Name: "Alice 2", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},   // Row 4 - duplicate of row 1, should fail
			{ID: uuid.New().String(), Email: "bob@example.com", Name: "Bob 2", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},       // Row 5 - duplicate of row 2, should fail
			{ID: uuid.New().String(), Email: "diana@example.com", Name: "Diana 1", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},   // Row 6 - first occurrence, should succeed
			{ID: uuid.New().String(), Email: "alice@example.com", Name: "Alice 3", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},   // Row 7 - duplicate of row 1, should fail
			{ID: uuid.New().String(), Email: "diana@example.com", Name: "Diana 2", Role: "user", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},   // Row 8 - duplicate of row 6, should fail
		}

		result := repo.BulkInsert(ctx, users)

		// Expected: 4 unique emails (alice, bob, charlie, diana) should succeed
		// Expected: 4 duplicates should fail
		assert.Equal(t, 4, result.SuccessCount, "Should have 4 successful inserts (unique emails)")
		assert.Equal(t, 4, result.FailedCount, "Should have 4 failed inserts (duplicates)")
		assert.Len(t, result.Errors, 4, "Should have 4 error records for duplicates")

		// Verify all duplicates are marked as in-batch duplicates
		for _, err := range result.Errors {
			assert.Equal(t, "duplicate_email_in_batch", err.Reason)
			assert.Contains(t, []int{4, 5, 7, 8}, err.Row, "Error should be for rows 4, 5, 7, or 8")
		}

		// Verify only 4 unique users were actually inserted
		var count int
		err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 4, count, "Should have exactly 4 unique users in database")

		// Verify the correct users were inserted
		var emails []string
		rows, err := testDB.Pool.Query(ctx, "SELECT email FROM users ORDER BY email")
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			emails = append(emails, email)
		}
		assert.Equal(t, []string{"alice@example.com", "bob@example.com", "charlie@example.com", "diana@example.com"}, emails)
	})
}

func TestPostgresUserRepository_StreamAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	repo := repository.NewPostgresUserRepository(testDB.Pool)
	ctx := context.Background()

	t.Run("stream empty table", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		var users []domain.User
		err := repo.StreamAll(ctx, func(user domain.User) error {
			users = append(users, user)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, users)
	})

	t.Run("stream all users", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		// Insert test users
		insertUsers := make([]domain.User, 5)
		for i := 0; i < 5; i++ {
			insertUsers[i] = domain.User{
				ID:        uuid.New().String(),
				Email:     uuid.New().String() + "@example.com",
				Name:      "User " + uuid.New().String(),
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
		result := repo.BulkInsert(ctx, insertUsers)
		require.Equal(t, 5, result.SuccessCount)

		// Stream users
		var streamedUsers []domain.User
		err := repo.StreamAll(ctx, func(user domain.User) error {
			streamedUsers = append(streamedUsers, user)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, streamedUsers, 5)

		// Verify fields are populated
		for _, user := range streamedUsers {
			assert.NotEmpty(t, user.ID)
			assert.NotEmpty(t, user.Email)
			assert.NotEmpty(t, user.Name)
			assert.NotEmpty(t, user.Role)
			assert.False(t, user.CreatedAt.IsZero())
			assert.False(t, user.UpdatedAt.IsZero())
		}
	})

	t.Run("callback error stops streaming", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		// Insert test users
		insertUsers := make([]domain.User, 10)
		for i := 0; i < 10; i++ {
			insertUsers[i] = domain.User{
				ID:        uuid.New().String(),
				Email:     uuid.New().String() + "@example.com",
				Name:      "User " + uuid.New().String(),
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
		result := repo.BulkInsert(ctx, insertUsers)
		require.Equal(t, 10, result.SuccessCount)

		// Stream with error after 3 users
		callbackErr := assert.AnError
		var count int
		err := repo.StreamAll(ctx, func(user domain.User) error {
			count++
			if count >= 3 {
				return callbackErr
			}
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("context cancellation", func(t *testing.T) {
		testDB.TruncateTables(t, "users")

		// Insert test users
		insertUsers := make([]domain.User, 5)
		for i := 0; i < 5; i++ {
			insertUsers[i] = domain.User{
				ID:        uuid.New().String(),
				Email:     uuid.New().String() + "@example.com",
				Name:      "User " + uuid.New().String(),
				Role:      "user",
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
		result := repo.BulkInsert(ctx, insertUsers)
		require.Equal(t, 5, result.SuccessCount)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := repo.StreamAll(cancelCtx, func(user domain.User) error {
			return nil
		})

		require.Error(t, err)
	})
}
