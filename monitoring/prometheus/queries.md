# PromQL Queries Reference

Quick reference for querying Bulk Import/Export API metrics in Prometheus.

**Prometheus UI**: http://localhost:9090

---

## HTTP Metrics

### Request Rate (requests/second)
```promql
rate(bulk_import_export_http_requests_total[5m])
```

### Request Rate by Endpoint
```promql
sum by (method, path) (rate(bulk_import_export_http_requests_total[5m]))
```

### Total Requests (last hour)
```promql
increase(bulk_import_export_http_requests_total[1h])
```

### Error Rate (5xx errors)
```promql
rate(bulk_import_export_http_requests_total{status=~"5.."}[5m])
```

### Error Rate Percentage
```promql
sum(rate(bulk_import_export_http_requests_total{status=~"5.."}[5m])) 
/ 
sum(rate(bulk_import_export_http_requests_total[5m])) * 100
```

### Success Rate Percentage
```promql
sum(rate(bulk_import_export_http_requests_total{status=~"2.."}[5m])) 
/ 
sum(rate(bulk_import_export_http_requests_total[5m])) * 100
```

### Requests Currently In-Flight
```promql
bulk_import_export_http_requests_in_flight
```

---

## Latency Metrics

### Average Request Duration
```promql
rate(bulk_import_export_http_request_duration_seconds_sum[5m]) 
/ 
rate(bulk_import_export_http_request_duration_seconds_count[5m])
```

### P50 Latency (Median)
```promql
histogram_quantile(0.50, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le))
```

### P90 Latency
```promql
histogram_quantile(0.90, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le))
```

### P99 Latency
```promql
histogram_quantile(0.99, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le))
```

### P99 Latency by Endpoint
```promql
histogram_quantile(0.99, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le, path))
```

### Requests Slower Than 1 Second
```promql
sum(rate(bulk_import_export_http_request_duration_seconds_bucket{le="1"}[5m])) 
/ 
sum(rate(bulk_import_export_http_request_duration_seconds_count[5m]))
```

---

## Job Metrics (Import/Export)

### Jobs Currently In Progress
```promql
bulk_import_export_jobs_in_progress
```

### Jobs In Progress by Type
```promql
sum by (job_type, resource_type) (bulk_import_export_jobs_in_progress)
```

### Job Completion Rate (jobs/minute)
```promql
rate(bulk_import_export_jobs_total[5m]) * 60
```

### Job Completion by Status
```promql
sum by (status) (rate(bulk_import_export_jobs_total[5m]))
```

### Job Failure Rate
```promql
sum(rate(bulk_import_export_jobs_total{status=~"failed|completed_with_errors"}[5m])) 
/ 
sum(rate(bulk_import_export_jobs_total[5m])) * 100
```

### Average Job Duration
```promql
rate(bulk_import_export_jobs_duration_seconds_sum[5m]) 
/ 
rate(bulk_import_export_jobs_duration_seconds_count[5m])
```

### P99 Job Duration
```promql
histogram_quantile(0.99, sum(rate(bulk_import_export_jobs_duration_seconds_bucket[5m])) by (le))
```

### P99 Job Duration by Type
```promql
histogram_quantile(0.99, sum(rate(bulk_import_export_jobs_duration_seconds_bucket[5m])) by (le, job_type, resource_type))
```

---

## Record Processing Metrics

### Records Processed per Second
```promql
rate(bulk_import_export_records_processed_total[5m])
```

### Records Processed per Minute
```promql
rate(bulk_import_export_records_processed_total[5m]) * 60
```

### Records by Result (success/failure)
```promql
sum by (result) (rate(bulk_import_export_records_processed_total[5m]))
```

### Record Success Rate
```promql
sum(rate(bulk_import_export_records_processed_total{result="success"}[5m])) 
/ 
sum(rate(bulk_import_export_records_processed_total[5m])) * 100
```

### Total Records Processed (last hour)
```promql
increase(bulk_import_export_records_processed_total[1h])
```

### Average Batch Processing Duration
```promql
rate(bulk_import_export_records_batch_duration_seconds_sum[5m]) 
/ 
rate(bulk_import_export_records_batch_duration_seconds_count[5m])
```

---

## Streaming Export Metrics

### Active Streaming Exports
```promql
bulk_import_export_streaming_exports_in_flight
```

### Streaming Exports per Minute
```promql
rate(bulk_import_export_streaming_exports_total[5m]) * 60
```

### Streaming Export Errors
```promql
rate(bulk_import_export_streaming_exports_total{result="error"}[5m])
```

### Average Streaming Export Duration
```promql
rate(bulk_import_export_streaming_export_duration_seconds_sum[5m]) 
/ 
rate(bulk_import_export_streaming_export_duration_seconds_count[5m])
```

### P99 Streaming Export Duration
```promql
histogram_quantile(0.99, sum(rate(bulk_import_export_streaming_export_duration_seconds_bucket[5m])) by (le))
```

### Records Streamed per Second
```promql
rate(bulk_import_export_streaming_records_total[5m])
```

---

## Database Connection Pool

### Current Pool Usage
```promql
bulk_import_export_db_pool_connections
```

### Connections In Use
```promql
bulk_import_export_db_pool_connections{state="in_use"}
```

### Idle Connections
```promql
bulk_import_export_db_pool_connections{state="idle"}
```

### Pool Utilization Percentage
```promql
bulk_import_export_db_pool_connections{state="in_use"} 
/ 
bulk_import_export_db_pool_connections{state="total"} * 100
```

---

## Service Health

### Service Up/Down
```promql
up{job="bulk-import-export"}
```

### Service Availability (last 5 minutes)
```promql
avg_over_time(up{job="bulk-import-export"}[5m]) * 100
```

### Scrape Duration
```promql
scrape_duration_seconds{job="bulk-import-export"}
```

---

## Useful Aggregate Queries

### Top 5 Slowest Endpoints (P99)
```promql
topk(5, histogram_quantile(0.99, sum(rate(bulk_import_export_http_request_duration_seconds_bucket[5m])) by (le, path)))
```

### Top 5 Busiest Endpoints
```promql
topk(5, sum by (path) (rate(bulk_import_export_http_requests_total[5m])))
```

### Import vs Export Job Comparison
```promql
sum by (job_type) (rate(bulk_import_export_jobs_total[5m]))
```

### Resource Type Processing Comparison
```promql
sum by (resource_type) (rate(bulk_import_export_records_processed_total[5m]))
```

---

## Quick Health Check Queries

Run these to verify the system is healthy:

```promql
# Service is up
up{job="bulk-import-export"} == 1

# No jobs stuck
bulk_import_export_jobs_in_progress < 10

# Pool not exhausted
bulk_import_export_db_pool_connections{state="idle"} > 0

# Error rate low
sum(rate(bulk_import_export_http_requests_total{status=~"5.."}[5m])) < 0.1
```
