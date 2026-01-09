-- Migration: create feedback and follow-up tables
CREATE TABLE IF NOT EXISTS feedback (
    id SERIAL PRIMARY KEY,
    user_email VARCHAR(255),
    scenario VARCHAR(100) NOT NULL,
    rating SMALLINT,
    comment TEXT,
    page_path TEXT,
    page_title TEXT,
    step TEXT,
    reason TEXT,
    previous_page_path TEXT,
    session_duration_ms BIGINT,
    page_duration_ms BIGINT,
    last_step_delta_ms BIGINT,
    referrer TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS feedback_followups (
    id SERIAL PRIMARY KEY,
    user_email VARCHAR(255) NOT NULL,
    trigger_key VARCHAR(100) NOT NULL,
    scheduled_at TIMESTAMP NOT NULL,
    sent_at TIMESTAMP,
    status VARCHAR(50) DEFAULT 'pending',
    metadata JSONB,
    last_error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_feedback_created_at ON feedback(created_at);
CREATE INDEX IF NOT EXISTS idx_feedback_followups_status ON feedback_followups(status, scheduled_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_feedback_followups_unique ON feedback_followups(user_email, trigger_key, scheduled_at);

ALTER TABLE feedback
    ADD COLUMN IF NOT EXISTS page_title TEXT,
    ADD COLUMN IF NOT EXISTS previous_page_path TEXT;

