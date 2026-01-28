-- Create comments table
CREATE TABLE IF NOT EXISTS comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    body TEXT NOT NULL,
    article_id UUID NOT NULL,
    user_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT comments_article_fk FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
    CONSTRAINT comments_user_fk FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create index on article_id for FK joins
CREATE INDEX IF NOT EXISTS idx_comments_article_id ON comments(article_id);

-- Create index on user_id for FK joins
CREATE INDEX IF NOT EXISTS idx_comments_user_id ON comments(user_id);

-- Create index on created_at for ordering
CREATE INDEX IF NOT EXISTS idx_comments_created_at ON comments(created_at);
