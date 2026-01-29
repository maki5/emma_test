package logger

import (
	"context"
	"log/slog"
	"os"
)

var defaultLogger *slog.Logger

func init() {
	// Initialize default structured logger
	// JSON format for production, text for development
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	defaultLogger = slog.New(handler)
}

// SetLogger allows setting a custom logger (useful for testing)
func SetLogger(logger *slog.Logger) {
	defaultLogger = logger
}

// GetLogger returns the default logger
func GetLogger() *slog.Logger {
	return defaultLogger
}

// Default returns the default logger (alias for GetLogger)
func Default() *slog.Logger {
	return defaultLogger
}

// Info logs an info message with optional attributes
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// InfoContext logs an info message with context
func InfoContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.InfoContext(ctx, msg, args...)
}

// Error logs an error message with optional attributes
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// ErrorContext logs an error message with context
func ErrorContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.ErrorContext(ctx, msg, args...)
}

// Warn logs a warning message with optional attributes
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// WarnContext logs a warning message with context
func WarnContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.WarnContext(ctx, msg, args...)
}

// Debug logs a debug message with optional attributes
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// DebugContext logs a debug message with context
func DebugContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.DebugContext(ctx, msg, args...)
}

// Fatal logs a fatal message and exits
func Fatal(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
	os.Exit(1)
}

// WithRequestID adds request_id to logger context
func WithRequestID(requestID string) *slog.Logger {
	return defaultLogger.With(slog.String("request_id", requestID))
}

// WithJobID adds job_id to logger context
func WithJobID(jobID string) *slog.Logger {
	return defaultLogger.With(slog.String("job_id", jobID))
}

// WithFields creates a logger with multiple fields
func WithFields(attrs ...slog.Attr) *slog.Logger {
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}
	return defaultLogger.With(args...)
}
