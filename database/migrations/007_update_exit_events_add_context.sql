-- Migration: Add additional context fields to exit events
ALTER TABLE exit_events
    ADD COLUMN IF NOT EXISTS previous_page_path TEXT,
    ADD COLUMN IF NOT EXISTS session_duration_ms BIGINT,
    ADD COLUMN IF NOT EXISTS page_duration_ms BIGINT,
    ADD COLUMN IF NOT EXISTS last_step_delta_ms BIGINT,
    ADD COLUMN IF NOT EXISTS referrer TEXT;
