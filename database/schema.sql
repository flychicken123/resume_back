-- AI Resume Builder Database Schema
-- PostgreSQL

-- Comments for Google OAuth fields

-- Users table
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    auth_provider VARCHAR(50) DEFAULT 'email',
    google_id VARCHAR(255),
    profile_picture VARCHAR(500),
    email_unsubscribed BOOLEAN DEFAULT FALSE,
    email_unsubscribed_at TIMESTAMP,
    job_preferences JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Resume data table
CREATE TABLE resumes (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_name VARCHAR(255) NOT NULL DEFAULT 'User Resume',
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(100),
    summary TEXT,
    skills TEXT,
    skills_categorized TEXT,
    experience TEXT,
    education TEXT,
    job_description TEXT,
    location VARCHAR(255),
    selected_format VARCHAR(50) DEFAULT 'classic-professional',
    last_resume_html TEXT DEFAULT '',
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

-- Projects table (added in migration 002)
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
    project_name VARCHAR(255) NOT NULL,
    description TEXT,
    technologies TEXT,
    project_url VARCHAR(500),
    role VARCHAR(255),
    start_date DATE,
    end_date DATE,
    is_current BOOLEAN DEFAULT FALSE,
    display_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Resume History table
CREATE TABLE resume_history (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    resume_name VARCHAR(255) NOT NULL,
    s3_path VARCHAR(500) NOT NULL,
    generated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for better performance
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_google_id ON users(google_id);
CREATE INDEX idx_users_auth_provider ON users(auth_provider);
CREATE INDEX idx_resumes_user_id ON resumes(user_id);
CREATE UNIQUE INDEX idx_resumes_user_unique ON resumes(user_id);
CREATE INDEX idx_experiences_resume_id ON experiences(resume_id);
CREATE INDEX idx_education_resume_id ON education(resume_id);
CREATE INDEX idx_projects_resume_id ON projects(resume_id);
CREATE INDEX idx_resume_history_user_id ON resume_history(user_id);
CREATE INDEX idx_resume_history_generated_at ON resume_history(generated_at DESC);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_resumes_updated_at BEFORE UPDATE ON resumes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_experiences_updated_at BEFORE UPDATE ON experiences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_education_updated_at BEFORE UPDATE ON education
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column(); 
