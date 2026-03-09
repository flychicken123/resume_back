-- Add job_preferences JSONB column to users table
-- Stores work authorization answers and other job application preferences
ALTER TABLE users ADD COLUMN IF NOT EXISTS job_preferences JSONB DEFAULT '{}';
