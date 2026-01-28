.PHONY: all build run test clean docker-up docker-down migrate-up migrate-down lint fmt

# Variables
BINARY_NAME=server
DOCKER_COMPOSE=docker compose
GO=go
GOTEST=$(GO) test
GOVET=$(GO) vet

# Default target
all: build

# Build the application
build:
	$(GO) build -o bin/$(BINARY_NAME) ./cmd/server

# Run the application locally
run: build
	./bin/$(BINARY_NAME)

# Run with hot reload (requires air)
dev:
	air

# Run all tests
test:
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Run unit tests only
test-unit:
	$(GOTEST) -v -short ./...

# Run integration tests
test-integration:
	$(GOTEST) -v -run Integration ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Format code
fmt:
	$(GO) fmt ./...

# Lint code
lint:
	golangci-lint run ./...

# Vet code
vet:
	$(GOVET) ./...

# Download dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Generate mocks
mocks:
	mockery --all --keeptree --output=internal/mocks

# Docker commands
docker-build:
	docker build -t bulk-import-export:latest .

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-restart:
	$(DOCKER_COMPOSE) restart

# Database migration commands
migrate-up:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/bulk_import_export?sslmode=disable" up

migrate-down:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/bulk_import_export?sslmode=disable" down

migrate-create:
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir ./migrations -seq $$name

# Database commands
db-shell:
	docker exec -it $$(docker ps -qf "name=postgres") psql -U postgres -d bulk_import_export

db-reset:
	$(DOCKER_COMPOSE) down -v
	$(DOCKER_COMPOSE) up -d postgres
	sleep 3
	$(DOCKER_COMPOSE) up migrate

# Install development tools
install-tools:
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/vektra/mockery/v2@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build the application"
	@echo "  run             - Build and run the application"
	@echo "  dev             - Run with hot reload (requires air)"
	@echo "  test            - Run all tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  test-unit       - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  clean           - Clean build artifacts"
	@echo "  fmt             - Format code"
	@echo "  lint            - Lint code"
	@echo "  deps            - Download dependencies"
	@echo "  mocks           - Generate mocks"
	@echo "  docker-build    - Build Docker image"
	@echo "  docker-up       - Start services with Docker Compose"
	@echo "  docker-down     - Stop services"
	@echo "  migrate-up      - Run database migrations"
	@echo "  migrate-down    - Rollback database migrations"
	@echo "  db-shell        - Open database shell"
	@echo "  db-reset        - Reset database"
	@echo "  install-tools   - Install development tools"
