-- ========================================
-- JOB AUTOMATION SYSTEM - COMPLETE MIGRATION
-- Single file for all tables and migrations
-- ========================================

-- Create user_job_profiles table if it doesn't exist
CREATE TABLE IF NOT EXISTS user_job_profiles (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(255),
    email VARCHAR(255),
    phone_number VARCHAR(20),
    country VARCHAR(100) DEFAULT 'United States',
    city VARCHAR(100),
    state VARCHAR(100),
    zip_code VARCHAR(20),
    address VARCHAR(255),
    linkedin_url VARCHAR(500),
    portfolio_url VARCHAR(500),
    work_authorization VARCHAR(50) DEFAULT 'yes',
    requires_sponsorship BOOLEAN DEFAULT false,
    willing_to_relocate BOOLEAN DEFAULT false,
    salary_expectation_min INTEGER,
    salary_expectation_max INTEGER,
    preferred_locations TEXT,
    available_start_date VARCHAR(50) DEFAULT 'immediately',
    years_of_experience INTEGER DEFAULT 0,
    gender VARCHAR(20),
    ethnicity VARCHAR(100),
    veteran_status VARCHAR(50),
    disability_status VARCHAR(50),
    sexual_orientation VARCHAR(50),
    transgender_status VARCHAR(50),
    most_recent_degree VARCHAR(100),
    graduation_year INTEGER,
    university VARCHAR(255),
    major VARCHAR(255),
    extra_qa JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

-- Create job_applications table if it doesn't exist
CREATE TABLE IF NOT EXISTS job_applications (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_id INTEGER,
    job_url VARCHAR(500) NOT NULL,
    company_name VARCHAR(255),
    position_title VARCHAR(255),
    application_status VARCHAR(50) DEFAULT 'submitted',
    application_code VARCHAR(50),
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    job_page_screenshot_url VARCHAR(500),
    application_screenshot_url VARCHAR(500),
    confirmation_screenshot_key TEXT,
    application_screenshot_key TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add new columns to user_job_profiles if they don't exist
ALTER TABLE user_job_profiles
ADD COLUMN IF NOT EXISTS sexual_orientation VARCHAR(50),
ADD COLUMN IF NOT EXISTS transgender_status VARCHAR(50),
ADD COLUMN IF NOT EXISTS most_recent_degree VARCHAR(100),
ADD COLUMN IF NOT EXISTS graduation_year INTEGER,
ADD COLUMN IF NOT EXISTS university VARCHAR(255),
ADD COLUMN IF NOT EXISTS major VARCHAR(255),
ADD COLUMN IF NOT EXISTS extra_qa JSONB DEFAULT '{}';

-- Add application tracking fields if they don't exist
ALTER TABLE job_applications
ADD COLUMN IF NOT EXISTS application_code VARCHAR(50),
ADD COLUMN IF NOT EXISTS confirmation_screenshot_key TEXT,
ADD COLUMN IF NOT EXISTS application_screenshot_key TEXT;

-- Add resume tracking to history
ALTER TABLE resume_history
ADD COLUMN IF NOT EXISTS resume_id INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS personal_data JSONB;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_user_job_profiles_user_id ON user_job_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_job_applications_user_id ON job_applications(user_id);
CREATE INDEX IF NOT EXISTS idx_job_applications_applied_at ON job_applications(applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_applications_status ON job_applications(application_status);
CREATE INDEX IF NOT EXISTS idx_job_applications_code ON job_applications(application_code);
CREATE INDEX IF NOT EXISTS idx_resume_history_resume_id ON resume_history(resume_id);

-- Create or replace trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers if they don't exist
DROP TRIGGER IF EXISTS update_user_job_profiles_updated_at ON user_job_profiles;
CREATE TRIGGER update_user_job_profiles_updated_at 
    BEFORE UPDATE ON user_job_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_job_applications_updated_at ON job_applications;
CREATE TRIGGER update_job_applications_updated_at 
    BEFORE UPDATE ON job_applications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add column comments for documentation
COMMENT ON TABLE user_job_profiles IS 'Stores user profile data for job application automation';
COMMENT ON TABLE job_applications IS 'Tracks job applications submitted through the system';

COMMENT ON COLUMN user_job_profiles.country IS 'User country for job applications';
COMMENT ON COLUMN user_job_profiles.city IS 'User city for job applications';
COMMENT ON COLUMN user_job_profiles.state IS 'User state/province for job applications';
COMMENT ON COLUMN user_job_profiles.zip_code IS 'User postal/zip code for job applications';
COMMENT ON COLUMN user_job_profiles.address IS 'User street address for job applications';
COMMENT ON COLUMN user_job_profiles.work_authorization IS 'yes, no, or requires_sponsorship';
COMMENT ON COLUMN user_job_profiles.preferred_locations IS 'JSON array of preferred cities/states';
COMMENT ON COLUMN user_job_profiles.available_start_date IS 'immediately, 2_weeks, 1_month, or specific_date';
COMMENT ON COLUMN user_job_profiles.gender IS 'male, female, non_binary, other, prefer_not_to_say';
COMMENT ON COLUMN user_job_profiles.ethnicity IS 'Ethnicity for diversity tracking';
COMMENT ON COLUMN user_job_profiles.veteran_status IS 'yes, no, prefer_not_to_say';
COMMENT ON COLUMN user_job_profiles.disability_status IS 'yes, no, prefer_not_to_say';
COMMENT ON COLUMN user_job_profiles.sexual_orientation IS 'Sexual orientation for diversity tracking';
COMMENT ON COLUMN user_job_profiles.transgender_status IS 'Yes, No, Prefer not to answer';
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