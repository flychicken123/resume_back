-- Down migration: 000011_add_job_company_failure_tracking

ALTER TABLE job_companies DROP COLUMN IF EXISTS sync_failure_count;
ALTER TABLE job_companies DROP COLUMN IF EXISTS last_sync_error;
