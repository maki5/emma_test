# Development Setup

## Overview

This guide covers setting up the local development environment with:
- PostgreSQL running in Docker
- Prometheus + Grafana for monitoring
- Application running locally (outside Docker)

---

## Prerequisites

- **Go 1.23+** installed
- **Docker & Docker Compose** installed
- **jq** for JSON parsing in test scripts
- **curl** for API testing

---

## Quick Start

```bash
# 1. Start infrastructure (PostgreSQL, Prometheus, Grafana)
make dev-setup

# 2. Run the application
make run-server

# 3. Run tests
make test

# 4. View dashboards
make metrics-dashboard
```

---

## Docker Compose Configuration

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: bulk_import_export_dev
      POSTGRES_USER: dev
      POSTGRES_PASSWORD: dev
    ports:
      - "5432:5432"
    volumes:
      - postgres_dev_data:/var/lib/postgresql/data
      - ./testdata:/testdata
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dev"]
      interval: 5s
      timeout: 5s
      retries: 5

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards
    depends_on:
      - prometheus

volumes:
  postgres_dev_data:
  grafana_data:
```

---

## Prometheus Configuration

```yaml
# monitoring/prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'bulk-import-export'
    static_configs:
      - targets: ['host.docker.internal:8080']
    metrics_path: '/metrics'
    scrape_interval: 5s
```

---

## Environment Configuration

```bash
# .env

# Database
DATABASE_URL=postgres://dev:dev@localhost:5432/bulk_import_export_dev?sslmode=disable
DB_MAX_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=1h

# Application
HTTP_PORT=8080
LOG_LEVEL=debug
LOG_FILE=logs/app.log

# Worker Configuration
WORKER_POOL_SIZE=10
BATCH_SIZE=1000
MAX_CONCURRENT_JOBS=5

# Safety Limits
MAX_FILE_SIZE_MB=500
REMOTE_URL_TIMEOUT=5m

# Async Export
EXPORT_FILE_PATH=./exports

# Metrics
ENABLE_METRICS=true
METRICS_PATH=/metrics

# Testing
TESTDATA_DIR=./testdata
TEST_TIMEOUT=300s

# Swagger
ENABLE_SWAGGER=true
SWAGGER_HOST=localhost:8080
SWAGGER_BASE_PATH=/
```

---

## Makefile

```makefile
# Makefile

.PHONY: dev-setup run-server stop clean test metrics-dashboard

# ============================================
# Environment Setup
# ============================================

dev-setup: docker-up wait-for-db migrate-up move-testdata
	@echo "✅ Development environment ready!"
	@echo ""
	@echo "📊 Grafana dashboard: http://localhost:3000 (admin/admin)"
	@echo "🔍 Prometheus: http://localhost:9090"
	@echo "📖 Swagger: http://localhost:8080/swagger/index.html (after starting server)"

docker-up:
	@echo "🐳 Starting Docker services..."
	docker-compose up -d

docker-down:
	@echo "🛑 Stopping Docker services..."
	docker-compose down

wait-for-db:
	@echo "⏳ Waiting for PostgreSQL to be ready..."
	@timeout 30s bash -c 'until docker-compose exec -T postgres pg_isready -U dev; do sleep 1; done'
	@echo "✅ PostgreSQL is ready"

# ============================================
# Database
# ============================================

migrate-up:
	@echo "🗄️ Running database migrations..."
	go run cmd/server/main.go migrate up

migrate-down:
	@echo "🗄️ Rolling back database migrations..."
	go run cmd/server/main.go migrate down

migrate-status:
	@echo "📊 Migration status..."
	go run cmd/server/main.go migrate status

db-reset: migrate-down migrate-up
	@echo "✅ Database reset complete"

# ============================================
# Application
# ============================================

run-server:
	@echo "🚀 Starting application server..."
	go run cmd/server/main.go serve

build:
	@echo "🔨 Building application..."
	go build -o bin/server cmd/server/main.go

# ============================================
# Testing
# ============================================

test: test-imports test-exports test-validation
	@echo "✅ All tests completed"

test-complete:
	@echo "🧪 Running complete API tests..."
	bash scripts/test-complete-api.sh

test-imports:
	@echo "🧪 Running import tests..."
	bash scripts/test-imports.sh

test-exports:
	@echo "🧪 Running export tests..."
	bash scripts/test-exports.sh

test-validation:
	@echo "🧪 Running validation tests..."
	bash scripts/test-validation.sh

test-performance:
	@echo "🧪 Running performance tests..."
	bash scripts/test-performance.sh

test-setup:
	@echo "🔧 Setting up test environment..."
	bash scripts/utils/setup.sh

# ============================================
# Data Management
# ============================================

move-testdata:
	@echo "📁 Organizing test data files..."
	@mkdir -p testdata
	@[ -f users_huge.csv ] && mv users_huge.csv testdata/ || true
	@[ -f articles_huge.ndjson ] && mv articles_huge.ndjson testdata/ || true
	@[ -f comments_huge.ndjson ] && mv comments_huge.ndjson testdata/ || true
	@echo "✅ Test data organized"

