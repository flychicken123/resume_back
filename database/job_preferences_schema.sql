-- User Application Preferences Table
-- Stores user's common answers to job application questions
CREATE TABLE IF NOT EXISTS user_application_preferences (
    id SERIAL PRIMARY KEY,
    user_email VARCHAR(255) NOT NULL,
    field_key VARCHAR(255) NOT NULL, -- e.g., "years_experience", "salary_expectation"
    field_value TEXT NOT NULL,
    field_type VARCHAR(50), -- text, number, select, checkbox, radio
    field_label TEXT, -- Human readable label
    confidence_score FLOAT DEFAULT 1.0, -- How confident we are in this value
    usage_count INT DEFAULT 0, -- How many times this has been used
    last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_email, field_key)
);

-- Job Application Form Fields Table
-- Stores detected form fields from job applications
CREATE TABLE IF NOT EXISTS job_form_fields (
    id SERIAL PRIMARY KEY,
    platform VARCHAR(100) NOT NULL, -- LinkedIn, Greenhouse, etc.
    company VARCHAR(255),
    field_id VARCHAR(255), -- HTML field ID or name
    field_label TEXT, -- Label text
    field_type VARCHAR(50), -- input type
    field_options TEXT, -- JSON array for select/radio options
    field_validation TEXT, -- JSON validation rules
    is_required BOOLEAN DEFAULT false,
    mapped_to VARCHAR(255), -- Maps to our standard field (e.g., "first_name")
    frequency INT DEFAULT 1, -- How often we've seen this field
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Application History Table
-- Stores what was submitted for each application
CREATE TABLE IF NOT EXISTS application_submissions (
    id SERIAL PRIMARY KEY,
    application_id VARCHAR(255) UNIQUE NOT NULL,
    user_email VARCHAR(255) NOT NULL,
    job_url TEXT NOT NULL,
    job_title VARCHAR(255),
    company VARCHAR(255),
    platform VARCHAR(100),
    form_data TEXT, -- JSON of all submitted form data
    status VARCHAR(50), -- submitted, pending_info, failed, manual_required
    missing_fields TEXT, -- JSON array of fields we couldn't fill
    submission_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    success_rate FLOAT, -- Percentage of fields we could auto-fill
    notes TEXT
);

-- Smart Field Mappings Table
-- Maps various field names to our standard fields
CREATE TABLE IF NOT EXISTS field_mappings (
    id SERIAL PRIMARY KEY,
    external_field VARCHAR(255) NOT NULL, -- Field name from job site
    standard_field VARCHAR(255) NOT NULL, -- Our standard field name
    platform VARCHAR(100), -- Platform specific mapping
    confidence FLOAT DEFAULT 1.0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(external_field, platform)
);

-- Insert common field mappings
INSERT INTO field_mappings (external_field, standard_field, platform) VALUES
-- Name mappings
('first_name', 'first_name', NULL),
('firstName', 'first_name', NULL),
('fname', 'first_name', NULL),
('given_name', 'first_name', NULL),
('last_name', 'last_name', NULL),
('lastName', 'last_name', NULL),
('lname', 'last_name', NULL),
('surname', 'last_name', NULL),
('family_name', 'last_name', NULL),

-- Contact mappings
('email', 'email', NULL),
('email_address', 'email', NULL),
('phone', 'phone', NULL),
('phone_number', 'phone', NULL),
('mobile', 'phone', NULL),
('telephone', 'phone', NULL),

-- Experience mappings
('years_of_experience', 'total_experience', NULL),
('total_experience', 'total_experience', NULL),
('experience_years', 'total_experience', NULL),
('work_experience', 'total_experience', NULL),

-- Salary mappings
('salary_expectation', 'expected_salary', NULL),
('expected_salary', 'expected_salary', NULL),
('desired_salary', 'expected_salary', NULL),
('compensation_expectation', 'expected_salary', NULL),

-- Availability mappings
('start_date', 'availability', NULL),
('available_date', 'availability', NULL),
('when_can_start', 'availability', NULL),
('notice_period', 'notice_period', NULL),

-- Location mappings
('current_location', 'location', NULL),
('city', 'city', NULL),
('state', 'state', NULL),
('country', 'country', NULL),
('zip', 'zip_code', NULL),
('postal_code', 'zip_code', NULL),

-- Work authorization
('work_authorization', 'work_authorization', NULL),
('visa_status', 'work_authorization', NULL),
('eligible_to_work', 'work_authorization', NULL),
('citizenship', 'citizenship', NULL),

-- LinkedIn specific
('linkedin_profile', 'linkedin_url', 'LinkedIn'),
('linkedin', 'linkedin_url', NULL),

-- Greenhouse specific
('gh_src', 'referral_source', 'Greenhouse'),
('how_did_you_hear', 'referral_source', NULL),

-- Common questions
('why_interested', 'why_interested', NULL),
('cover_letter', 'cover_letter', NULL),
('portfolio', 'portfolio_url', NULL),
('website', 'portfolio_url', NULL),
('github', 'github_url', NULL),

-- Diversity questions
('gender', 'gender', NULL),
('race_ethnicity', 'ethnicity', NULL),
('veteran_status', 'veteran_status', NULL),
('disability_status', 'disability_status', NULL)
ON CONFLICT (external_field, platform) DO NOTHING;

-- Create indexes for performance
CREATE INDEX idx_user_preferences_email ON user_application_preferences(user_email);
CREATE INDEX idx_form_fields_platform ON job_form_fields(platform);
CREATE INDEX idx_submissions_user ON application_submissions(user_email);
CREATE INDEX idx_submissions_status ON application_submissions(status);
CREATE INDEX idx_field_mappings_external ON field_mappings(external_field);
CREATE INDEX idx_field_mappings_standard ON field_mappings(standard_field);