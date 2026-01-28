package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	originalEnv := make(map[string]string)
	envVars := []string{
		"SERVER_PORT",
		"DB_HOST",
		"DB_PORT",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_SSL_MODE",
		"DB_MAX_CONNS",
		"DB_MIN_CONNS",
		"BATCH_SIZE",
		"WORKER_POOL_SIZE",
		"EXPORT_DIR",
	}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
	}

	defer func() {
		for env, val := range originalEnv {
			if val == "" {
				os.Unsetenv(env)
			} else {
				os.Setenv(env, val)
			}
		}
	}()

	for _, env := range envVars {
		os.Unsetenv(env)
	}

	t.Run("default values", func(t *testing.T) {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.ServerPort != "8080" {
			t.Errorf("ServerPort = %v, want 8080", cfg.ServerPort)
		}
		if cfg.DBHost != "localhost" {
			t.Errorf("DBHost = %v, want localhost", cfg.DBHost)
		}
		if cfg.DBPort != 5432 {
			t.Errorf("DBPort = %v, want 5432", cfg.DBPort)
		}
		if cfg.DBUser != "postgres" {
			t.Errorf("DBUser = %v, want postgres", cfg.DBUser)
		}
		if cfg.DBName != "bulk_import_export" {
			t.Errorf("DBName = %v, want bulk_import_export", cfg.DBName)
		}
		if cfg.DBSSLMode != "disable" {
			t.Errorf("DBSSLMode = %v, want disable", cfg.DBSSLMode)
		}
		if cfg.DBMaxConns != 25 {
			t.Errorf("DBMaxConns = %v, want 25", cfg.DBMaxConns)
		}
		if cfg.DBMinConns != 5 {
			t.Errorf("DBMinConns = %v, want 5", cfg.DBMinConns)
		}
		if cfg.BatchSize != 1000 {
			t.Errorf("BatchSize = %v, want 1000", cfg.BatchSize)
		}
		if cfg.WorkerPoolSize != 4 {
			t.Errorf("WorkerPoolSize = %v, want 4", cfg.WorkerPoolSize)
		}
		if cfg.ExportDir != "./exports" {
			t.Errorf("ExportDir = %v, want ./exports", cfg.ExportDir)
		}
	})

	t.Run("custom values from environment", func(t *testing.T) {
		os.Setenv("SERVER_PORT", "9090")
		os.Setenv("DB_HOST", "db.example.com")
		os.Setenv("DB_PORT", "5433")
		os.Setenv("DB_USER", "testuser")
		os.Setenv("DB_PASSWORD", "testpass")
		os.Setenv("DB_NAME", "testdb")
		os.Setenv("DB_SSL_MODE", "require")
		os.Setenv("DB_MAX_CONNS", "50")
		os.Setenv("DB_MIN_CONNS", "10")
		os.Setenv("BATCH_SIZE", "500")
		os.Setenv("WORKER_POOL_SIZE", "8")
		os.Setenv("EXPORT_DIR", "/tmp/exports")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.ServerPort != "9090" {
			t.Errorf("ServerPort = %v, want 9090", cfg.ServerPort)
		}
		if cfg.DBHost != "db.example.com" {
			t.Errorf("DBHost = %v, want db.example.com", cfg.DBHost)
		}
		if cfg.DBPort != 5433 {
			t.Errorf("DBPort = %v, want 5433", cfg.DBPort)
		}
		if cfg.DBUser != "testuser" {
			t.Errorf("DBUser = %v, want testuser", cfg.DBUser)
		}
		if cfg.DBPassword != "testpass" {
			t.Errorf("DBPassword = %v, want testpass", cfg.DBPassword)
		}
		if cfg.DBName != "testdb" {
			t.Errorf("DBName = %v, want testdb", cfg.DBName)
		}
		if cfg.DBSSLMode != "require" {
			t.Errorf("DBSSLMode = %v, want require", cfg.DBSSLMode)
		}
		if cfg.DBMaxConns != 50 {
			t.Errorf("DBMaxConns = %v, want 50", cfg.DBMaxConns)
		}
		if cfg.DBMinConns != 10 {
			t.Errorf("DBMinConns = %v, want 10", cfg.DBMinConns)
		}
		if cfg.BatchSize != 500 {
			t.Errorf("BatchSize = %v, want 500", cfg.BatchSize)
		}
		if cfg.WorkerPoolSize != 8 {
			t.Errorf("WorkerPoolSize = %v, want 8", cfg.WorkerPoolSize)
		}
		if cfg.ExportDir != "/tmp/exports" {
			t.Errorf("ExportDir = %v, want /tmp/exports", cfg.ExportDir)
		}
	})

	t.Run("duration fields have correct defaults", func(t *testing.T) {
		for _, env := range envVars {
			os.Unsetenv(env)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.DBMaxConnLifetime != time.Hour {
			t.Errorf("DBMaxConnLifetime = %v, want 1h", cfg.DBMaxConnLifetime)
		}
		if cfg.DBMaxConnIdleTime != 30*time.Minute {
			t.Errorf("DBMaxConnIdleTime = %v, want 30m", cfg.DBMaxConnIdleTime)
		}
		if cfg.DBHealthCheckPeriod != time.Minute {
			t.Errorf("DBHealthCheckPeriod = %v, want 1m", cfg.DBHealthCheckPeriod)
		}
	})
}
