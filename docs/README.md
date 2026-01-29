# Bulk Import/Export System

## Overview

A high-performance bulk data import/export system built with Go and Gin framework. Handles users, articles, and comments with async imports, streaming/async exports, per-record error tracking, and Prometheus metrics.

---

## Documentation

| Document | Description |
|----------|-------------|
| [API Specification](./api-specification.md) | Complete endpoint documentation with examples |

---

## Technology Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.24 |
| Framework | Gin |
| Database | PostgreSQL 15 (pgx driver) |
| Validation | ozzo-validation |
| Metrics | Prometheus + Grafana |
| Testing | mockery + testcontainers |

---

## Architecture

**3-Layer Design:**

| Layer | Components | Purpose |
|-------|------------|---------|
| Handler | Import, Export, Health handlers | HTTP request handling |
| Service | ImportService, ExportService | Business logic, worker pools |
| Repository | User, Article, Comment, Job repos | Database access |

**Key Patterns:**
- Worker pools for controlled concurrency
- Batch processing with per-batch transactions
- CTE-based bulk inserts for conflict handling
- Idempotency via unique tokens
- Streaming exports with database cursors

---

## Database Schema

| Table | Key Constraints |
|-------|-----------------|
| users | email UNIQUE |
| articles | slug UNIQUE, author_id FK → users |
| comments | article_id FK → articles, user_id FK → users |
| import_jobs | idempotency_token UNIQUE |
| export_jobs | idempotency_token UNIQUE |

**Import order:** users → articles → comments (respects FK dependencies)

---

## Validation

| Layer | Validates | Purpose |
|-------|-----------|---------|
| App-side | Format, limits, enums | Fast-fail with rich errors |
| DB-side | UNIQUE, FOREIGN KEY | Race condition safety |

---

## Metrics & Monitoring

| Metric Type | Examples |
|-------------|----------|
| HTTP | Request count, latency (P50/P90/P99), in-flight |
| Jobs | Completion rate, duration, in-progress count |
| Records | Processed count, success/failure rate |
| Database | Connection pool utilization |

**Access:** Prometheus at `:9090`, Grafana at `:3000` with pre-configured dashboards.

---

## Testing

| Level | Tool | Scope |
|-------|------|-------|
| Unit | mockery | Handler, Service layer |
| Integration | testcontainers | Repository with real PostgreSQL |
| E2E | curl scripts | Full API flows |

---

## Quick Start

1. Start PostgreSQL: `make db-start`
2. Run migrations: `make migrate-up`
3. Start app: `make run`
4. Access API at http://localhost:8080

For monitoring: `make monitoring-start` (Prometheus + Grafana)

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness probe |
| GET | `/live` | Liveness probe |
| GET | `/metrics` | Prometheus metrics |
| POST | `/api/v1/imports` | Start import job |
| GET | `/api/v1/imports/:id` | Get import status |
| GET | `/api/v1/exports/:resource/stream` | Stream export |
| POST | `/api/v1/exports` | Start async export |
| GET | `/api/v1/exports/:id` | Get export status |

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| HTTP_PORT | 8080 | Server port |
| DATABASE_URL | required | PostgreSQL URL |
| WORKER_POOL_SIZE | 10 | Concurrent workers |
| BATCH_SIZE | 1000 | Records per batch |
| EXPORT_DIR | ./exports | Export file directory |
