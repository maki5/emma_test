package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"bulk-import-export/internal/domain"
	"bulk-import-export/internal/logger"
)

// PostgresUserRepository implements UserRepository using PostgreSQL.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository creates a new PostgresUserRepository.
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

// BulkInsert inserts users in bulk using CTE with pre-validation.
func (r *PostgresUserRepository) BulkInsert(ctx context.Context, users []domain.User) domain.BatchResult {
	// Pre-allocate errors slice to avoid expensive reallocations
	estimatedErrors := len(users) / 10 // ~10% error rate estimate
	if estimatedErrors < 10 {
		estimatedErrors = 10
	}
	result := domain.BatchResult{
		SuccessCount: 0,
		FailedCount:  0,
		Errors:       make([]domain.RecordError, 0, estimatedErrors),
	}

	if len(users) == 0 {
		return result
	}

	// Check for context cancellation before starting database operations
	if err := ctx.Err(); err != nil {
		result.FailedCount = len(users)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("context cancelled: %v", err),
		})
		return result
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		result.FailedCount = len(users)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("failed to begin transaction: %v", err),
		})
		return result
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query, args := r.buildBulkInsertQuery(users)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		result.FailedCount = len(users)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("bulk insert failed: %v", err),
		})
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var rowNum int
		var success bool
		var errorMsg *string

		if err := rows.Scan(&rowNum, &success, &errorMsg); err != nil {
			logger.Default().Error("Failed to scan bulk insert result row",
				slog.String("repository", "user"),
				slog.String("error", err.Error()))
			result.FailedCount++
			result.Errors = append(result.Errors, domain.RecordError{
				Row:    0,
				Field:  "database",
				Reason: fmt.Sprintf("scan error: %v", err),
			})
			continue
		}

		if success {
			result.SuccessCount++
		} else {
			result.FailedCount++
			reason := "unknown error"
			if errorMsg != nil {
				reason = *errorMsg
			}
			result.Errors = append(result.Errors, domain.RecordError{
				Row:    rowNum,
				Field:  "email",
				Reason: reason,
			})
		}
	}

	if err := rows.Err(); err != nil {
		result.FailedCount = len(users) - result.SuccessCount
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("error reading results: %v", err),
		})
		return result
	}

	if result.SuccessCount > 0 {
		if err := tx.Commit(ctx); err != nil {
			result.FailedCount = len(users)
			result.SuccessCount = 0
			result.Errors = []domain.RecordError{{
				Row:    0,
				Field:  "database",
				Reason: fmt.Sprintf("failed to commit transaction: %v", err),
			}}
			return result
		}
	}

	return result
}

func (r *PostgresUserRepository) buildBulkInsertQuery(users []domain.User) (string, []interface{}) {
	var values []string
	var args []interface{}
	argNum := 1

	for i, u := range users {
		values = append(values, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			argNum, argNum+1, argNum+2, argNum+3, argNum+4, argNum+5))
		// Convert bool and int to string for CTE VALUES clause compatibility
		activeStr := "false"
		if u.Active {
			activeStr = "true"
		}
		args = append(args, u.ID, u.Email, u.Name, u.Role, activeStr, fmt.Sprintf("%d", i+1))
		argNum += 6
	}

	// The query:
	// 1. Collects input data with row numbers
	// 2. Identifies emails that already exist in the DB
	// 3. Identifies in-batch duplicates (keep only first occurrence)
	// 4. Separates valid rows from invalid rows
	// 5. Inserts valid rows and returns WHICH rows were actually inserted
	// 6. Reports success only for rows that were actually inserted
	query := fmt.Sprintf(`
WITH input_data AS (
    SELECT id, email, name, role, is_active, row_num::integer AS row_num 
    FROM (VALUES %s) AS t(id, email, name, role, is_active, row_num)
),
existing_emails AS (
    SELECT email FROM users WHERE email IN (SELECT email FROM input_data)
),
-- Identify in-batch duplicates: for each email, only keep the first row_num
first_occurrence AS (
    SELECT email, MIN(row_num) AS first_row_num
    FROM input_data
    GROUP BY email
),
-- In-batch duplicates are rows that are not the first occurrence
in_batch_duplicates AS (
    SELECT id.row_num, 'duplicate_email_in_batch' AS error_reason
    FROM input_data id
    JOIN first_occurrence fo ON id.email = fo.email
    WHERE id.row_num > fo.first_row_num
),
valid_rows AS (
    SELECT id, email, name, role, is_active, row_num
    FROM input_data
    WHERE email NOT IN (SELECT email FROM existing_emails)
      AND row_num NOT IN (SELECT row_num FROM in_batch_duplicates)
),
invalid_rows AS (
    SELECT row_num, 'duplicate_email' AS error_reason
    FROM input_data
    WHERE email IN (SELECT email FROM existing_emails)
    UNION ALL
    SELECT row_num, error_reason FROM in_batch_duplicates
),
inserted AS (
    INSERT INTO users (id, email, name, role, is_active, created_at, updated_at)
    SELECT id::uuid, email, name, role, is_active::boolean, NOW(), NOW()
    FROM valid_rows
    ON CONFLICT (email) DO NOTHING
    RETURNING email, 1 AS inserted
)
SELECT vr.row_num, 
       CASE WHEN i.email IS NOT NULL THEN true ELSE false END AS success, 
       CASE WHEN i.email IS NULL THEN 'concurrent_duplicate_email'::text ELSE NULL END AS error_message 
FROM valid_rows vr
LEFT JOIN inserted i ON vr.email = i.email
UNION ALL
SELECT row_num, false AS success, error_reason FROM invalid_rows
ORDER BY row_num
`, strings.Join(values, ", "))

	return query, args
}

// StreamAll streams all users for export with O(1) memory.
func (r *PostgresUserRepository) StreamAll(ctx context.Context, callback func(domain.User) error) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, name, role, is_active, created_at, updated_at
		FROM users
		ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}

		if err := callback(u); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("callback error: %w", err)
		}
	}

	return rows.Err()
}
