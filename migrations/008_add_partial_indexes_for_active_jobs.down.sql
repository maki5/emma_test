-- Remove partial indexes for active jobs

DROP INDEX IF EXISTS idx_export_jobs_resource_status;
DROP INDEX IF EXISTS idx_import_jobs_resource_status;
DROP INDEX IF EXISTS idx_export_jobs_active;
DROP INDEX IF EXISTS idx_import_jobs_active;
