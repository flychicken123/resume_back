-- Down migration: 000012_create_chat_history

DROP INDEX IF EXISTS idx_assistant_chat_session_created_at;
DROP TABLE IF EXISTS assistant_chat_messages;
