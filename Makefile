.PHONY: all build run dev test clean lint fmt help

# Variables
BINARY_NAME=server
DOCKER_COMPOSE=docker compose
GO=go
DB_URL=postgres://postgres:postgres@localhost:5432/bulk_import_export?sslmode=disable

# =============================================================================
# Main Targets
# =============================================================================

## Build the application
build:
	@echo "🔨 Building..."
	$(GO) build -o bin/$(BINARY_NAME) ./cmd/server

## Start everything for local development (DB + migrations + app with logs)
dev: db-start db-wait migrate-up
	@echo "🚀 Starting application..."
	@echo "📍 API: http://localhost:8080"
	@echo "❤️  Health: http://localhost:8080/health"
	@echo ""
	$(GO) run ./cmd/server

## Run the application (without starting DB)
run: build
	./bin/$(BINARY_NAME)

## Run API tests (requires running server)
test-api:
	@echo "🧪 Running API tests..."
	./scripts/test-api.sh

# =============================================================================
# Database
# =============================================================================

## Start PostgreSQL in Docker
db-start:
	@echo "🐳 Starting PostgreSQL..."
	$(DOCKER_COMPOSE) up -d postgres
	@echo "✅ PostgreSQL container started"

## Wait for database to be ready
db-wait:
	@echo "⏳ Waiting for PostgreSQL..."
	@until $(DOCKER_COMPOSE) exec -T postgres pg_isready -U postgres > /dev/null 2>&1; do sleep 1; done
	@echo "✅ PostgreSQL is ready"

## Stop PostgreSQL
db-stop:
	@echo "🛑 Stopping PostgreSQL..."
	$(DOCKER_COMPOSE) down

## Run database migrations
migrate-up:
	@echo "🗄️  Running migrations..."
	migrate -path ./migrations -database "$(DB_URL)" up

## Rollback database migrations
migrate-down:
	@echo "🗄️  Rolling back migrations..."
	migrate -path ./migrations -database "$(DB_URL)" down

## Open database shell
db-shell:
	$(DOCKER_COMPOSE) exec postgres psql -U postgres -d bulk_import_export

## Reset database (destroy and recreate)
db-reset: db-stop
	@echo "🔄 Resetting database..."
	$(DOCKER_COMPOSE) down -v
	$(MAKE) db-start db-wait migrate-up

# =============================================================================
# Testing
# =============================================================================

## Run all unit tests
test:
	$(GO) test -v -race -cover ./...

## Run tests with coverage report
test-coverage:
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report: coverage.html"

# =============================================================================
# Code Quality
# =============================================================================

## Format code
fmt:
	$(GO) fmt ./...

## Run linter
lint:
	golangci-lint run ./...

## Run go vet
vet:
	$(GO) vet ./...

## Generate mocks
mocks:
	mockery

# =============================================================================
# Cleanup
# =============================================================================

## Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

## Clean everything (including Docker volumes)
clean-all: clean db-stop
	$(DOCKER_COMPOSE) down -v

# =============================================================================
# Setup
# =============================================================================

## Download dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

## Install development tools
install-tools:
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/vektra/mockery/v2@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# =============================================================================
# Help
# =============================================================================

## Show this help
help:
	@echo "Bulk Import/Export API - Development Commands"
	@echo ""
	@echo "Quick Start:"
	@echo "  make dev        - Start DB, run migrations, and start app with logs"
	@echo "  make test-api   - Run API integration tests (requires running server)"
	@echo ""
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /' | head -30
