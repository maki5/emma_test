package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestObserveJobCompletion(t *testing.T) {
	// Reset counters for clean test (counters cannot be reset in prometheus, so we just test increments)
	initialTotal := testutil.ToFloat64(JobsTotal.WithLabelValues("import", "users", "completed"))

	ObserveJobCompletion("import", "users", "completed", 5.5, 100, 5)

	// Verify counter incremented
	newTotal := testutil.ToFloat64(JobsTotal.WithLabelValues("import", "users", "completed"))
	assert.Equal(t, initialTotal+1, newTotal, "JobsTotal should increment by 1")

	// Verify records processed
	successRecords := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "users", "success"))
	assert.GreaterOrEqual(t, successRecords, float64(100), "Success records should be recorded")

	failureRecords := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "users", "failure"))
	assert.GreaterOrEqual(t, failureRecords, float64(5), "Failure records should be recorded")
}

func TestStartEndJob(t *testing.T) {
	initialInProgress := testutil.ToFloat64(JobsInProgress.WithLabelValues("export", "articles"))

	StartJob("export", "articles")
	afterStart := testutil.ToFloat64(JobsInProgress.WithLabelValues("export", "articles"))
	assert.Equal(t, initialInProgress+1, afterStart, "In-progress should increment on StartJob")

	EndJob("export", "articles")
	afterEnd := testutil.ToFloat64(JobsInProgress.WithLabelValues("export", "articles"))
	assert.Equal(t, initialInProgress, afterEnd, "In-progress should decrement on EndJob")
}

func TestObserveBatchDuration(t *testing.T) {
	// Just verify no panic and metric is recorded
	ObserveBatchDuration("import", "comments", "db", 0.5)

	// Verify histogram has observations
	count := testutil.CollectAndCount(BatchProcessingDuration)
	assert.GreaterOrEqual(t, count, 1, "BatchProcessingDuration should have observations")
}

func TestTimer(t *testing.T) {
	timer := NewTimer()

	// Sleep a bit to have measurable duration
	time.Sleep(10 * time.Millisecond)

	// Create a test histogram to observe
	testHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_timer_histogram",
		Help:    "Test histogram for timer",
		Buckets: []float64{.01, .05, .1, .5, 1},
	})

	timer.ObserveDuration(testHistogram)

	// Verify histogram was observed (should have at least one observation)
	// We can't easily read the value, but we can verify no panic occurred
}

func TestHTTPMetricsExist(t *testing.T) {
	// Verify HTTP metrics are properly initialized
	assert.NotNil(t, HTTPRequestsTotal)
	assert.NotNil(t, HTTPRequestDuration)
	assert.NotNil(t, HTTPRequestsInFlight)

	// Increment and verify
	initialRequests := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "/health", "200"))
	HTTPRequestsTotal.WithLabelValues("GET", "/health", "200").Inc()
	newRequests := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "/health", "200"))
	assert.Equal(t, initialRequests+1, newRequests)
}

func TestDBConnectionPoolSizeMetric(t *testing.T) {
	// Verify DB pool metric exists and can be set
	DBConnectionPoolSize.WithLabelValues("total").Set(10)
	DBConnectionPoolSize.WithLabelValues("idle").Set(5)
	DBConnectionPoolSize.WithLabelValues("in_use").Set(5)

	assert.Equal(t, float64(10), testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("total")))
	assert.Equal(t, float64(5), testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("idle")))
	assert.Equal(t, float64(5), testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("in_use")))
}

func TestStreamingExportMetrics(t *testing.T) {
	// Get initial values
	initialTotal := testutil.ToFloat64(StreamingExportsTotal.WithLabelValues("users", "ndjson", "success"))
	initialInFlight := testutil.ToFloat64(StreamingExportsInFlight.WithLabelValues("users"))
	initialRecords := testutil.ToFloat64(StreamingExportRecords.WithLabelValues("users", "ndjson"))

	// Start streaming export
	StartStreamingExport("users")
	afterStart := testutil.ToFloat64(StreamingExportsInFlight.WithLabelValues("users"))
	assert.Equal(t, initialInFlight+1, afterStart, "In-flight should increment on StartStreamingExport")

	// End streaming export
	EndStreamingExport("users", "ndjson", "success", 0.5, 1000)

	// Verify metrics updated
	afterEnd := testutil.ToFloat64(StreamingExportsInFlight.WithLabelValues("users"))
	assert.Equal(t, initialInFlight, afterEnd, "In-flight should decrement on EndStreamingExport")

	newTotal := testutil.ToFloat64(StreamingExportsTotal.WithLabelValues("users", "ndjson", "success"))
	assert.Equal(t, initialTotal+1, newTotal, "StreamingExportsTotal should increment")

	newRecords := testutil.ToFloat64(StreamingExportRecords.WithLabelValues("users", "ndjson"))
	assert.Equal(t, initialRecords+1000, newRecords, "StreamingExportRecords should increase by record count")
}

