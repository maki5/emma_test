# Metrics and Observability

The Bulk Import/Export API includes comprehensive Prometheus metrics for observability with pre-configured Grafana dashboards.

## Quick Start

### Start the Monitoring Stack

```bash
# Start all services (PostgreSQL + Prometheus + Grafana)
make start-all

# Or just monitoring (if DB is already running)
make monitoring-start
```

### Access Points

| Service     | URL                          | Notes                        |
|-------------|------------------------------|------------------------------|
| API         | http://localhost:8080        | Main application             |
| Metrics     | http://localhost:8080/metrics| Raw Prometheus metrics       |
| Prometheus  | http://localhost:9090        | Query UI and alerts          |
| Grafana     | http://localhost:3000        | Dashboards (admin/admin)     |

## Available Metrics

### HTTP Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bulk_import_export_http_requests_total` | Counter | Total HTTP requests (labels: method, path, status) |
| `bulk_import_export_http_request_duration_seconds` | Histogram | Request latency (labels: method, path) |
| `bulk_import_export_http_requests_in_flight` | Gauge | Currently processing requests |

### Job Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bulk_import_export_jobs_total` | Counter | Completed jobs (labels: job_type, resource_type, status) |
| `bulk_import_export_jobs_in_progress` | Gauge | Currently running jobs |
| `bulk_import_export_jobs_duration_seconds` | Histogram | Job processing time |

### Record Processing Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bulk_import_export_records_processed_total` | Counter | Total records processed (labels: job_type, resource_type, result) |
| `bulk_import_export_records_batch_duration_seconds` | Histogram | Batch processing time |

### Streaming Export Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bulk_import_export_streaming_exports_total` | Counter | Total streaming exports (labels: resource_type, format, result) |
| `bulk_import_export_streaming_export_duration_seconds` | Histogram | Streaming export duration |
| `bulk_import_export_streaming_records_total` | Counter | Total records streamed (labels: resource_type, format) |
| `bulk_import_export_streaming_exports_in_flight` | Gauge | Currently active streaming exports |

### Database Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `bulk_import_export_db_pool_connections` | Gauge | Connection pool stats (labels: state=total/idle/in_use) |

## Alerting Rules

Prometheus alerting rules are configured in `monitoring/prometheus/alerts.yml`:

| Alert | Severity | Description |
|-------|----------|-------------|
| `HighHTTPErrorRate` | warning | HTTP 5xx error rate > 5% |
| `CriticalHTTPErrorRate` | critical | HTTP 5xx error rate > 15% |
| `HighHTTPLatency` | warning | P99 latency > 5 seconds |
| `HighJobFailureRate` | warning | Job failure rate > 10% |
| `JobProcessingStuck` | warning | Jobs in progress but none completing |
| `HighRecordFailureRate` | warning | Record failure rate > 10% |
| `DatabaseConnectionPoolExhaustion` | warning | Pool > 90% utilized |
| `DatabaseConnectionPoolCritical` | critical | Pool > 98% utilized |
| `NoIdleDatabaseConnections` | warning | All connections in use |
| `StreamingExportErrors` | warning | Streaming exports failing |
| `SlowStreamingExports` | warning | P99 streaming export > 30s |
| `ServiceDown` | critical | Service unreachable |
| `ServiceUnstable` | warning | Service < 90% availability |

View active alerts at: http://localhost:9090/alerts

## PromQL Queries

For a complete list of useful queries, see:
**[monitoring/prometheus/queries.md](../monitoring/prometheus/queries.md)**

Quick examples:

```promql
# Request rate
rate(bulk_import_export_http_requests_total[5m])

# Error rate percentage
sum(rate(bulk_import_export_http_requests_total{status=~"5.."}[5m])) 
/ sum(rate(bulk_import_export_http_requests_total[5m])) * 100

# P99 latency
histogram_quantile(0.99, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le))

# Jobs in progress
bulk_import_export_jobs_in_progress

# DB pool utilization
bulk_import_export_db_pool_connections{state="in_use"} 
/ bulk_import_export_db_pool_connections{state="total"} * 100
```

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Application   │────►│   Prometheus    │────►│    Grafana      │
│   :8080/metrics │     │     :9090       │     │     :3000       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
         │                      │
         │                      ▼
         │               ┌─────────────────┐
         │               │  Alerting Rules │
         │               │  (alerts.yml)   │
         │               └─────────────────┘
         ▼
┌─────────────────┐
│   PostgreSQL    │
│     :5432       │
└─────────────────┘
```

- **Application** exposes `/metrics` endpoint in Prometheus format
- **Prometheus** scrapes metrics every 15 seconds and evaluates alerts
- **Grafana** visualizes metrics with pre-configured dashboards

## Grafana Dashboards

Dashboards are automatically provisioned when Grafana starts. No manual setup required!

**Pre-configured panels include:**
- Service health and availability
- HTTP request rates by endpoint and status code
- Request latency (P50, P90, P99)
- Success/error rates
- Jobs in progress by type
- Job completion rates
- Record processing rates
- Streaming export metrics
- Database connection pool utilization

**Provisioning files:**
- `monitoring/grafana/provisioning/datasources/datasources.yml` - Prometheus datasource
- `monitoring/grafana/provisioning/dashboards/dashboards.yml` - Dashboard provider config
- `monitoring/grafana/provisioning/dashboards/bulk-import-export.json` - Dashboard definition

## Configuration

### Prometheus Configuration

`monitoring/prometheus/prometheus.yml`:
- Scrape interval: 15 seconds
- Targets: `host.docker.internal:8080` (the Go app)
- Alert rules: `monitoring/prometheus/alerts.yml`

## Extending Metrics

To add new metrics, modify `internal/metrics/metrics.go`:

```go
var MyNewMetric = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: namespace,
        Subsystem: "my_subsystem",
        Name:      "my_metric_total",
        Help:      "Description of what this measures",
    },
    []string{"label1", "label2"},
)
```

Then use it in your code:

```go
metrics.MyNewMetric.WithLabelValues("value1", "value2").Inc()
```
