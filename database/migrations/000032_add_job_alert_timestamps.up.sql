ALTER TABLE users
ADD COLUMN IF NOT EXISTS last_job_alert_sent_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS last_job_alert_resume_hash TEXT;
