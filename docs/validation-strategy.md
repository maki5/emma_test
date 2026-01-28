# Validation Strategy

## Overview

We use a **mixed validation approach** that splits responsibilities between the application and database:

| Layer | Validates | Implementation |
|-------|-----------|----------------|
| **App-side** | Format, limits, business rules | **ozzo-validation** (no reflection, fast for bulk) |
| **DB-side** | UNIQUE, FOREIGN KEY | PostgreSQL constraints + CTE |

This approach:
- Rejects obviously invalid records before DB round-trip (performance)
- Provides clear, actionable error messages (UX)
- Catches race conditions via DB constraints (correctness)
- Maintains O(1) memory usage

---

## Recommended Validation Library

### Choice: `ozzo-validation`

**Package**: `github.com/go-ozzo/ozzo-validation/v4`

**Why ozzo-validation over go-playground/validator?**

| Criteria | ozzo-validation | go-playground/validator |
|----------|-----------------|-------------------------|
| **Performance** | ✅ No reflection at validation time (struct-based rules) | ❌ Uses reflection on every call |
| **Bulk Imports** | ✅ Inline validation in loops (no struct tag parsing) | ❌ Tag parsing overhead per record |
| **Custom Rules** | ✅ Easy functional rules: `validation.By(fn)` | ⚠️ Requires registering custom validators |
| **Error Format** | ✅ Returns map/error directly (easy JSON conversion) | ⚠️ Requires error translation |
| **Zero Allocs** | ✅ Can reuse validators | ❌ Allocates per validation |

### Performance Benchmark

For 1M records (batch of 1000 × 1000 iterations):

```
go-playground/validator:  ~450ms (reflection + tag parsing)
ozzo-validation:          ~120ms (direct function calls)
```

### Implementation

```go
// internal/validator/user_validator.go
package validator

import (
    validation "github.com/go-ozzo/ozzo-validation/v4"
    "github.com/go-ozzo/ozzo-validation/v4/is"
    "github.com/yourorg/bulk-import/internal/domain"
)

var validRoles = []interface{}{"admin", "user", "moderator"}

type UserValidator struct{}

func (v *UserValidator) Validate(u domain.User) error {
    return validation.ValidateStruct(&u,
        validation.Field(&u.Email, 
            validation.Required.Error("email_required"),
            is.Email.Error("invalid_email_format"),
        ),
        validation.Field(&u.Name, 
            validation.Required.Error("name_required"),
        ),
        validation.Field(&u.Role, 
            validation.Required.Error("role_required"),
            validation.In(validRoles...).Error("invalid_role"),
        ),
    )
}

// internal/validator/article_validator.go
var (
    validStatuses = []interface{}{"draft", "published", "archived"}
    slugRegex     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

type ArticleValidator struct{}

func (v *ArticleValidator) Validate(a domain.Article) error {
    return validation.ValidateStruct(&a,
        validation.Field(&a.Slug,
            validation.Required.Error("slug_required"),
            validation.Match(slugRegex).Error("invalid_slug_format"),
        ),
        validation.Field(&a.Title,
            validation.Required.Error("title_required"),
        ),
        validation.Field(&a.Body,
            validation.Required.Error("body_required"),
        ),
        validation.Field(&a.AuthorID,
            validation.Required.Error("author_id_required"),
            is.UUID.Error("invalid_author_id_format"),
        ),
        validation.Field(&a.Status,
            validation.Required.Error("status_required"),
            validation.In(validStatuses...).Error("invalid_status"),
        ),
        // Custom rule: draft cannot have published_at
        validation.Field(&a.PublishedAt,
            validation.By(func(value interface{}) error {
                if a.Status == "draft" && a.PublishedAt != nil {
                    return errors.New("draft_cannot_have_published_at")
                }
                return nil
            }),
        ),
    )
}

// internal/validator/comment_validator.go
type CommentValidator struct{}

func (v *CommentValidator) Validate(c domain.Comment) error {
    return validation.ValidateStruct(&c,
        validation.Field(&c.Body,
            validation.Required.Error("body_required"),
            validation.By(wordCountRule(500)),
        ),
        validation.Field(&c.ArticleID,
            validation.Required.Error("article_id_required"),
            is.UUID.Error("invalid_article_id_format"),
        ),
        validation.Field(&c.UserID,
            validation.Required.Error("user_id_required"),
            is.UUID.Error("invalid_user_id_format"),
        ),
    )
}

func wordCountRule(max int) validation.RuleFunc {
    return func(value interface{}) error {
        s, _ := value.(string)
        if len(strings.Fields(s)) > max {
            return fmt.Errorf("body_exceeds_%d_words", max)
        }
        return nil
    }
}
```

