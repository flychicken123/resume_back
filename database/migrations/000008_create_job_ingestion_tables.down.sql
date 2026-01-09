-- Down migration: 000008_create_job_ingestion_tables

DROP TRIGGER IF EXISTS trg_job_companies_updated ON job_companies;
DROP FUNCTION IF EXISTS set_job_company_updated_at();

DROP INDEX IF EXISTS idx_job_sync_runs_company_started;
DROP INDEX IF EXISTS idx_job_postings_last_seen;
DROP INDEX IF EXISTS idx_job_postings_company_active;

DROP TABLE IF EXISTS job_sync_runs;
DROP TABLE IF EXISTS job_postings;
DROP TABLE IF EXISTS job_companies;
