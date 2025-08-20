-- SQL script to drop unused tables from existing database
-- Run this to clean up your local database

-- Drop tables if they exist
DROP TABLE IF EXISTS job_automation_logs CASCADE;
DROP TABLE IF EXISTS job_form_requirements CASCADE;
DROP TABLE IF EXISTS user_job_preferences CASCADE;

-- Drop any orphaned indexes
DROP INDEX IF EXISTS idx_job_automation_logs_application_id;
DROP INDEX IF EXISTS idx_job_form_requirements_domain;
DROP INDEX IF EXISTS idx_user_job_preferences_user_id;
DROP INDEX IF EXISTS idx_user_job_preferences_domain;

-- Add comment about changes
COMMENT ON TABLE job_applications IS 'Job application status and any error logs are now stored directly in this table instead of separate log tables';