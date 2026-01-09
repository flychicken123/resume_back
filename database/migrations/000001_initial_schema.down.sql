-- Down migration: 000001_initial_schema

DROP TRIGGER IF EXISTS update_education_updated_at ON education;
DROP TRIGGER IF EXISTS update_experiences_updated_at ON experiences;
DROP TRIGGER IF EXISTS update_resumes_updated_at ON resumes;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

DROP FUNCTION IF EXISTS update_updated_at_column();

DROP INDEX IF EXISTS idx_resume_history_generated_at;
DROP INDEX IF EXISTS idx_resume_history_user_id;
DROP INDEX IF EXISTS idx_education_resume_id;
DROP INDEX IF EXISTS idx_experiences_resume_id;
DROP INDEX IF EXISTS idx_resumes_user_id;
DROP INDEX IF EXISTS idx_users_auth_provider;
DROP INDEX IF EXISTS idx_users_google_id;
DROP INDEX IF EXISTS idx_users_email;

DROP TABLE IF EXISTS resume_history;
DROP TABLE IF EXISTS education;
DROP TABLE IF EXISTS experiences;
DROP TABLE IF EXISTS resumes;
DROP TABLE IF EXISTS users;
