package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"bulk-import-export/internal/domain"
)

// PostgresArticleRepository implements ArticleRepository using PostgreSQL.
type PostgresArticleRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresArticleRepository creates a new PostgresArticleRepository.
func NewPostgresArticleRepository(pool *pgxpool.Pool) *PostgresArticleRepository {
	return &PostgresArticleRepository{pool: pool}
}

// BulkInsert inserts articles in bulk using CTE with FK validation.
func (r *PostgresArticleRepository) BulkInsert(ctx context.Context, articles []domain.Article) domain.BatchResult {
	// Pre-allocate errors slice to avoid expensive reallocations
	estimatedErrors := len(articles) / 10 // ~10% error rate estimate
	if estimatedErrors < 10 {
		estimatedErrors = 10
	}
	result := domain.BatchResult{
		SuccessCount: 0,
		FailedCount:  0,
		Errors:       make([]domain.RecordError, 0, estimatedErrors),
	}

	if len(articles) == 0 {
		return result
	}

	// Check for context cancellation before starting database operations
	if err := ctx.Err(); err != nil {
		result.FailedCount = len(articles)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("context cancelled: %v", err),
		})
		return result
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		result.FailedCount = len(articles)
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("failed to begin transaction: %v", err),
		})
		return result
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query, args := r.buildBulkInsertQuery(articles)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		result.FailedCount = len(articles)
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
			log.Printf("[article_repository] Failed to scan bulk insert result row: %v", err)
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
				Field:  "article",
				Reason: reason,
			})
		}
	}

	if err := rows.Err(); err != nil {
		result.FailedCount = len(articles) - result.SuccessCount
		result.Errors = append(result.Errors, domain.RecordError{
			Row:    0,
			Field:  "database",
			Reason: fmt.Sprintf("error reading results: %v", err),
		})
		return result
	}

	if result.SuccessCount > 0 {
		if err := tx.Commit(ctx); err != nil {
			result.FailedCount = len(articles)
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

func (r *PostgresArticleRepository) buildBulkInsertQuery(articles []domain.Article) (string, []interface{}) {
	var values []string
	var args []interface{}
	argNum := 1

	for i, a := range articles {
		desc := ""
		if a.Description != nil {
			desc = *a.Description
		}
		// Use JSON encoding for tags to avoid issues with commas in tag values
		tagsJSON := "[]"
		if len(a.Tags) > 0 {
			tagsBytes, _ := json.Marshal(a.Tags)
			tagsJSON = string(tagsBytes)
		}

		values = append(values, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			argNum, argNum+1, argNum+2, argNum+3, argNum+4, argNum+5, argNum+6, argNum+7, argNum+8))
		args = append(args, a.ID, a.Slug, a.Title, desc, a.Body, tagsJSON, a.AuthorID, a.Status, fmt.Sprintf("%d", i+1))
		argNum += 9
	}

	// The query tracks actual insertions via LEFT JOIN to detect concurrent duplicates
	query := fmt.Sprintf(`
WITH input_data AS (
    SELECT id, slug, title, description, body, tags, author_id, status, row_num::integer AS row_num
    FROM (VALUES %s) AS t(id, slug, title, description, body, tags, author_id, status, row_num)
),
valid_authors AS (
    SELECT id FROM users WHERE id IN (SELECT author_id::uuid FROM input_data)
),
existing_slugs AS (
    SELECT slug FROM articles WHERE slug IN (SELECT slug FROM input_data)
),
valid_rows AS (
    SELECT id, slug, title, description, body, tags, author_id, status, row_num
    FROM input_data
    WHERE author_id::uuid IN (SELECT id FROM valid_authors)
      AND slug NOT IN (SELECT slug FROM existing_slugs)
),
invalid_fk AS (
    SELECT row_num, 'author_not_found' AS error_reason
    FROM input_data
    WHERE author_id::uuid NOT IN (SELECT id FROM valid_authors)
),
invalid_slug AS (
    SELECT row_num, 'duplicate_slug' AS error_reason
    FROM input_data
    WHERE slug IN (SELECT slug FROM existing_slugs)
),
inserted AS (
    INSERT INTO articles (id, slug, title, description, body, tags, author_id, status, created_at, updated_at)
    SELECT 
        id::uuid, 
        slug, 
        title, 
        NULLIF(description, ''), 
        body, 
        CASE WHEN tags = '[]' THEN NULL ELSE (SELECT array_agg(elem::text) FROM jsonb_array_elements_text(tags::jsonb) AS elem) END, 
        author_id::uuid, 
        status, 
        NOW(), 
        NOW()
    FROM valid_rows
    ON CONFLICT (slug) DO NOTHING
    RETURNING slug, 1 AS inserted
)
SELECT vr.row_num, 
       CASE WHEN i.slug IS NOT NULL THEN true ELSE false END AS success, 
       CASE WHEN i.slug IS NULL THEN 'concurrent_duplicate_slug'::text ELSE NULL END AS error_message 
FROM valid_rows vr
LEFT JOIN inserted i ON vr.slug = i.slug
UNION ALL
SELECT row_num, false AS success, error_reason FROM invalid_fk
UNION ALL
SELECT row_num, false AS success, error_reason FROM invalid_slug
ORDER BY row_num
`, strings.Join(values, ", "))

	return query, args
}

// StreamAll streams all articles for export with O(1) memory.
func (r *PostgresArticleRepository) StreamAll(ctx context.Context, callback func(domain.Article) error) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, title, description, body, tags, author_id, status, published_at, created_at, updated_at
		FROM articles
		ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("query articles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a domain.Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Description, &a.Body, &a.Tags, &a.AuthorID, &a.Status, &a.PublishedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return fmt.Errorf("scan article: %w", err)
		}

		if err := callback(a); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("callback error: %w", err)
		}
	}

	return rows.Err()
}
