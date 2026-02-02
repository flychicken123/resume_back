-- Remove email_unsubscribed columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS email_unsubscribed_at;
ALTER TABLE users DROP COLUMN IF EXISTS email_unsubscribed;
