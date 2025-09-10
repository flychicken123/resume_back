-- Migration Tracking Table
-- This table keeps track of which migrations have been applied to the database

CREATE TABLE IF NOT EXISTS migration_history (
    id SERIAL PRIMARY KEY,
    migration_name VARCHAR(255) UNIQUE NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    checksum VARCHAR(64),
    description TEXT
);

-- Insert initial migrations (if tables already exist)
-- These can be run safely as they check for existence first
DO $$
BEGIN
    -- Check if users table exists (indicating 001 was applied)
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
        INSERT INTO migration_history (migration_name, description) 
        VALUES ('001_initial_schema.sql', 'Initial database schema for AI Resume Builder')
        ON CONFLICT (migration_name) DO NOTHING;
    END IF;
    
    -- Check if projects table exists (indicating 002 was applied)
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'projects') THEN
        INSERT INTO migration_history (migration_name, description)
        VALUES ('002_add_projects_table.sql', 'Add projects table for students and professionals without work experience')
        ON CONFLICT (migration_name) DO NOTHING;
    END IF;
END $$;