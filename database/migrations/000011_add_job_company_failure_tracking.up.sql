-- Migration: 016_add_job_company_failure_tracking.sql
-- Description: Add failure tracking columns to job_companies
-- Date: 2025-10-06

ALTER TABLE job_companies
    ADD COLUMN IF NOT EXISTS last_sync_error TEXT,
    ADD COLUMN IF NOT EXISTS sync_failure_count INTEGER NOT NULL DEFAULT 0;