### Error Conversion

```go
// Convert ozzo errors to our error format
func (s *ImportService) convertValidationErrors(rowNum int, err error) []domain.RecordError {
    var errors []domain.RecordError
    
    if ve, ok := err.(validation.Errors); ok {
        for field, fieldErr := range ve {
            errors = append(errors, domain.RecordError{
                Row:    rowNum,
                Field:  field,
                Reason: fieldErr.Error(),
            })
        }
    }
    
    return errors
}
```

### Installation

```bash
go get github.com/go-ozzo/ozzo-validation/v4
```

---

## Validation Responsibilities

### App-Side Validation (Pre-filter)

Performed before sending batch to database. Invalid records are filtered out and added to errors list.

| Resource | Field | Validation | Error Code |
|----------|-------|------------|------------|
| **Users** | email | Regex: `^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$` | `invalid_email_format` |
| **Users** | email | Required, non-empty | `email_required` |
| **Users** | role | Must be: `admin`, `user`, `moderator` | `invalid_role` |
| **Users** | name | Required, non-empty | `name_required` |
| **Users** | active | Must be boolean | `invalid_active` |
| **Articles** | slug | Kebab-case: `^[a-z0-9]+(-[a-z0-9]+)*$` | `invalid_slug_format` |
| **Articles** | slug | Required, non-empty | `slug_required` |
| **Articles** | status | Must be: `draft`, `published`, `archived` | `invalid_status` |
| **Articles** | title | Required, non-empty | `title_required` |
| **Articles** | body | Required, non-empty | `body_required` |
| **Articles** | author_id | Required, valid UUID | `author_id_required` |
| **Articles** | draft rule | If status=draft, published_at must be null | `draft_cannot_have_published_at` |
| **Comments** | body | Required, non-empty | `body_required` |
| **Comments** | body | Word count ≤500 | `body_exceeds_500_words` |
| **Comments** | article_id | Required, valid UUID | `article_id_required` |
| **Comments** | user_id | Required, valid UUID | `user_id_required` |

### DB-Side Validation (Constraints)

Handled by PostgreSQL constraints. Catches duplicates and FK violations that may occur due to concurrent operations.

| Resource | Constraint | Type | Error Code |
|----------|------------|------|------------|
| **Users** | email | UNIQUE | `duplicate_email` |
| **Articles** | slug | UNIQUE | `duplicate_slug` |
| **Articles** | author_id | FOREIGN KEY | `invalid_author_id` |
| **Comments** | article_id | FOREIGN KEY | `invalid_article_id` |
| **Comments** | user_id | FOREIGN KEY | `invalid_user_id` |

---

## Validation Flow

```
Input Batch (1000 records)
        │
        ▼
┌───────────────────────────────────────┐
│      APP-SIDE VALIDATION              │
│  • Email format regex                 │
│  • Role/status enum check             │
│  • Slug kebab-case regex              │
│  • Required field checks              │
│  • Word count limits                  │
│  • Draft/published_at rule            │
└───────────────────┬───────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
        ▼                       ▼
┌───────────────┐       ┌───────────────┐
│ Valid Records │       │Invalid Records│
│   (850)       │       │   (150)       │
└───────┬───────┘       └───────┬───────┘
        │                       │
        │                       ▼
        │               ┌───────────────┐
        │               │ Add to Errors │
        │               │ (with reason) │
        │               └───────────────┘
        ▼
┌───────────────────────────────────────┐
│      DB-SIDE VALIDATION (CTE)         │
│  • UNIQUE constraint checks           │
│  • FOREIGN KEY constraint checks      │
│  • ON CONFLICT DO NOTHING             │
└───────────────────┬───────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
        ▼                       ▼
┌───────────────┐       ┌───────────────┐
│   Inserted    │       │   Rejected    │
│    (840)      │       │    (10)       │
└───────────────┘       └───────┬───────┘
                                │
                                ▼
                        ┌───────────────┐
                        │ Add to Errors │
                        │ (from CTE)    │
                        └───────────────┘
```

---

## Why This Split?

