ALTER TABLE job_postings ADD COLUMN IF NOT EXISTS career_field TEXT;
ALTER TABLE job_postings ADD COLUMN IF NOT EXISTS extracted_skills TEXT[];

CREATE INDEX IF NOT EXISTS idx_job_postings_career_field
    ON job_postings (career_field) WHERE career_field IS NOT NULL;

ALTER TABLE job_companies ADD COLUMN IF NOT EXISTS quality_score NUMERIC(3,2) DEFAULT 1.0;
