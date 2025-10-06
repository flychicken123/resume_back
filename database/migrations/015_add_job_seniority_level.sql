ALTER TABLE job_postings
    ADD COLUMN seniority_level INTEGER NOT NULL DEFAULT 2;

CREATE INDEX IF NOT EXISTS idx_job_postings_seniority_level
    ON job_postings (seniority_level);
