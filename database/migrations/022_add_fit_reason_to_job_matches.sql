-- Migration: Add fit_reason column to resume_job_matches
-- Description: Stores AI-generated explanation of why a job is a good fit for the candidate

ALTER TABLE resume_job_matches
ADD COLUMN IF NOT EXISTS fit_reason TEXT DEFAULT NULL;

-- Add comment for documentation
COMMENT ON COLUMN resume_job_matches.fit_reason IS 'AI-generated explanation of why this job matches the resume (pre-computed for top 5 matches)';
