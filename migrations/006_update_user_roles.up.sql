-- Update users role constraint to include all valid roles
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'user', 'moderator', 'manager', 'reader', 'editor', 'author'));
