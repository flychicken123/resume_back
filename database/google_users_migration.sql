-- Migration to add Google OAuth specific fields to users table
-- This helps track Google users better

-- Add Google-specific fields to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider VARCHAR(50) DEFAULT 'email';
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_id VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS profile_picture VARCHAR(500);

-- Add index for Google ID lookups
CREATE INDEX IF NOT EXISTS idx_users_google_id ON users(google_id);

-- Add index for auth provider lookups
CREATE INDEX IF NOT EXISTS idx_users_auth_provider ON users(auth_provider);

-- Update existing Google OAuth users to have the correct auth_provider
UPDATE users SET auth_provider = 'google' WHERE password = 'google_oauth_user';

-- Add comment to explain the auth_provider field
COMMENT ON COLUMN users.auth_provider IS 'Authentication provider: email, google, etc.';
COMMENT ON COLUMN users.google_id IS 'Google OAuth user ID';
COMMENT ON COLUMN users.profile_picture IS 'User profile picture URL from Google';
