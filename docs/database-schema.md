# Database Schema

## Overview

The database uses PostgreSQL 15 with constraints for data validation. Failed records are tracked directly via CTE queries that identify which rows were rejected by constraints.

---

## Entity Relationship Diagram

```
┌───────────────────┐       ┌───────────────────┐
│      users        │       │   import_jobs     │
├───────────────────┤       ├───────────────────┤
│ id (PK)           │       │ id (PK)           │
│ email (UNIQUE)    │       │ resource_type     │
│ name              │       │ status            │
│ role              │       │ total_records     │
│ active            │       │ processed_records │
│ created_at        │       │ successful_records│
│ updated_at        │       │ error_records     │
└─────────┬─────────┘       │ errors (JSONB)    │
          │                 │ failed_batches    │
          │                 │ failure_reason    │
          │ author_id       │ idempotency_key   │
          │                 │ started_at        │
          ▼                 │ completed_at      │
┌───────────────────┐       └───────────────────┘
│     articles      │       ┌───────────────────┐
├───────────────────┤       ├───────────────────┤
│ id (PK)           │       │ id (PK)           │
│ slug (UNIQUE)     │       │ resource_type     │
│ title             │       │ format            │
│ description       │       │ status            │
│ body              │       │ filters (JSONB)   │
│ author_id (FK)────┼───────│ file_path         │
│ tags (TEXT[])     │       │ download_url      │
│ published_at      │       │ started_at        │
│ status            │       │ completed_at      │
│ created_at        │       └───────────────────┘
│ updated_at        │
└─────────┬─────────┘
          │ article_id
          ▼
┌───────────────────┐
│     comments      │
├───────────────────┤
│ id (PK)           │
│ body              │
│ article_id (FK)───┼─── Must reference existing article
│ user_id (FK)──────┼─── Must reference existing user
│ created_at        │
└───────────────────┘
```

---

## Table Definitions

See [Migration Files](#migration-files) below for complete CREATE TABLE statements with all constraints and indexes.

**Quick Reference:**

| Table | Primary Key | Unique Constraints | Foreign Keys |
|-------|-------------|-------------------|---------------|
| users | id (UUID) | email | - |
| articles | id (UUID) | slug | author_id → users |
| comments | id (UUID) | - | article_id → articles, user_id → users |
| import_jobs | id (UUID) | idempotency_key | - |
| export_jobs | id (UUID) | idempotency_key | - |

---

## Validation Constraints Summary

> **Note**: Format/limit validations (email regex, role enum, slug format, word limits) are performed **app-side** for better error messages and fast-fail. Only UNIQUE and FK constraints remain in DB to handle race conditions.

| Table | Constraint | Type | Purpose |
|-------|------------|------|---------|  
| users | `users_email_unique` | UNIQUE | Prevent duplicate emails |
| articles | `articles_slug_unique` | UNIQUE | Prevent duplicate slugs |
| articles | `articles_author_fk` | FOREIGN KEY | Ensure author exists |
| comments | `comments_article_fk` | FOREIGN KEY | Ensure article exists |
| comments | `comments_user_fk` | FOREIGN KEY | Ensure user exists |

## Migration Files

### Up Migration

```sql
-- migrations/001_create_tables.up.sql

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_email_unique UNIQUE (email)
    -- NOTE: email format and role validation done app-side for better error messages
);

-- Articles table
CREATE TABLE articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    body TEXT NOT NULL,
    author_id UUID NOT NULL,
    tags TEXT[] DEFAULT '{}',
    published_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT articles_slug_unique UNIQUE (slug),
    CONSTRAINT articles_author_fk FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
    -- NOTE: slug format, status enum, draft rule, title/body checks done app-side
);

-- Comments table
CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    body TEXT NOT NULL,
    article_id UUID NOT NULL,
    user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT comments_article_fk FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
    CONSTRAINT comments_user_fk FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    -- NOTE: body required and word limit checks done app-side
);

-- Import jobs table
CREATE TABLE import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    total_records INTEGER NOT NULL DEFAULT 0,
    processed_records INTEGER NOT NULL DEFAULT 0,
    successful_records INTEGER NOT NULL DEFAULT 0,
    error_records INTEGER NOT NULL DEFAULT 0,
    errors JSONB DEFAULT '[]'::jsonb,       -- Per-record validation errors
    failed_batches JSONB DEFAULT '[]'::jsonb, -- Array of {batch_number, records_in_batch, error}
    failure_reason TEXT,                    -- Why the import failed (if status=failed)
    idempotency_key VARCHAR(255),
    file_path TEXT,
    file_url TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT import_jobs_status_valid CHECK (status IN ('pending', 'processing', 'completed', 'completed_with_errors', 'failed', 'cancelled')),
    CONSTRAINT import_jobs_resource_valid CHECK (resource_type IN ('users', 'articles', 'comments')),
    CONSTRAINT import_jobs_idempotency_unique UNIQUE (idempotency_key)
);

-- Export jobs table
CREATE TABLE export_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(255),           -- For safe retries (optional)
    resource_type VARCHAR(50) NOT NULL,
    format VARCHAR(20) NOT NULL DEFAULT 'ndjson',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    filters JSONB DEFAULT '{}'::jsonb,
    fields TEXT[],
    file_path TEXT,
    file_size_bytes BIGINT,                 -- Size of generated file
    download_url TEXT,
    record_count INTEGER,
    failure_reason TEXT,                    -- Why the export failed (if status=failed)
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT export_jobs_status_valid CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    CONSTRAINT export_jobs_resource_valid CHECK (resource_type IN ('users', 'articles', 'comments')),
    CONSTRAINT export_jobs_format_valid CHECK (format IN ('ndjson', 'csv', 'json')),
    CONSTRAINT export_jobs_idempotency_unique UNIQUE (idempotency_key)
);

-- Create indexes
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_articles_slug ON articles(slug);
CREATE INDEX idx_articles_author ON articles(author_id);
CREATE INDEX idx_articles_status ON articles(status);
CREATE INDEX idx_articles_published ON articles(published_at) WHERE published_at IS NOT NULL;
CREATE INDEX idx_comments_article ON comments(article_id);
CREATE INDEX idx_comments_user ON comments(user_id);
CREATE INDEX idx_import_jobs_status ON import_jobs(status);
CREATE INDEX idx_import_jobs_idempotency ON import_jobs(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_import_jobs_created ON import_jobs(created_at DESC);  -- For job listing
CREATE INDEX idx_export_jobs_status ON export_jobs(status);
CREATE INDEX idx_export_jobs_idempotency ON export_jobs(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_export_jobs_created ON export_jobs(created_at DESC);  -- For job listing
```

### Down Migration

```sql
-- migrations/001_create_tables.down.sql
DROP TABLE IF EXISTS comments CASCADE;
DROP TABLE IF EXISTS articles CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS import_jobs CASCADE;
DROP TABLE IF EXISTS export_jobs CASCADE;
```

---

## Related Documents

- [Validation Strategy](./validation-strategy.md) - How CTE queries identify failed records
- [Architecture](./architecture.md) - System design overview
