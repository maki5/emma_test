-- Remove role constraint from users table
-- Role validation is handled at the application level for better performance
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
