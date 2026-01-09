-- Down migration: 000009_add_job_company_unique_index

DROP INDEX IF EXISTS idx_resume_job_matches_matched_at;
DROP INDEX IF EXISTS idx_resume_job_matches_user_resume;
DROP TABLE IF EXISTS resume_job_matches;
DROP INDEX IF EXISTS ux_job_companies_careers_url;
