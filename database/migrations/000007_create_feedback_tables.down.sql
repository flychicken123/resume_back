-- Down migration: 000007_create_feedback_tables

DROP INDEX IF EXISTS idx_feedback_followups_unique;
DROP INDEX IF EXISTS idx_feedback_followups_status;
DROP INDEX IF EXISTS idx_feedback_created_at;

DROP TABLE IF EXISTS feedback_followups;
DROP TABLE IF EXISTS feedback;
