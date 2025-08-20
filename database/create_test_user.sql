-- Create a test user for testing job applications
INSERT INTO users (username, email, password_hash, google_id, is_verified, created_at, updated_at)
VALUES (
    'testuser',
    'test@example.com',
    '$2a$10$YourHashedPasswordHere', -- This would be the bcrypt hash of the password
    NULL,
    true,
    NOW(),
    NOW()
) ON CONFLICT (email) DO NOTHING;

-- Get the user ID
WITH user_info AS (
    SELECT id FROM users WHERE email = 'test@example.com'
)
-- Create a job profile for the test user
INSERT INTO user_job_profiles (
    user_id,
    phone_number,
    country,
    city,
    state,
    zip_code,
    address,
    linkedin_url,
    portfolio_url,
    work_authorization,
    requires_sponsorship,
    willing_to_relocate,
    salary_expectation_min,
    salary_expectation_max,
    preferred_locations,
    available_start_date,
    years_of_experience,
    gender,
    ethnicity,
    veteran_status,
    disability_status,
    created_at,
    updated_at
)
SELECT 
    id,
    '(555) 123-4567',
    'United States',
    'San Francisco',
    'California',
    '94105',
    '123 Main St',
    'https://linkedin.com/in/testuser',
    'https://github.com/testuser',
    'yes',
    false,
    true,
    100000,
    150000,
    'San Francisco, New York, Remote',
    'immediately',
    5,
    'male',
    'prefer_not_to_say',
    'no',
    'no',
    NOW(),
    NOW()
FROM user_info
ON CONFLICT (user_id) DO NOTHING;