-- ========================================
-- JOB AUTOMATION SYSTEM - DATABASE MIGRATION
-- Single migration file for all required changes
-- ========================================

-- Add new columns to user_job_profiles table
ALTER TABLE user_job_profiles
ADD COLUMN IF NOT EXISTS sexual_orientation VARCHAR(50),
ADD COLUMN IF NOT EXISTS transgender_status VARCHAR(50),
ADD COLUMN IF NOT EXISTS most_recent_degree VARCHAR(100),
ADD COLUMN IF NOT EXISTS graduation_year INTEGER,
ADD COLUMN IF NOT EXISTS university VARCHAR(255),
ADD COLUMN IF NOT EXISTS major VARCHAR(255),
ADD COLUMN IF NOT EXISTS extra_qa JSONB DEFAULT '{}';

-- Add application tracking fields
ALTER TABLE job_applications
ADD COLUMN IF NOT EXISTS application_code VARCHAR(50),
ADD COLUMN IF NOT EXISTS confirmation_screenshot_key TEXT,
ADD COLUMN IF NOT EXISTS application_screenshot_key TEXT;

-- Add resume tracking to history
ALTER TABLE resume_history
ADD COLUMN IF NOT EXISTS resume_id INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS personal_data JSONB;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_job_applications_code ON job_applications(application_code);
CREATE INDEX IF NOT EXISTS idx_resume_history_resume_id ON resume_history(resume_id);

-- Add column comments
COMMENT ON COLUMN user_job_profiles.sexual_orientation IS 'Sexual orientation for diversity tracking';
COMMENT ON COLUMN user_job_profiles.transgender_status IS 'Transgender status: Yes, No, Prefer not to answer';
COMMENT ON COLUMN user_job_profiles.most_recent_degree IS 'Highest degree obtained';
COMMENT ON COLUMN user_job_profiles.graduation_year IS 'Year of graduation';
COMMENT ON COLUMN user_job_profiles.university IS 'University/Institution name';
COMMENT ON COLUMN user_job_profiles.major IS 'Major/Field of study';
COMMENT ON COLUMN user_job_profiles.extra_qa IS 'Stores additional Q&A pairs from job applications';
COMMENT ON COLUMN job_applications.application_code IS 'Unique code for tracking applications';
COMMENT ON COLUMN job_applications.confirmation_screenshot_key IS 'S3 key for confirmation screenshot';
COMMENT ON COLUMN job_applications.application_screenshot_key IS 'S3 key for application screenshot';
COMMENT ON COLUMN resume_history.resume_id IS 'Reference to the resume used';
COMMENT ON COLUMN resume_history.personal_data IS 'Additional personal data from resume';