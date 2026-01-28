-- Create export_jobs table
CREATE TABLE IF NOT EXISTS export_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL,
    format VARCHAR(20) NOT NULL DEFAULT 'ndjson',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    total_records INTEGER NOT NULL DEFAULT 0,
    file_path TEXT,
    idempotency_token VARCHAR(255) NOT NULL,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT export_jobs_idempotency_token_unique UNIQUE (idempotency_token),
    CONSTRAINT export_jobs_resource_type_check CHECK (resource_type IN ('users', 'articles', 'comments')),
    CONSTRAINT export_jobs_format_check CHECK (format IN ('csv', 'ndjson')),
    CONSTRAINT export_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

-- Create index on idempotency_token for fast lookups
CREATE INDEX IF NOT EXISTS idx_export_jobs_idempotency_token ON export_jobs(idempotency_token);

-- Create index on status for filtering
CREATE INDEX IF NOT EXISTS idx_export_jobs_status ON export_jobs(status);

-- Create index on created_at for ordering
CREATE INDEX IF NOT EXISTS idx_export_jobs_created_at ON export_jobs(created_at);
