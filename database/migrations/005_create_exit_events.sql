-- Migration: Create exit events table
CREATE TABLE IF NOT EXISTS exit_events (
    id SERIAL PRIMARY KEY,
    user_email VARCHAR(255),
    page_path TEXT NOT NULL,
    page_title TEXT,
    step TEXT,
    reason TEXT,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
