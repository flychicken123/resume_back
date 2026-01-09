-- Down migration: 000015_resume_profile_combined

ALTER TABLE resumes DROP COLUMN IF EXISTS location;
ALTER TABLE resumes DROP COLUMN IF EXISTS job_description;
ALTER TABLE resumes DROP COLUMN IF EXISTS education;
ALTER TABLE resumes DROP COLUMN IF EXISTS experience;
ALTER TABLE resumes DROP COLUMN IF EXISTS skills_categorized;
ALTER TABLE resumes DROP COLUMN IF EXISTS resume_name;

DROP INDEX IF EXISTS idx_resumes_user_unique;

ALTER TABLE resumes ADD COLUMN IF NOT EXISTS profile_data JSONB;
