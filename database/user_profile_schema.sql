-- User Profile table for essential job application data
-- This stores key information extracted when user downloads resume
-- Detailed experience is extracted from S3 files when needed

CREATE TABLE user_profiles (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    
    -- Personal Information
    full_name VARCHAR(255),
    phone VARCHAR(100),
    location_city VARCHAR(100),
    location_state VARCHAR(100),
    location_country VARCHAR(100) DEFAULT 'United States',
    
    -- Professional Summary
    current_job_title VARCHAR(255),
    current_company VARCHAR(255),
    years_of_experience INTEGER DEFAULT 0,
    skills TEXT, -- JSON array of skills
    professional_summary TEXT,
    
    -- Education (highest/most recent)
    highest_degree VARCHAR(255),
    school VARCHAR(255),
    field_of_study VARCHAR(255),
    graduation_year INTEGER,
    gpa VARCHAR(20),
    
    -- Work Authorization & Availability
    work_authorization VARCHAR(255) DEFAULT 'Authorized to work in US',
    start_date_preference VARCHAR(100) DEFAULT 'Immediately',
    notice_period VARCHAR(100) DEFAULT '2 weeks',
    salary_expectation VARCHAR(100) DEFAULT 'Competitive',
    
    -- Social Links
    linkedin_url VARCHAR(500),
    portfolio_url VARCHAR(500),
    github_url VARCHAR(500),
    
    -- Resume Source Info
    latest_resume_s3_path VARCHAR(500), -- Link to latest resume for experience extraction
    last_extracted_at TIMESTAMP,
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_user_profiles_user_id ON user_profiles(user_id);
CREATE INDEX idx_user_profiles_updated_at ON user_profiles(updated_at DESC);

-- Trigger for updated_at
CREATE TRIGGER update_user_profiles_updated_at BEFORE UPDATE ON user_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();