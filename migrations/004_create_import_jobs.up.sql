-- Create import_jobs table
CREATE TABLE IF NOT EXISTS import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    total_records INTEGER NOT NULL DEFAULT 0,
    processed_records INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    idempotency_token VARCHAR(255) NOT NULL,
    metadata JSONB,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT import_jobs_idempotency_token_unique UNIQUE (idempotency_token),
    CONSTRAINT import_jobs_resource_type_check CHECK (resource_type IN ('users', 'articles', 'comments')),
    CONSTRAINT import_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'completed_with_errors', 'failed'))
);

-- Create index on idempotency_token for fast lookups
CREATE INDEX IF NOT EXISTS idx_import_jobs_idempotency_token ON import_jobs(idempotency_token);

-- Create index on status for filtering
CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON import_jobs(status);

-- Create index on created_at for ordering
CREATE INDEX IF NOT EXISTS idx_import_jobs_created_at ON import_jobs(created_at);
