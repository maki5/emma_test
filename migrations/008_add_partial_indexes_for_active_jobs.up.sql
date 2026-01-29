-- Add partial indexes for active jobs (not completed/failed)
-- These indexes are much smaller than full indexes since most jobs are historical.
-- Performance benefit: Queries for active jobs (monitoring, cleanup, processing)
-- only need to scan a small subset of the data.

-- Partial index on import_jobs for active jobs only
-- Covers: pending, processing, completed_with_errors (jobs that may need attention)
CREATE INDEX IF NOT EXISTS idx_import_jobs_active
    ON import_jobs(status)
    WHERE status NOT IN ('completed', 'failed');

-- Partial index on export_jobs for active jobs only
-- Covers: pending, processing (jobs that are in-flight)
CREATE INDEX IF NOT EXISTS idx_export_jobs_active
    ON export_jobs(status)
    WHERE status NOT IN ('completed', 'failed');

-- Composite index for job querying by resource_type and status
-- Useful for: "Find all pending user imports"
CREATE INDEX IF NOT EXISTS idx_import_jobs_resource_status
    ON import_jobs(resource_type, status);

CREATE INDEX IF NOT EXISTS idx_export_jobs_resource_status
    ON export_jobs(resource_type, status);
