// Package metrics provides Prometheus metrics for observability.
// Metrics are organized by domain: HTTP requests, job processing, and database operations.
package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "bulk_import_export"
)

var (
	// HTTP metrics - track request volume and latency
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests by method, path, and status code",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"method", "path"},
	)

	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being processed",
		},
	)

	// Job metrics - track import/export job processing
	JobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "total",
			Help:      "Total number of jobs by type, resource type, and status",
		},
		[]string{"job_type", "resource_type", "status"},
	)

	JobsInProgress = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "in_progress",
			Help:      "Number of jobs currently in progress",
		},
		[]string{"job_type", "resource_type"},
	)

	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "jobs",
			Name:      "duration_seconds",
			Help:      "Job processing duration in seconds",
			Buckets:   []float64{.1, .5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"job_type", "resource_type"},
	)

	// Record processing metrics - track records within jobs
	RecordsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "records",
			Name:      "processed_total",
			Help:      "Total number of records processed by job type, resource type, and result",
		},
		[]string{"job_type", "resource_type", "result"},
	)

	BatchProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "records",
			Name:      "batch_duration_seconds",
			Help:      "Batch processing duration in seconds",
			Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"job_type", "resource_type", "operation"},
	)

	// Streaming export metrics - track streaming exports
	StreamingExportsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "streaming",
			Name:      "exports_total",
			Help:      "Total number of streaming exports by resource type, format, and result",
		},
		[]string{"resource_type", "format", "result"},
	)

	StreamingExportDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "streaming",
			Name:      "export_duration_seconds",
			Help:      "Streaming export duration in seconds",
			Buckets:   []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"resource_type", "format"},
	)

	StreamingExportRecords = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "streaming",
			Name:      "records_total",
			Help:      "Total number of records streamed by resource type and format",
		},
		[]string{"resource_type", "format"},
	)

	StreamingExportsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "streaming",
			Name:      "exports_in_flight",
			Help:      "Number of streaming exports currently in progress",
		},
		[]string{"resource_type"},
	)

	// Database metrics - track database operation performance
	DBConnectionPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "db",
			Name:      "pool_connections",
			Help:      "Database connection pool stats",
		},
		[]string{"state"},
	)
)

// PoolStats is an interface for getting pool statistics
// This allows for easier testing by mocking the pool stats
type PoolStats interface {
	TotalConns() int32
	IdleConns() int32
	AcquiredConns() int32
}

// PoolStatsProvider is an interface for providing pool stats
type PoolStatsProvider interface {
	Stat() PoolStats
}

// pgxPoolAdapter adapts pgxpool.Pool to PoolStatsProvider
type pgxPoolAdapter struct {
	pool *pgxpool.Pool
}

func (a *pgxPoolAdapter) Stat() PoolStats {
	return a.pool.Stat()
}

// PoolStatsCollector collects database pool statistics periodically
type PoolStatsCollector struct {
	provider PoolStatsProvider
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewPoolStatsCollector creates a new pool stats collector
func NewPoolStatsCollector(pool *pgxpool.Pool) *PoolStatsCollector {
	return &PoolStatsCollector{
		provider: &pgxPoolAdapter{pool: pool},
		stopChan: make(chan struct{}),
	}
}

// NewPoolStatsCollectorWithProvider creates a new pool stats collector with a custom provider (for testing)
func NewPoolStatsCollectorWithProvider(provider PoolStatsProvider) *PoolStatsCollector {
	return &PoolStatsCollector{
		provider: provider,
		stopChan: make(chan struct{}),
	}
}

// Start begins collecting pool stats every interval
func (c *PoolStatsCollector) Start(interval time.Duration) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Collect immediately on start
		c.collect()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopChan:
				return
			}
		}
	}()
}

func (c *PoolStatsCollector) collect() {
	stats := c.provider.Stat()
	DBConnectionPoolSize.WithLabelValues("total").Set(float64(stats.TotalConns()))
	DBConnectionPoolSize.WithLabelValues("idle").Set(float64(stats.IdleConns()))
	DBConnectionPoolSize.WithLabelValues("in_use").Set(float64(stats.AcquiredConns()))
}

// Stop stops the pool stats collector
func (c *PoolStatsCollector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}

// ObserveJobCompletion records metrics when a job completes
func ObserveJobCompletion(jobType, resourceType, status string, durationSeconds float64, successCount, failureCount int) {
	JobsTotal.WithLabelValues(jobType, resourceType, status).Inc()
	JobDuration.WithLabelValues(jobType, resourceType).Observe(durationSeconds)

	if successCount > 0 {
		RecordsProcessed.WithLabelValues(jobType, resourceType, "success").Add(float64(successCount))
	}
	if failureCount > 0 {
		RecordsProcessed.WithLabelValues(jobType, resourceType, "failure").Add(float64(failureCount))
	}
}

// StartJob increments the in-progress counter for a job
func StartJob(jobType, resourceType string) {
	JobsInProgress.WithLabelValues(jobType, resourceType).Inc()
}

// EndJob decrements the in-progress counter for a job
func EndJob(jobType, resourceType string) {
	JobsInProgress.WithLabelValues(jobType, resourceType).Dec()
}

// ObserveBatchDuration records the time taken to process a batch
func ObserveBatchDuration(jobType, resourceType, operation string, durationSeconds float64) {
	BatchProcessingDuration.WithLabelValues(jobType, resourceType, operation).Observe(durationSeconds)
}

// Timer is a helper for measuring operation duration
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer starting now
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ObserveDuration records the elapsed time since the timer was created
func (t *Timer) ObserveDuration(observer prometheus.Observer) {
	observer.Observe(time.Since(t.start).Seconds())
}

// LogHealthCheckMetrics logs database health check result (for debugging)
func LogHealthCheckMetrics(ctx context.Context, pool *pgxpool.Pool) {
	stats := pool.Stat()
	slog.Debug("Database pool stats",
		slog.Int("total_conns", int(stats.TotalConns())),
		slog.Int("idle_conns", int(stats.IdleConns())),
		slog.Int("acquired_conns", int(stats.AcquiredConns())),
		slog.Int64("acquire_count", stats.AcquireCount()),
		slog.Int64("canceled_acquire_count", stats.CanceledAcquireCount()),
	)
}

// StartStreamingExport starts tracking a streaming export
func StartStreamingExport(resourceType string) {
	StreamingExportsInFlight.WithLabelValues(resourceType).Inc()
}

// EndStreamingExport ends tracking a streaming export and records metrics
func EndStreamingExport(resourceType, format, result string, durationSeconds float64, recordCount int) {
	StreamingExportsInFlight.WithLabelValues(resourceType).Dec()
	StreamingExportsTotal.WithLabelValues(resourceType, format, result).Inc()
	StreamingExportDuration.WithLabelValues(resourceType, format).Observe(durationSeconds)
	if recordCount > 0 {
		StreamingExportRecords.WithLabelValues(resourceType, format).Add(float64(recordCount))
	}
}
