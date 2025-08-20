-- Add location and demographic fields to user_job_profiles table
ALTER TABLE user_job_profiles 
ADD COLUMN IF NOT EXISTS country VARCHAR(100),
ADD COLUMN IF NOT EXISTS city VARCHAR(100),
ADD COLUMN IF NOT EXISTS state VARCHAR(100),
ADD COLUMN IF NOT EXISTS zip_code VARCHAR(20),
ADD COLUMN IF NOT EXISTS address VARCHAR(255),
ADD COLUMN IF NOT EXISTS gender VARCHAR(20),
ADD COLUMN IF NOT EXISTS ethnicity VARCHAR(100),
ADD COLUMN IF NOT EXISTS veteran_status VARCHAR(50),
ADD COLUMN IF NOT EXISTS disability_status VARCHAR(50);

-- Set default values for existing records
UPDATE user_job_profiles 
SET country = 'United States'
WHERE country IS NULL;