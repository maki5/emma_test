-- Re-add role constraint to users table
-- Note: This will fail if there are rows with roles not in the allowed list
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'user', 'moderator'));
