package repository_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB holds the test database connection and container
type TestDB struct {
	Pool      *pgxpool.Pool
	Container testcontainers.Container
	ConnStr   string
}

// SetupTestDB creates a PostgreSQL container and applies migrations
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()
	ctx := context.Background()

	// Get the migrations directory path
	_, currentFile, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations")

	// Create PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start postgres container: %v", err)
	}

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Run migrations
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		connStr,
	)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("Failed to create migrate instance: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("Failed to run migrations: %v", err)
	}
	m.Close()

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		t.Fatalf("Failed to ping database: %v", err)
	}

	return &TestDB{
		Pool:      pool,
		Container: pgContainer,
		ConnStr:   connStr,
	}
}

// Cleanup closes the connection pool and terminates the container
func (tdb *TestDB) Cleanup(t *testing.T) {
	t.Helper()
	if tdb.Pool != nil {
		tdb.Pool.Close()
	}
	if tdb.Container != nil {
		if err := tdb.Container.Terminate(context.Background()); err != nil {
			t.Logf("Warning: failed to terminate container: %v", err)
		}
	}
}

// TruncateTables clears all data from tables for test isolation
func (tdb *TestDB) TruncateTables(t *testing.T, tables ...string) {
	t.Helper()
	ctx := context.Background()
	for _, table := range tables {
		_, err := tdb.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("Failed to truncate table %s: %v", table, err)
		}
	}
}
