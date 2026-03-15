ALTER TABLE users DROP COLUMN IF EXISTS followup_reminders_enabled;
ALTER TABLE job_applications DROP COLUMN IF EXISTS reminders_enabled;
