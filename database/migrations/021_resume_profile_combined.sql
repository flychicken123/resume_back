-- Description: Consolidated resume profile changes (previously 021-024)
-- Adds profile_data (for backfill), enforces single resume per user, adds resume_name,
-- introduces structured resume fields, backfills from JSON, then drops profile_data.

-- Add profile_data column to capture all resume fields (experiences, education, etc.)
ALTER TABLE IF EXISTS resumes
ADD COLUMN IF NOT EXISTS profile_data JSONB;

-- Clean up duplicate resume rows before adding uniqueness
-- Keep the most recent record (highest id) per user
WITH ranked AS (
    SELECT id, user_id,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY id DESC) AS rn
    FROM resumes
)
DELETE FROM resumes
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

-- Ensure a single resume profile per user
CREATE UNIQUE INDEX IF NOT EXISTS idx_resumes_user_unique ON resumes(user_id);

-- Add explicit resume_name and backfill existing records
ALTER TABLE IF EXISTS resumes
ADD COLUMN IF NOT EXISTS resume_name VARCHAR(255);

UPDATE resumes
SET resume_name = COALESCE(NULLIF(resume_name, ''), NULLIF(name, ''), 'User Resume');

ALTER TABLE IF EXISTS resumes
ALTER COLUMN resume_name SET DEFAULT 'User Resume';

ALTER TABLE IF EXISTS resumes
ALTER COLUMN resume_name SET NOT NULL;

-- Add structured resume fields
ALTER TABLE IF EXISTS resumes
    ADD COLUMN IF NOT EXISTS skills_categorized TEXT,
    ADD COLUMN IF NOT EXISTS experience TEXT,
    ADD COLUMN IF NOT EXISTS education TEXT,
    ADD COLUMN IF NOT EXISTS job_description TEXT,
    ADD COLUMN IF NOT EXISTS location VARCHAR(255);

-- Backfill structured fields from profile_data JSON if present
UPDATE resumes
SET
    skills_categorized = COALESCE(skills_categorized, profile_data ->> 'skillsCategorized'),
    experience         = COALESCE(experience, profile_data ->> 'experience'),
    education          = COALESCE(education, profile_data ->> 'education'),
    job_description    = COALESCE(job_description, profile_data ->> 'jobDescription'),
    location           = COALESCE(location, profile_data ->> 'location');

-- Remove the legacy profile_data blob after backfilling
ALTER TABLE IF EXISTS resumes
DROP COLUMN IF EXISTS profile_data;
