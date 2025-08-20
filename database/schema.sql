-- AI Resume Builder Database Schema
-- PostgreSQL

-- Users table
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    auth_provider VARCHAR(50) DEFAULT 'email',
    google_id VARCHAR(255),
    profile_picture VARCHAR(500),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Resume data table
CREATE TABLE resumes (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(100),
    summary TEXT,
    skills TEXT,
    selected_format VARCHAR(50) DEFAULT 'temp1',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Experience entries table
CREATE TABLE experiences (
    id SERIAL PRIMARY KEY,
    resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
    job_title VARCHAR(255) NOT NULL,
    company VARCHAR(255) NOT NULL,
    city VARCHAR(100),
    state VARCHAR(100),
    start_date DATE,
    end_date DATE,
    currently_working BOOLEAN DEFAULT FALSE,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Education entries table
CREATE TABLE education (
    id SERIAL PRIMARY KEY,
    resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
    degree VARCHAR(255) NOT NULL,
    school VARCHAR(255) NOT NULL,
    field VARCHAR(255),
    graduation_year INTEGER,
    gpa VARCHAR(20),
    honors TEXT,
    location VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Resume History table
CREATE TABLE resume_history (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_id INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
    resume_name VARCHAR(255) NOT NULL,
    s3_path VARCHAR(500) NOT NULL,
    personal_data JSONB, -- Stores additional personal data from resume
    generated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Job Applications table for one-click apply feature
-- Note: resume_id references resume_history.id (not resumes.id) since it's the generated PDF that's used
CREATE TABLE job_applications (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_id INTEGER, -- References resume_history.id (no FK constraint for flexibility)
    job_url VARCHAR(500) NOT NULL,
    company_name VARCHAR(255),
    position_title VARCHAR(255),
    application_status VARCHAR(50) DEFAULT 'submitted',
    application_code VARCHAR(50), -- Unique code for tracking applications
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    job_page_screenshot_url VARCHAR(500), -- S3 URL of job page screenshot
    application_screenshot_url VARCHAR(500), -- S3 URL of application form screenshot
    confirmation_screenshot_key TEXT, -- S3 key for confirmation screenshot
    application_screenshot_key TEXT, -- S3 key for application screenshot
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- User Job Profiles table for application automation (populated from resume data)
CREATE TABLE user_job_profiles (
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
    work_authorization VARCHAR(50) DEFAULT 'yes', -- 'yes', 'no', 'requires_sponsorship'
    requires_sponsorship BOOLEAN DEFAULT false,
    willing_to_relocate BOOLEAN DEFAULT false,
    salary_expectation_min INTEGER,
    salary_expectation_max INTEGER,
    preferred_locations TEXT, -- JSON array of preferred cities/states
    available_start_date VARCHAR(50) DEFAULT 'immediately', -- 'immediately', '2_weeks', '1_month', 'specific_date'
    years_of_experience INTEGER DEFAULT 0,
    gender VARCHAR(20), -- 'male', 'female', 'non_binary', 'other', 'prefer_not_to_say'
    ethnicity VARCHAR(100), -- demographic information for diversity tracking
    veteran_status VARCHAR(50), -- 'yes', 'no', 'prefer_not_to_say'
    disability_status VARCHAR(50), -- 'yes', 'no', 'prefer_not_to_say'
    sexual_orientation VARCHAR(50), -- Sexual orientation for diversity tracking
    transgender_status VARCHAR(50), -- 'Yes', 'No', 'Prefer not to answer'
    most_recent_degree VARCHAR(100), -- 'Bachelor''s', 'Master''s', 'PhD', etc.
    graduation_year INTEGER, -- Year of graduation
    university VARCHAR(255), -- University/Institution name
    major VARCHAR(255), -- Major/Field of study
    extra_qa JSONB DEFAULT '{}', -- Stores additional Q&A pairs from job applications
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

-- Removed user_job_preferences table
-- User profile data should come from resume input only

-- Removed job_automation_logs and job_form_requirements tables
-- Job application status and any error logs are stored directly in job_applications table


-- Indexes for better performance
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_google_id ON users(google_id);
CREATE INDEX idx_users_auth_provider ON users(auth_provider);
CREATE INDEX idx_resumes_user_id ON resumes(user_id);
CREATE INDEX idx_experiences_resume_id ON experiences(resume_id);
CREATE INDEX idx_education_resume_id ON education(resume_id);
CREATE INDEX idx_resume_history_user_id ON resume_history(user_id);
CREATE INDEX idx_resume_history_generated_at ON resume_history(generated_at DESC);

-- Job application indexes
CREATE INDEX idx_job_applications_user_id ON job_applications(user_id);
CREATE INDEX idx_job_applications_applied_at ON job_applications(applied_at DESC);
CREATE INDEX idx_job_applications_status ON job_applications(application_status);
CREATE INDEX idx_job_applications_code ON job_applications(application_code);

-- Job profile indexes
CREATE INDEX idx_user_job_profiles_user_id ON user_job_profiles(user_id);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for all tables with updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_resumes_updated_at BEFORE UPDATE ON resumes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_experiences_updated_at BEFORE UPDATE ON experiences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_education_updated_at BEFORE UPDATE ON education
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_job_applications_updated_at BEFORE UPDATE ON job_applications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_job_profiles_updated_at BEFORE UPDATE ON user_job_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

 