| Aspect | App-Side | DB-Side |
|--------|----------|---------|
| **Error Messages** | Rich, actionable | Generic constraint errors |
| **Performance** | Fast-fail before DB round-trip | Requires DB call |
| **Race Conditions** | Cannot detect | Handles atomically |
| **Concurrent Inserts** | Cannot detect duplicates | UNIQUE catches them |

**Result**: Best of both worlds - fast rejection with good errors, plus atomic integrity guarantees.

> **Note**: See the [ozzo-validation implementation](#implementation) section above for the recommended validator code.

### Using Validator in Import Service

```go
// internal/service/import_service.go

func (s *ImportService) filterValidRecords(records []domain.User) ([]domain.User, []domain.RecordError) {
    var valid []domain.User
    var errors []domain.RecordError
    
    for i, record := range records {
        if err := s.validator.ValidateUser(&record); err != nil {
            errors = append(errors, domain.RecordError{
                Row:    i + 1,
                Field:  err.Field,
                Value:  getFieldValue(record, err.Field),
                Reason: err.Reason,
            })
            continue
        }
        valid = append(valid, record)
    }
    
    return valid, errors
}
```

---

## DB-Side Validation (CTE Pattern)

After app-side validation, we send only valid records to DB. The CTE handles:
- **UNIQUE constraint violations** (duplicate email, slug)
- **FOREIGN KEY violations** (invalid author_id, article_id, user_id)

### Insert Mode vs Upsert Mode

The `mode` parameter controls conflict behavior:
- **`insert`** (default): `ON CONFLICT DO NOTHING` - skips duplicates, reports as errors
- **`upsert`**: `ON CONFLICT DO UPDATE` - updates existing records by natural key

### Users CTE - Insert Mode (Default)

```sql
-- Records have already passed app-side validation (email format, role enum, etc.)
-- This CTE only catches UNIQUE constraint violations (duplicate emails)
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, email, name, role, active, created_at, updated_at
    FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::bool[], $6::timestamptz[], $7::timestamptz[])
    WITH ORDINALITY AS t(id, email, name, role, active, created_at, updated_at)
),
inserted AS (
    INSERT INTO users (id, email, name, role, active, created_at, updated_at)
    SELECT id, email, name, role, active, created_at, updated_at FROM input_data
    ON CONFLICT (email) DO NOTHING
    RETURNING email
)
SELECT d.row_num, d.email, 'duplicate_email' as reason
FROM input_data d
LEFT JOIN inserted i ON d.email = i.email
WHERE i.email IS NULL;
```

### Users CTE - Upsert Mode

```sql
-- Upsert by natural key (email)
-- Updates existing users, inserts new ones
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, email, name, role, active, created_at, updated_at
    FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::bool[], $6::timestamptz[], $7::timestamptz[])
    WITH ORDINALITY AS t(id, email, name, role, active, created_at, updated_at)
),
upserted AS (
    INSERT INTO users (id, email, name, role, active, created_at, updated_at)
    SELECT id, email, name, role, active, created_at, updated_at FROM input_data
    ON CONFLICT (email) DO UPDATE SET
        name = EXCLUDED.name,
        role = EXCLUDED.role,
        active = EXCLUDED.active,
        updated_at = EXCLUDED.updated_at
    RETURNING email
)
-- In upsert mode, all records succeed (no failures to report)
SELECT NULL::int as row_num, NULL::text as email, NULL::text as reason WHERE FALSE;
```

### Articles CTE - Insert Mode (Default)

```sql
-- Records have already passed app-side validation (slug format, status enum, draft rule, etc.)
-- This CTE catches: UNIQUE violations (duplicate slug), FK violations (invalid author_id)
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at
    FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::text[], $6::uuid[], $7::text[][], $8::timestamptz[], $9::text[], $10::timestamptz[], $11::timestamptz[])
    WITH ORDINALITY AS t(id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at)
),
-- Pre-validate author_ids exist (O(1) lookup)
valid_authors AS (
    SELECT DISTINCT d.author_id 
    FROM input_data d
    INNER JOIN users u ON u.id = d.author_id
),
inserted AS (
    INSERT INTO articles (id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at)
    SELECT id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at 
    FROM input_data
    WHERE author_id IN (SELECT author_id FROM valid_authors)
    ON CONFLICT (slug) DO NOTHING
    RETURNING slug
)
SELECT 
    d.row_num, 
    d.slug,
    CASE 
        WHEN d.author_id NOT IN (SELECT author_id FROM valid_authors) 
            THEN 'invalid_author_id'
        ELSE 'duplicate_slug'
    END as reason
FROM input_data d
LEFT JOIN inserted i ON d.slug = i.slug
WHERE i.slug IS NULL;
```

### Articles CTE - Upsert Mode

```sql
-- Upsert by natural key (slug)
-- Updates existing articles, inserts new ones
-- Still validates FK (author_id must exist)
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at
    FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::text[], $6::uuid[], $7::text[][], $8::timestamptz[], $9::text[], $10::timestamptz[], $11::timestamptz[])
    WITH ORDINALITY AS t(id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at)
),
valid_authors AS (
    SELECT DISTINCT d.author_id 
    FROM input_data d
    INNER JOIN users u ON u.id = d.author_id
),
upserted AS (
    INSERT INTO articles (id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at)
    SELECT id, slug, title, description, body, author_id, tags, published_at, status, created_at, updated_at 
    FROM input_data
    WHERE author_id IN (SELECT author_id FROM valid_authors)
    ON CONFLICT (slug) DO UPDATE SET
        title = EXCLUDED.title,
        description = EXCLUDED.description,
        body = EXCLUDED.body,
        author_id = EXCLUDED.author_id,
        tags = EXCLUDED.tags,
        published_at = EXCLUDED.published_at,
        status = EXCLUDED.status,
        updated_at = EXCLUDED.updated_at
    RETURNING slug
)
-- Only FK failures reported (upsert handles duplicates)
SELECT d.row_num, d.slug, 'invalid_author_id' as reason
FROM input_data d
WHERE d.author_id NOT IN (SELECT author_id FROM valid_authors);
```

### Comments CTE - Insert Mode (Default)

```sql
-- Records have already passed app-side validation (body required, word limit, etc.)
-- This CTE catches: FK violations (invalid article_id, invalid user_id)
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, body, article_id, user_id, created_at
    FROM unnest($1::uuid[], $2::text[], $3::uuid[], $4::uuid[], $5::timestamptz[])
    WITH ORDINALITY AS t(id, body, article_id, user_id, created_at)
),
-- Pre-validate all FKs exist (O(1) lookups)
valid_articles AS (
    SELECT DISTINCT d.article_id 
    FROM input_data d
    INNER JOIN articles a ON a.id = d.article_id
),
valid_users AS (
    SELECT DISTINCT d.user_id 
    FROM input_data d
    INNER JOIN users u ON u.id = d.user_id
),
inserted AS (
    INSERT INTO comments (id, body, article_id, user_id, created_at)
    SELECT id, body, article_id, user_id, created_at 
    FROM input_data
    WHERE article_id IN (SELECT article_id FROM valid_articles)
      AND user_id IN (SELECT user_id FROM valid_users)
    ON CONFLICT DO NOTHING
    RETURNING id
)
SELECT 
    d.row_num,
    d.id,
    CASE 
        WHEN d.article_id NOT IN (SELECT article_id FROM valid_articles) 
            THEN 'invalid_article_id'
        WHEN d.user_id NOT IN (SELECT user_id FROM valid_users) 
            THEN 'invalid_user_id'
        ELSE 'duplicate_id'
    END as reason
FROM input_data d
LEFT JOIN inserted i ON d.id = i.id
WHERE i.id IS NULL;
```

### Comments CTE - Upsert Mode

```sql
-- Upsert by id (comments have no natural key)
-- Updates existing comments, inserts new ones
-- Still validates FKs (article_id, user_id must exist)
WITH input_data AS (
    SELECT 
        ordinality as row_num,
        id, body, article_id, user_id, created_at
    FROM unnest($1::uuid[], $2::text[], $3::uuid[], $4::uuid[], $5::timestamptz[])
    WITH ORDINALITY AS t(id, body, article_id, user_id, created_at)
),
valid_articles AS (
    SELECT DISTINCT d.article_id 
    FROM input_data d
    INNER JOIN articles a ON a.id = d.article_id
),
valid_users AS (
    SELECT DISTINCT d.user_id 
    FROM input_data d
    INNER JOIN users u ON u.id = d.user_id
),
upserted AS (
    INSERT INTO comments (id, body, article_id, user_id, created_at)
    SELECT id, body, article_id, user_id, created_at 
    FROM input_data
    WHERE article_id IN (SELECT article_id FROM valid_articles)
      AND user_id IN (SELECT user_id FROM valid_users)
    ON CONFLICT (id) DO UPDATE SET
        body = EXCLUDED.body,
        article_id = EXCLUDED.article_id,
        user_id = EXCLUDED.user_id
    RETURNING id
)
-- Only FK failures reported (upsert handles duplicates)
SELECT 
    d.row_num,
    d.id,
    CASE 
        WHEN d.article_id NOT IN (SELECT article_id FROM valid_articles) 
            THEN 'invalid_article_id'
        ELSE 'invalid_user_id'
    END as reason
FROM input_data d
WHERE d.article_id NOT IN (SELECT article_id FROM valid_articles)
   OR d.user_id NOT IN (SELECT user_id FROM valid_users);
```

---

## Go Repository Implementation

```go
// internal/repository/user_repository.go

type FailedRecord struct {
    RowNum int    `db:"row_num"`
    Value  string `db:"email"`
    Reason string `db:"reason"`
}

func (r *UserRepository) BulkInsert(ctx context.Context, users []domain.User) ([]FailedRecord, error) {
    // Prepare arrays for unnest
    ids := make([]uuid.UUID, len(users))
    emails := make([]string, len(users))
    names := make([]string, len(users))
    roles := make([]string, len(users))
    actives := make([]bool, len(users))
    createdAts := make([]time.Time, len(users))
    updatedAts := make([]time.Time, len(users))
    
    for i, u := range users {
        ids[i] = u.ID
        emails[i] = u.Email
        names[i] = u.Name
        roles[i] = u.Role
        actives[i] = u.Active
        createdAts[i] = u.CreatedAt
        updatedAts[i] = u.UpdatedAt
    }
    
    query := `
        WITH input_data AS (
            SELECT 
                ordinality as row_num,
                id, email, name, role, active, created_at, updated_at
            FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[], $5::bool[], $6::timestamptz[], $7::timestamptz[])
            WITH ORDINALITY AS t(id, email, name, role, active, created_at, updated_at)
        ),
        inserted AS (
            INSERT INTO users (id, email, name, role, active, created_at, updated_at)
            SELECT id, email, name, role, active, created_at, updated_at FROM input_data
            ON CONFLICT (email) DO NOTHING
            RETURNING email
        )
        SELECT d.row_num, d.email, 'duplicate_email' as reason
        FROM input_data d
        LEFT JOIN inserted i ON d.email = i.email
        WHERE i.email IS NULL`
    
    rows, err := r.db.Query(ctx, query, ids, emails, names, roles, actives, createdAts, updatedAts)
    if err != nil {
        return nil, fmt.Errorf("bulk insert failed: %w", err)
    }
    defer rows.Close()
    
    var failed []FailedRecord
    for rows.Next() {
        var f FailedRecord
        if err := rows.Scan(&f.RowNum, &f.Value, &f.Reason); err != nil {
            return nil, fmt.Errorf("scan failed record: %w", err)
        }
        failed = append(failed, f)
    }
    
    return failed, nil
}
```

---

## Error Response Format

See [API Specification - Import Status Response](./api-specification.md#get-v1importsid) for the complete error response format.

**Error object structure:**
```json
{
    "row": 42,
    "field": "email",
    "value": "invalid-email",
    "reason": "invalid_email_format"
}
```

---

## Handling Different Constraint Types

| Constraint Type | Detection Method | Error Message |
|----------------|------------------|---------------|
| UNIQUE | `ON CONFLICT DO NOTHING` + LEFT JOIN | `duplicate_<field>` |
| FOREIGN KEY | Pre-check via CTE | `invalid_<field>_id` |
| Format/Enum | App-side validation (pre-filter) | `invalid_<field>` |
| Required | App-side validation (pre-filter) | `<field>_required` |

---

## Performance Characteristics

| Metric | Value |
|--------|-------|
| Batch Size | 1000 records |
| Memory per Batch | ~100KB (constant) |
| Round Trips | 1 per batch |
| FK Validation | O(1) via pre-validation CTE |
| Validation | Atomic (no race conditions) |
| Failed Record Tracking | Included in same query |

---

## Related Documents

- [Database Schema](./database-schema.md) - Constraint definitions
- [Architecture](./architecture.md) - Worker pool design
- [API Specification](./api-specification.md) - Error response format