# ============================================
# Monitoring
# ============================================

metrics-dashboard:
	@echo "📊 Opening Grafana dashboard..."
	open http://localhost:3000

prometheus:
	@echo "🔍 Opening Prometheus..."
	open http://localhost:9090

swagger:
	@echo "📖 Opening Swagger documentation..."
	open http://localhost:8080/swagger/index.html

# ============================================
# Logs
# ============================================

logs:
	docker-compose logs -f

logs-postgres:
	docker-compose logs -f postgres

logs-app:
	@echo "📋 Application logs:"
	tail -f logs/app.log

# ============================================
# Cleanup
# ============================================

stop: docker-down
	@echo "✅ All services stopped"

clean: stop
	@echo "🧹 Cleaning up..."
	docker-compose down -v
	docker system prune -f
	rm -rf logs/*.log
	rm -rf bin/
	@echo "✅ Cleanup complete"

# ============================================
# Code Quality
# ============================================

fmt:
	@echo "🎨 Formatting code..."
	go fmt ./...

lint:
	@echo "🔍 Running linter..."
	golangci-lint run

swagger-gen:
	@echo "📖 Generating Swagger docs..."
	swag init -g cmd/server/main.go -o docs/swagger

# ============================================
# Help
# ============================================

help:
	@echo "Available commands:"
	@echo ""
	@echo "Setup:"
	@echo "  make dev-setup      - Full environment setup (Docker + DB + testdata)"
	@echo "  make docker-up      - Start Docker services only"
	@echo "  make docker-down    - Stop Docker services"
	@echo ""
	@echo "Database:"
	@echo "  make migrate-up     - Run migrations"
	@echo "  make migrate-down   - Rollback migrations"
	@echo "  make db-reset       - Reset database"
	@echo ""
	@echo "Application:"
	@echo "  make run-server     - Start the application"
	@echo "  make build          - Build binary"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all tests"
	@echo "  make test-imports   - Test import endpoints"
	@echo "  make test-exports   - Test export endpoints"
	@echo "  make test-performance - Run performance tests"
	@echo ""
	@echo "Monitoring:"
	@echo "  make metrics-dashboard - Open Grafana"
	@echo "  make prometheus     - Open Prometheus"
	@echo "  make swagger        - Open Swagger UI"
	@echo ""
	@echo "Cleanup:"
	@echo "  make stop           - Stop all services"
	@echo "  make clean          - Full cleanup"
```

---

## Go Dependencies

```go
// go.mod
module bulk-import-export

go 1.23

require (
    // Base framework
    github.com/gin-gonic/gin v1.9.1
    
    // Database
    github.com/jackc/pgx/v5 v5.4.3
    
    // API Documentation
    github.com/swaggo/gin-swagger v1.6.0
    github.com/swaggo/swag v1.16.1
    
    // Logging & Metrics
    github.com/sirupsen/logrus v1.9.3
    github.com/prometheus/client_golang v1.16.0
    
    // Utilities
    github.com/google/uuid v1.3.1
    github.com/spf13/viper v1.16.0
    
    // Testing
    github.com/stretchr/testify v1.8.4
    
    // Database migrations
    github.com/golang-migrate/migrate/v4 v4.16.2
)
```

---

## Directory Structure After Setup

```
bulk-import-export/
├── .env                          # Environment configuration
├── docker-compose.yml            # Docker services
├── Makefile                      # Development commands
├── go.mod / go.sum               # Go dependencies
│
├── cmd/
│   └── server/
│       └── main.go               # Application entry point
│
├── internal/                     # Application code (3 layers)
│   ├── domain/                   # Business entities
│   ├── service/                  # Business logic + worker pool
│   ├── repository/               # Data access + CTE helper
│   ├── handler/                  # HTTP handlers
│   └── infrastructure/           # DB, metrics
│
├── migrations/                   # SQL migrations
├── scripts/                      # Test scripts
├── monitoring/                   # Prometheus config
├── testdata/                     # Test data files
└── docs/                         # Documentation
```

---

## Service URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| Application | http://localhost:8080 | - |
| Swagger UI | http://localhost:8080/swagger/index.html | - |
| Metrics | http://localhost:8080/metrics | - |
| PostgreSQL | localhost:5432 | dev/dev |
| Prometheus | http://localhost:9090 | - |
| Grafana | http://localhost:3000 | admin/admin |

---

## Troubleshooting

### PostgreSQL Connection Failed

```bash
# Check if PostgreSQL is running
docker-compose ps

# Check PostgreSQL logs
docker-compose logs postgres

# Restart PostgreSQL
docker-compose restart postgres
```

### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill the process
kill -9 <PID>
```

### Migration Failed

```bash
# Check migration status
make migrate-status

# Reset database
make db-reset
```

### Test Data Not Found

```bash
# Run test setup
make test-setup

# Verify files
ls -la testdata/
```

---

## Related Documents

- [Architecture](./architecture.md) - System design
- [Testing Strategy](./testing-strategy.md) - Test scripts
- [API Specification](./api-specification.md) - Endpoint details