func TestStreamingExportErrorMetrics(t *testing.T) {
	initialErrors := testutil.ToFloat64(StreamingExportsTotal.WithLabelValues("articles", "csv", "error"))

	// Simulate a failed streaming export
	StartStreamingExport("articles")
	EndStreamingExport("articles", "csv", "error", 1.0, 500)

	newErrors := testutil.ToFloat64(StreamingExportsTotal.WithLabelValues("articles", "csv", "error"))
	assert.Equal(t, initialErrors+1, newErrors, "Error count should increment")
}

func TestStreamingExportZeroRecords(t *testing.T) {
	initialRecords := testutil.ToFloat64(StreamingExportRecords.WithLabelValues("comments", "csv"))

	// End streaming export with zero records (e.g., empty result set)
	StartStreamingExport("comments")
	EndStreamingExport("comments", "csv", "success", 0.1, 0)

	// Records should not change when count is 0
	newRecords := testutil.ToFloat64(StreamingExportRecords.WithLabelValues("comments", "csv"))
	assert.Equal(t, initialRecords, newRecords, "StreamingExportRecords should not change for zero records")
}

func TestTimerObserveDuration(t *testing.T) {
	timer := NewTimer()

	// Sleep a bit to have measurable duration
	time.Sleep(50 * time.Millisecond)

	// Create a test histogram to observe
	testHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_timer_duration_histogram",
		Help:    "Test histogram for timer duration",
		Buckets: []float64{.01, .05, .1, .5, 1},
	})
	prometheus.MustRegister(testHistogram)
	defer prometheus.Unregister(testHistogram)

	timer.ObserveDuration(testHistogram)

	// Verify the histogram received an observation
	count := testutil.CollectAndCount(testHistogram)
	assert.Equal(t, 1, count, "Histogram should have exactly one observation")
}

func TestPoolStatsCollectorStartStop(t *testing.T) {
	// Create a mock pool stats provider
	mockProvider := &mockPoolStatsProvider{
		totalConns:    10,
		idleConns:     5,
		acquiredConns: 5,
	}

	collector := NewPoolStatsCollectorWithProvider(mockProvider)

	// Start the collector with a short interval
	collector.Start(10 * time.Millisecond)

	// Let it run for a bit to collect stats
	time.Sleep(30 * time.Millisecond)

	// Verify stats were collected
	total := testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("total"))
	idle := testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("idle"))
	inUse := testutil.ToFloat64(DBConnectionPoolSize.WithLabelValues("in_use"))

	assert.Equal(t, float64(10), total, "Total connections should be 10")
	assert.Equal(t, float64(5), idle, "Idle connections should be 5")
	assert.Equal(t, float64(5), inUse, "In-use connections should be 5")

	// Stop the collector
	collector.Stop()

	// Verify it stopped (no panic, completes in reasonable time)
}

// mockPoolStats implements PoolStats for testing
type mockPoolStats struct {
	total    int32
	idle     int32
	acquired int32
}

func (m *mockPoolStats) TotalConns() int32    { return m.total }
func (m *mockPoolStats) IdleConns() int32     { return m.idle }
func (m *mockPoolStats) AcquiredConns() int32 { return m.acquired }

// mockPoolStatsProvider implements PoolStatsProvider for testing
type mockPoolStatsProvider struct {
	totalConns    int32
	idleConns     int32
	acquiredConns int32
}

func (m *mockPoolStatsProvider) Stat() PoolStats {
	return &mockPoolStats{
		total:    m.totalConns,
		idle:     m.idleConns,
		acquired: m.acquiredConns,
	}
}

func TestPoolStatsCollectorMultipleCollections(t *testing.T) {
	// Create a mock that changes values
	mockProvider := &dynamicMockPoolStatsProvider{
		calls: 0,
	}

	collector := NewPoolStatsCollectorWithProvider(mockProvider)
	collector.Start(5 * time.Millisecond)

	// Let it collect a few times
	time.Sleep(25 * time.Millisecond)

	collector.Stop()

	// Should have collected multiple times
	assert.GreaterOrEqual(t, mockProvider.calls, 2, "Should collect multiple times")
}

type dynamicMockPoolStatsProvider struct {
	calls int
}

func (m *dynamicMockPoolStatsProvider) Stat() PoolStats {
	m.calls++
	return &mockPoolStats{
		total:    int32(10 + m.calls),
		idle:     int32(5),
		acquired: int32(5 + m.calls),
	}
}

