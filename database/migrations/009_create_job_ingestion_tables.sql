-- Migration: 009_create_job_ingestion_tables.sql
-- Description: Schema for ATS-connected company catalog and synchronized job postings
-- Date: 2025-10-02

CREATE TABLE job_companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    website_url TEXT,
    careers_url TEXT NOT NULL,
    ats_provider TEXT NOT NULL,
    external_identifier TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sync_interval_minutes INTEGER NOT NULL DEFAULT 180,
    last_synced_at TIMESTAMP,
    last_sync_status TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE job_postings (
    id BIGSERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES job_companies(id) ON DELETE CASCADE,
    external_job_id TEXT NOT NULL,
    title TEXT NOT NULL,
    location TEXT,
    remote_type TEXT,
    department TEXT,
    employment_type TEXT,
    job_url TEXT NOT NULL,
    application_url TEXT,
    description TEXT,
    salary_min NUMERIC,
    salary_max NUMERIC,
    salary_currency TEXT,
    posted_at TIMESTAMP,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    raw_payload JSONB,
    UNIQUE(company_id, external_job_id)
);

CREATE TABLE job_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES job_companies(id) ON DELETE CASCADE,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP,
    status TEXT NOT NULL,
    message TEXT,
    jobs_found INTEGER NOT NULL DEFAULT 0,
    jobs_created INTEGER NOT NULL DEFAULT 0,
    jobs_updated INTEGER NOT NULL DEFAULT 0,
    jobs_closed INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_job_postings_company_active ON job_postings(company_id, is_active);
CREATE INDEX idx_job_postings_last_seen ON job_postings(last_seen_at DESC);
CREATE INDEX idx_job_sync_runs_company_started ON job_sync_runs(company_id, started_at DESC);

-- Trigger to keep updated_at fresh
CREATE OR REPLACE FUNCTION set_job_company_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_job_companies_updated
    BEFORE UPDATE ON job_companies
    FOR EACH ROW
    EXECUTE FUNCTION set_job_company_updated_at();
