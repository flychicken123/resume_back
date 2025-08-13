-- Migration script to add resume_history table
-- Run this script on existing databases to add the new resume history feature

-- Add resume_history table
CREATE TABLE IF NOT EXISTS resume_history (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_name VARCHAR(255) NOT NULL,
    s3_path VARCHAR(500) NOT NULL,
    generated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes for better performance
CREATE INDEX IF NOT EXISTS idx_resume_history_user_id ON resume_history(user_id);
CREATE INDEX IF NOT EXISTS idx_resume_history_generated_at ON resume_history(generated_at DESC);

-- Add comment to document the table
COMMENT ON TABLE resume_history IS 'Stores the last 3 generated resumes for each user';
COMMENT ON COLUMN resume_history.user_id IS 'Reference to the user who generated the resume';
COMMENT ON COLUMN resume_history.resume_name IS 'Display name for the resume';
COMMENT ON COLUMN resume_history.s3_path IS 'S3 URL or path to the generated PDF';
COMMENT ON COLUMN resume_history.generated_at IS 'When the resume was generated';
COMMENT ON COLUMN resume_history.created_at IS 'When the record was created';
