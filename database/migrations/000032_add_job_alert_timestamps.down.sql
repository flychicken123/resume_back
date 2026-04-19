ALTER TABLE users
DROP COLUMN IF EXISTS last_job_alert_resume_hash,
DROP COLUMN IF EXISTS last_job_alert_sent_at;
