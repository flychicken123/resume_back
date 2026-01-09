-- Migration: add marketing opt-in fields to users
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS marketing_opt_in BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS marketing_opted_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS marketing_opt_in_source TEXT,
    ADD COLUMN IF NOT EXISTS signup_plan_preference VARCHAR(50);
