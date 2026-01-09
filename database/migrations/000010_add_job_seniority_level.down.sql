-- Down migration: 000010_add_job_seniority_level

DROP INDEX IF EXISTS idx_job_postings_seniority_level;
ALTER TABLE job_postings DROP COLUMN IF EXISTS seniority_level;
