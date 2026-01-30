# Bulk Import/Export System

High-performance bulk data import/export API built with Go. Handles users, articles, and comments with async processing, streaming exports, and Prometheus metrics.

---

## Quick Start

```bash
# Start everything (DB + monitoring + migrations + app)
make dev

# Stop all services
make stop

# Run API tests (requires running server)
make test-api
```

**Access Points:**

| Service | URL | Credentials |
|---------|-----|-------------|
| API | http://localhost:8080 | - |
| Health Check | http://localhost:8080/health | - |
| Metrics | http://localhost:8080/metrics | - |
| Prometheus | http://localhost:9090 | - |
| Grafana | http://localhost:3000 | admin/admin |

---

## Development Workflow

### Running Tests

```bash
make test          # Unit tests
make test-api      # API integration tests (requires running server)
make test-coverage # Tests with coverage report
```

### Code Quality

```bash
make fmt           # Format code
make lint          # Run linter
make vet           # Run go vet
```

### Database Operations

```bash
make db-reset      # Reset database (destroy and recreate)
make db-shell      # Open database shell
make migrate-up    # Run migrations
make migrate-down  # Rollback migrations
```

---

## Make Commands Reference

| Command | Description |
|---------|-------------|
| `make dev` | Start everything (DB + monitoring + app) |
| `make stop` | Stop all services |
| `make test-api` | Run API integration tests |
| `make test` | Run unit tests |
| `make build` | Build binary |
| `make run` | Run app (without starting infrastructure) |
| `make clean` | Clean build artifacts |
| `make clean-all` | Clean everything including Docker volumes |
| `make help` | Show all available commands |

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check with DB status |
| GET | `/ready` | Kubernetes readiness probe |
| GET | `/live` | Kubernetes liveness probe |
| GET | `/metrics` | Prometheus metrics |
| POST | `/api/v1/imports` | Start async import job |
| GET | `/api/v1/imports/:id` | Get import job status |
| GET | `/api/v1/exports/:resource/stream` | Stream export (download) |
| POST | `/api/v1/exports` | Start async export job |
| GET | `/api/v1/exports/:id` | Get export job status |
| GET | `/api/v1/exports/:id/download` | Download completed export |

See [api-specification.md](./api-specification.md) for detailed documentation.

---

## Architecture

| Layer | Purpose |
|-------|---------|
| Handler | HTTP request handling, validation |
| Service | Business logic, worker pools, batch processing |
| Repository | Database access with CTE-based bulk operations |

**Key Features:**
- Worker pools for controlled concurrency
- Per-batch transactions with per-record error tracking
- Idempotency via unique tokens
- Streaming exports with database cursors

---

## Technology Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.24 |
| Framework | Gin |
| Database | PostgreSQL 15 (pgx) |
| Metrics | Prometheus + Grafana |
| Testing | mockery + testcontainers |

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| HTTP_PORT | 8080 | Server port |
| DATABASE_URL | required | PostgreSQL connection URL |
| WORKER_POOL_SIZE | 10 | Concurrent import workers |
| BATCH_SIZE | 1000 | Records per batch |
| EXPORT_DIR | ./exports | Export file directory |
