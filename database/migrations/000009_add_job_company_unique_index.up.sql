-- Migration: 010_job_ingestion_enhancements.sql
-- Description: Unique careers URL index, updated Greenhouse seed data, resume job match storage, and daily sync cadence
-- Date: 2025-10-04

CREATE UNIQUE INDEX IF NOT EXISTS ux_job_companies_careers_url
ON job_companies (careers_url);

INSERT INTO job_companies (name, website_url, careers_url, ats_provider, external_identifier, is_active, sync_interval_minutes)
VALUES
    ('Stripe', 'https://stripe.com', 'https://boards.greenhouse.io/stripe', 'greenhouse', 'stripe', TRUE, 1440),
    ('Coinbase', 'https://www.coinbase.com', 'https://boards.greenhouse.io/coinbase', 'greenhouse', 'coinbase', TRUE, 1440),
    ('Discord', 'https://discord.com', 'https://boards.greenhouse.io/discord', 'greenhouse', 'discord', TRUE, 1440),
    ('DoorDash', 'https://www.doordash.com', 'https://boards.greenhouse.io/doordash', 'greenhouse', 'doordash', FALSE, 1440),
    ('Duolingo', 'https://www.duolingo.com', 'https://boards.greenhouse.io/duolingo', 'greenhouse', 'duolingo', TRUE, 1440),
    ('Datadog', 'https://www.datadoghq.com', 'https://boards.greenhouse.io/datadog', 'greenhouse', 'datadog', TRUE, 1440),
    ('Notion', 'https://www.notion.so', 'https://boards.greenhouse.io/notion', 'greenhouse', 'notion', FALSE, 1440),
    ('Airtable', 'https://www.airtable.com', 'https://boards.greenhouse.io/airtable', 'greenhouse', 'airtable', TRUE, 1440),
    ('Gusto', 'https://gusto.com', 'https://boards.greenhouse.io/gusto', 'greenhouse', 'gusto', TRUE, 1440),
    ('Brex', 'https://www.brex.com', 'https://boards.greenhouse.io/brex', 'greenhouse', 'brex', TRUE, 1440),
    ('Plaid', 'https://plaid.com', 'https://boards.greenhouse.io/plaid', 'greenhouse', 'plaid', FALSE, 1440),
    ('Asana', 'https://asana.com', 'https://boards.greenhouse.io/asana', 'greenhouse', 'asana', TRUE, 1440),
    ('Figma', 'https://www.figma.com', 'https://boards.greenhouse.io/figma', 'greenhouse', 'figma', TRUE, 1440),
    ('Robinhood', 'https://www.robinhood.com', 'https://boards.greenhouse.io/robinhood', 'greenhouse', 'robinhood', TRUE, 1440),
    ('Reddit', 'https://www.redditinc.com', 'https://boards.greenhouse.io/reddit', 'greenhouse', 'reddit', TRUE, 1440),
    ('Coursera', 'https://www.coursera.org', 'https://boards.greenhouse.io/coursera', 'greenhouse', 'coursera', TRUE, 1440),
    ('Dropbox', 'https://www.dropbox.com', 'https://boards.greenhouse.io/dropbox', 'greenhouse', 'dropbox', TRUE, 1440),
    ('Intercom', 'https://www.intercom.com', 'https://boards.greenhouse.io/intercom', 'greenhouse', 'intercom', TRUE, 1440),
    ('Databricks', 'https://www.databricks.com', 'https://boards.greenhouse.io/databricks', 'greenhouse', 'databricks', TRUE, 1440),
    ('Pilot', 'https://pilot.com', 'https://boards.greenhouse.io/pilot', 'greenhouse', 'pilot', TRUE, 1440),
    ('Scale AI', 'https://scale.com', 'https://boards.greenhouse.io/scaleai', 'greenhouse', 'scaleai', TRUE, 1440),
    ('Anthropic', 'https://www.anthropic.com', 'https://boards.greenhouse.io/anthropic', 'greenhouse', 'anthropic', TRUE, 1440)
ON CONFLICT (careers_url) DO UPDATE SET
    name = EXCLUDED.name,
    website_url = EXCLUDED.website_url,
    external_identifier = EXCLUDED.external_identifier,
    ats_provider = EXCLUDED.ats_provider,
    is_active = EXCLUDED.is_active,
    sync_interval_minutes = EXCLUDED.sync_interval_minutes,
    updated_at = CURRENT_TIMESTAMP;

UPDATE job_companies
SET last_sync_status = 'disabled: greenhouse board unavailable',
    updated_at = CURRENT_TIMESTAMP
WHERE careers_url IN (
    'https://boards.greenhouse.io/doordash',
    'https://boards.greenhouse.io/notion',
    'https://boards.greenhouse.io/plaid'
);

CREATE TABLE IF NOT EXISTS resume_job_matches (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resume_hash TEXT NOT NULL,
    job_posting_id BIGINT NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    match_score NUMERIC(6, 2) NOT NULL,
    matched_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, resume_hash, job_posting_id)
);

CREATE INDEX IF NOT EXISTS idx_resume_job_matches_user_resume
    ON resume_job_matches (user_id, resume_hash, match_score DESC);

CREATE INDEX IF NOT EXISTS idx_resume_job_matches_matched_at
    ON resume_job_matches (matched_at);

UPDATE job_companies
SET sync_interval_minutes = 1440,
    updated_at = CURRENT_TIMESTAMP
WHERE sync_interval_minutes IS NULL OR sync_interval_minutes <> 1440;
