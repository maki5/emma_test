# Bulk Import/Export System - Implementation Plan

## Overview

A bulk data import/export system built with Go and Gin framework. Handles users, articles, and comments with async imports, streaming/async exports, and per-record error tracking.

---

## Quick Links

| Document | Description |
|----------|-------------|
| [Architecture](./architecture.md) | 3-layer design with embedded worker pools |
| [Database Schema](./database-schema.md) | Tables, constraints, and indexes |
| [Validation Strategy](./validation-strategy.md) | CTE-based validation with database constraints |
| [API Specification](./api-specification.md) | Import/Export endpoint details |
| [Testing Strategy](./testing-strategy.md) | Bash scripts and test scenarios |
| [Development Setup](./development-setup.md) | Docker Compose, Makefile, environment |

---

## Technology Stack

| Category | Technology |
|----------|------------|
| **Language** | Go 1.23 |
| **Framework** | Gin (based on [golang-gin-realworld-example-app](https://github.com/gothinkster/golang-gin-realworld-example-app)) |
| **Database** | PostgreSQL 15 with pgx driver |
| **Validation** | [ozzo-validation](https://github.com/go-ozzo/ozzo-validation) (no reflection, ~3.5x faster for bulk) |
| **API Docs** | Swagger with gin-swagger |
| **Monitoring** | Prometheus (Grafana optional) |
| **Development** | Docker Compose (DB in container, app runs locally) |

---

## Core Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Single Binary** | Embedded worker pools via goroutines, no separate worker process |
| **Worker Pool** | Configurable pool size (default: 4) for concurrent import/export processing |
| **Mixed Validation** | App-side via ozzo-validation (fast-fail, rich errors); DB-side for UNIQUE/FK (atomic, race-safe) |
| **CTE for DB Errors** | Failed records (duplicates, FK violations) tracked in same INSERT query |
| **Streaming I/O** | O(1) memory for both imports (batch) and exports (cursor) |
| **Batched Inserts** | 1000-record batches, each in own transaction for performance |
| **Continue on Failure** | Failed batches logged, remaining batches continue processing |
| **Error Aggregation** | All errors (app-side + DB-side + batch failures) collected in final response |
| **Import Order** | users → articles → comments (respects FK dependencies) |

---

## Testing Strategy

| Level | Tool | Purpose |
|-------|------|---------|
| **Unit Tests** | mockery | Handler/Service layer tests with mocks |
| **Integration Tests** | testcontainers | Repository tests with real PostgreSQL |
| **E2E Tests** | bash + curl | Full API testing with test data |

---

## Implementation Phases

### Phase 1: Foundation (Days 1-2)
- [ ] Project setup with Docker Compose (PostgreSQL + Prometheus)
- [ ] Database migrations with constraints
- [ ] Basic Gin server with Swagger docs

### Phase 2: Core Functionality (Days 3-4)
- [ ] Import endpoints (multipart + remote URL)
- [ ] Export endpoints (streaming + async)
- [ ] CTE-based bulk insert with failed record tracking
- [ ] Worker pool in service layer

### Phase 3: Testing (Days 5-6)
- [ ] Bash test scripts for all endpoints
- [ ] Performance validation with test data
- [ ] Error handling and edge cases
