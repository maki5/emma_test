-- Create articles table
CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    body TEXT NOT NULL,
    tags TEXT[],
    author_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    published_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT articles_slug_unique UNIQUE (slug),
    CONSTRAINT articles_status_check CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT articles_author_fk FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create index on slug for fast lookups
CREATE INDEX IF NOT EXISTS idx_articles_slug ON articles(slug);

-- Create index on author_id for FK joins
CREATE INDEX IF NOT EXISTS idx_articles_author_id ON articles(author_id);

-- Create index on status for filtering
CREATE INDEX IF NOT EXISTS idx_articles_status ON articles(status);

-- Create index on created_at for ordering
CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles(created_at);