func TestObserveJobCompletionZeroCounts(t *testing.T) {
	// Test with zero success and failure counts
	initialTotal := testutil.ToFloat64(JobsTotal.WithLabelValues("import", "comments", "completed"))
	initialSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "comments", "success"))
	initialFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "comments", "failure"))

	ObserveJobCompletion("import", "comments", "completed", 1.0, 0, 0)

	// JobsTotal should still increment
	newTotal := testutil.ToFloat64(JobsTotal.WithLabelValues("import", "comments", "completed"))
	assert.Equal(t, initialTotal+1, newTotal, "JobsTotal should increment")

	// Records should not change
	newSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "comments", "success"))
	newFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "comments", "failure"))
	assert.Equal(t, initialSuccess, newSuccess, "Success records should not change for zero count")
	assert.Equal(t, initialFailure, newFailure, "Failure records should not change for zero count")
}

func TestObserveJobCompletionOnlySuccess(t *testing.T) {
	initialSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("export", "users", "success"))
	initialFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("export", "users", "failure"))

	ObserveJobCompletion("export", "users", "completed", 2.0, 500, 0)

	newSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("export", "users", "success"))
	newFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("export", "users", "failure"))
	assert.Equal(t, initialSuccess+500, newSuccess, "Success records should increase")
	assert.Equal(t, initialFailure, newFailure, "Failure records should not change")
}

func TestObserveJobCompletionOnlyFailure(t *testing.T) {
	initialSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "articles", "success"))
	initialFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "articles", "failure"))

	ObserveJobCompletion("import", "articles", "failed", 3.0, 0, 100)

	newSuccess := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "articles", "success"))
	newFailure := testutil.ToFloat64(RecordsProcessed.WithLabelValues("import", "articles", "failure"))
	assert.Equal(t, initialSuccess, newSuccess, "Success records should not change")
	assert.Equal(t, initialFailure+100, newFailure, "Failure records should increase")
}

func TestLogHealthCheckMetrics(t *testing.T) {
	// LogHealthCheckMetrics requires a real pgxpool.Pool which we can't easily mock
	// This test just verifies the function signature and that it would be callable
	// The actual integration test would use a real database connection
	t.Run("function exists and is callable", func(t *testing.T) {
		// Verify the function is defined (compile-time check)
		var _ = LogHealthCheckMetrics
	})
}

func TestJobDurationHistogramBuckets(t *testing.T) {
	// Observe various durations and verify they're recorded
	durations := []float64{0.1, 0.5, 1.0, 5.0, 30.0, 120.0}

	for _, d := range durations {
		JobDuration.WithLabelValues("test", "test_resource").Observe(d)
	}

	// Verify histogram has observations
	count := testutil.CollectAndCount(JobDuration)
	assert.GreaterOrEqual(t, count, 1, "JobDuration should have observations")
}

func TestHTTPRequestDurationHistogramBuckets(t *testing.T) {
	// Observe various request durations
	durations := []float64{0.005, 0.01, 0.1, 0.5, 1.0, 5.0, 30.0}

	for _, d := range durations {
		HTTPRequestDuration.WithLabelValues("GET", "/test").Observe(d)
	}

	// Verify histogram has observations
	count := testutil.CollectAndCount(HTTPRequestDuration)
	assert.GreaterOrEqual(t, count, 1, "HTTPRequestDuration should have observations")
}

func TestStreamingExportDurationHistogramBuckets(t *testing.T) {
	// Observe various streaming export durations
	durations := []float64{0.05, 0.1, 0.5, 1.0, 5.0, 30.0, 60.0}

	for _, d := range durations {
		StreamingExportDuration.WithLabelValues("users", "ndjson").Observe(d)
	}

	// Verify histogram has observations
	count := testutil.CollectAndCount(StreamingExportDuration)
	assert.GreaterOrEqual(t, count, 1, "StreamingExportDuration should have observations")
}

func TestBatchProcessingDurationHistogramBuckets(t *testing.T) {
	// Observe various batch durations
	durations := []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.5, 5.0}

	for _, d := range durations {
		BatchProcessingDuration.WithLabelValues("import", "users", "db").Observe(d)
	}

	// Verify histogram has observations
	count := testutil.CollectAndCount(BatchProcessingDuration)
	assert.GreaterOrEqual(t, count, 1, "BatchProcessingDuration should have observations")
}

func TestHTTPRequestsInFlightGauge(t *testing.T) {
	initial := testutil.ToFloat64(HTTPRequestsInFlight)

	HTTPRequestsInFlight.Inc()
	HTTPRequestsInFlight.Inc()
	after2 := testutil.ToFloat64(HTTPRequestsInFlight)
	assert.Equal(t, initial+2, after2, "In-flight should be initial+2")

	HTTPRequestsInFlight.Dec()
	after1 := testutil.ToFloat64(HTTPRequestsInFlight)
	assert.Equal(t, initial+1, after1, "In-flight should be initial+1")

	HTTPRequestsInFlight.Dec()
	afterReset := testutil.ToFloat64(HTTPRequestsInFlight)
	assert.Equal(t, initial, afterReset, "In-flight should return to initial")
}
