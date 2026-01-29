# Development Setup

## Quick Start

```bash
# Start everything (PostgreSQL + migrations + app with logs)
make dev
```

This single command will:
1. Start PostgreSQL in Docker
2. Wait for database to be ready
3. Run migrations
4. Start the application with logs in your terminal

## Prerequisites

- **Go 1.24+** installed
- **Docker** installed
- **jq** for JSON parsing in test scripts
- **migrate** CLI for database migrations

Install development tools:
```bash
make install-tools
```

## Running Tests

With the server running (in a separate terminal):
```bash
make test-api
```

Unit tests:
```bash
make test
```

## Available Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start DB, migrations, and app with logs |
| `make run` | Build and run app (DB must be running) |
| `make test` | Run unit tests |
| `make test-api` | Run API integration tests |
| `make db-start` | Start PostgreSQL only |
| `make db-stop` | Stop PostgreSQL |
| `make db-shell` | Open psql shell |
| `make db-reset` | Reset database (destroy and recreate) |
| `make migrate-up` | Run migrations |
| `make migrate-down` | Rollback migrations |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make mocks` | Generate mocks |
| `make clean` | Clean build artifacts |

## Environment Variables

The application uses these environment variables (with defaults):

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP server port |
| `DB_HOST` | `localhost` | Database host |
| `DB_PORT` | `5432` | Database port |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | `postgres` | Database password |
| `DB_NAME` | `bulk_import_export` | Database name |
| `DB_SSL_MODE` | `disable` | SSL mode |
| `WORKER_POOL_SIZE` | `4` | Worker pool size |
| `BATCH_SIZE` | `1000` | Batch size for imports |
| `EXPORT_DIR` | `./exports` | Directory for export files |

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/imports` | Start import job |
| GET | `/api/v1/imports/:id` | Get import job status |
| GET | `/api/v1/exports?resource=...&format=...` | Stream export |
| POST | `/api/v1/exports` | Start async export job |
| GET | `/api/v1/exports/:id` | Get export job status |

## Project Structure

```
.
├── cmd/server/          # Application entrypoint
├── internal/
│   ├── config/          # Configuration
│   ├── domain/          # Domain models
│   ├── handler/         # HTTP handlers
│   ├── middleware/      # HTTP middleware
│   ├── repository/      # Database repositories
│   ├── service/         # Business logic
│   └── validator/       # Validation logic
├── migrations/          # Database migrations
├── scripts/             # Utility scripts
├── testdata/            # Test data files
└── exports/             # Export output directory
```
