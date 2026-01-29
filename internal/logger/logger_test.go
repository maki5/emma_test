package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"bulk-import-export/internal/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	logger.Info("test message",
		slog.String("key", "value"),
		slog.Int("count", 42),
	)

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
	assert.Contains(t, output, "count")
	assert.Contains(t, output, "42")
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	logger.Error("error occurred",
		slog.String("error", "test error"),
	)

	output := buf.String()
	assert.Contains(t, output, "error occurred")
	assert.Contains(t, output, "test error")
}

func TestLogger_WithRequestID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	reqLogger := logger.WithRequestID("req-123")
	reqLogger.Info("processing request")

	output := buf.String()
	assert.Contains(t, output, "processing request")
	assert.Contains(t, output, "request_id")
	assert.Contains(t, output, "req-123")
}

func TestLogger_WithJobID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	jobLogger := logger.WithJobID("job-456")
	jobLogger.Info("processing job")

	output := buf.String()
	assert.Contains(t, output, "processing job")
	assert.Contains(t, output, "job_id")
	assert.Contains(t, output, "job-456")
}

func TestLogger_InfoContext(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	ctx := context.Background()
	logger.InfoContext(ctx, "context message",
		slog.String("key", "value"),
	)

	output := buf.String()
	assert.Contains(t, output, "context message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

func TestLogger_GetLogger(t *testing.T) {
	lg := logger.GetLogger()
	require.NotNil(t, lg)
}

func TestLogger_Default(t *testing.T) {
	lg := logger.Default()
	require.NotNil(t, lg)
	// Default() should return the same instance as GetLogger()
	assert.Equal(t, logger.GetLogger(), lg)
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	testLogger := slog.New(handler)
	logger.SetLogger(testLogger)

	fieldsLogger := logger.WithFields(
		slog.String("service", "import"),
		slog.Int("batch_size", 1000),
	)
	fieldsLogger.Info("batch processing")

	output := buf.String()
	assert.Contains(t, output, "batch processing")
	assert.Contains(t, output, "service")
	assert.Contains(t, output, "import")
	assert.Contains(t, output, "batch_size")
	assert.Contains(t, output, "1000")
}
