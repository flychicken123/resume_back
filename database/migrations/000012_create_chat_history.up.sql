-- Migration: Create assistant chat history table
CREATE TABLE IF NOT EXISTS assistant_chat_messages (
    id SERIAL PRIMARY KEY,
    session_id TEXT NOT NULL,
    role VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    user_email VARCHAR(255),
    page_path TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_assistant_chat_session_created_at
    ON assistant_chat_messages (session_id, created_at);
