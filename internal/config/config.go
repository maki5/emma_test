package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	// Server configuration
	ServerPort   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// Database configuration
	DBHost              string
	DBPort              int
	DBUser              string
	DBPassword          string
	DBName              string
	DBSSLMode           string
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration

	// Worker pool configuration
	WorkerPoolSize int

	// Batch processing configuration
	BatchSize int

	// Export configuration
	ExportDir string

	// Logging configuration
	LogLevel string
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		ReadTimeout:         getEnvDuration("HTTP_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:        getEnvDuration("HTTP_WRITE_TIMEOUT", 5*time.Minute),
		IdleTimeout:         getEnvDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		DBHost:              getEnv("DB_HOST", "localhost"),
		DBPort:              getEnvInt("DB_PORT", 5432),
		DBUser:              getEnv("DB_USER", "postgres"),
		DBPassword:          getEnv("DB_PASSWORD", "postgres"),
		DBName:              getEnv("DB_NAME", "bulk_import_export"),
		DBSSLMode:           getEnv("DB_SSL_MODE", "disable"),
		DBMaxConns:          int32(getEnvInt("DB_MAX_CONNS", 25)),
		DBMinConns:          int32(getEnvInt("DB_MIN_CONNS", 5)),
		DBMaxConnLifetime:   getEnvDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:   getEnvDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBHealthCheckPeriod: getEnvDuration("DB_HEALTH_CHECK_PERIOD", time.Minute),
		WorkerPoolSize:      getEnvInt("WORKER_POOL_SIZE", 4),
		BatchSize:           getEnvInt("BATCH_SIZE", 1000),
		ExportDir:           getEnv("EXPORT_DIR", "./exports"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate validates the configuration.
func (c *Config) validate() error {
	if c.ServerPort == "" {
		return fmt.Errorf("SERVER_PORT is required")
	}
	if c.DBHost == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	if c.DBUser == "" {
		return fmt.Errorf("DB_USER is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	if c.WorkerPoolSize < 1 {
		return fmt.Errorf("WORKER_POOL_SIZE must be at least 1")
	}
	if c.BatchSize < 1 {
		return fmt.Errorf("BATCH_SIZE must be at least 1")
	}
	return nil
}

// getEnv gets an environment variable with a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as int with a default value.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvDuration gets an environment variable as duration with a default value.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
