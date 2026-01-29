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

// PostgresCommentRepository implements CommentRepository using PostgreSQL.
type PostgresCommentRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresCommentRepository creates a new PostgresCommentRepository.
func NewPostgresCommentRepository(pool *pgxpool.Pool) *PostgresCommentRepository {
	return &PostgresCommentRepository{pool: pool}
}

// BulkInsert inserts comments in bulk using CTE with FK validation.
func (r *PostgresCommentRepository) BulkInsert(ctx context.Context, comments []domain.Comment) domain.BatchResult {
	// Pre-allocate errors slice to avoid expensive reallocations
	estimatedErrors := len(comments) / 10 // ~10% error rate estimate
	if estimatedErrors < 10 {
		estimatedErrors = 10
	}
	result := domain.BatchResult{
		SuccessCount: 0,
		FailedCount:  0,
		Errors:       make([]domain.RecordError, 0, estimatedErrors),
	}

	if len(comments) == 0 {
		return result
	}

	// Check for context cancellation before starting database operations
	if err := ctx.Err(); err != nil {
		result.FailedCount = len(comments)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("context cancelled: %v", err),
		})
		return result
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		result.FailedCount = len(comments)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("failed to begin transaction: %v", err),
		})
		return result
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query, args := r.buildBulkInsertQuery(comments)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		result.FailedCount = len(comments)
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
				slog.String("repository", "comment"),
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
				Field:  "comment",
				Reason: reason,
			})
		}
	}

	if err := rows.Err(); err != nil {
		result.FailedCount = len(comments) - result.SuccessCount
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("error reading results: %v", err),
		})
		return result
	}

	if result.SuccessCount > 0 {
		if err := tx.Commit(ctx); err != nil {
			result.FailedCount = len(comments)
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

func (r *PostgresCommentRepository) buildBulkInsertQuery(comments []domain.Comment) (string, []interface{}) {
	var values []string
	var args []interface{}
	argNum := 1

	for i, c := range comments {
		values = append(values, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)",
			argNum, argNum+1, argNum+2, argNum+3, argNum+4))
		args = append(args, c.ID, c.Body, c.ArticleID, c.UserID, fmt.Sprintf("%d", i+1))
		argNum += 5
	}

	// The query tracks actual insertions via LEFT JOIN to detect concurrent issues
	query := fmt.Sprintf(`
WITH input_data AS (
    SELECT id, body, article_id, user_id, row_num::integer AS row_num
    FROM (VALUES %s) AS t(id, body, article_id, user_id, row_num)
),
valid_articles AS (
    SELECT id FROM articles WHERE id IN (SELECT article_id::uuid FROM input_data)
),
valid_users AS (
    SELECT id FROM users WHERE id IN (SELECT user_id::uuid FROM input_data)
),
valid_rows AS (
    SELECT id, body, article_id, user_id, row_num
    FROM input_data
    WHERE article_id::uuid IN (SELECT id FROM valid_articles)
      AND user_id::uuid IN (SELECT id FROM valid_users)
),
invalid_article AS (
    SELECT row_num, 'article_not_found' AS error_reason
    FROM input_data
    WHERE article_id::uuid NOT IN (SELECT id FROM valid_articles)
),
invalid_user AS (
    SELECT row_num, 'user_not_found' AS error_reason
    FROM input_data
    WHERE user_id::uuid NOT IN (SELECT id FROM valid_users)
      AND article_id::uuid IN (SELECT id FROM valid_articles)
),
inserted AS (
    INSERT INTO comments (id, body, article_id, user_id, created_at)
    SELECT id::uuid, body, article_id::uuid, user_id::uuid, NOW()
    FROM valid_rows
    RETURNING id, 1 AS inserted
)
SELECT vr.row_num, 
       CASE WHEN i.id IS NOT NULL THEN true ELSE false END AS success, 
       CASE WHEN i.id IS NULL THEN 'insert_failed'::text ELSE NULL END AS error_message 
FROM valid_rows vr
LEFT JOIN inserted i ON vr.id::uuid = i.id
UNION ALL
SELECT row_num, false AS success, error_reason FROM invalid_article
UNION ALL
SELECT row_num, false AS success, error_reason FROM invalid_user
ORDER BY row_num
`, strings.Join(values, ", "))

	return query, args
}

// StreamAll streams all comments for export with O(1) memory.
func (r *PostgresCommentRepository) StreamAll(ctx context.Context, callback func(domain.Comment) error) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, body, article_id, user_id, created_at
		FROM comments
		ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.Body, &c.ArticleID, &c.UserID, &c.CreatedAt); err != nil {
			return fmt.Errorf("scan comment: %w", err)
		}

		if err := callback(c); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("callback error: %w", err)
		}
	}

	return rows.Err()
}
