-- Add email_unsubscribed column to users table for email opt-out functionality
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_unsubscribed BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_unsubscribed_at TIMESTAMP;
