-- Migration: 020_create_experiments_tables.sql
-- Description: Core tables for experimentation and A/B testing

CREATE TABLE IF NOT EXISTS experiments (
    id SERIAL PRIMARY KEY,
    experiment_key VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'running', 'paused', 'completed')),
    start_at TIMESTAMP WITH TIME ZONE,
    end_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS experiment_variants (
    id SERIAL PRIMARY KEY,
    experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    variant_key VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    weight INTEGER NOT NULL DEFAULT 50 CHECK (weight >= 0),
    is_control BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (experiment_id, variant_key)
);

CREATE TABLE IF NOT EXISTS experiment_assignments (
    id BIGSERIAL PRIMARY KEY,
    experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    variant_id INTEGER NOT NULL REFERENCES experiment_variants(id) ON DELETE CASCADE,
    user_identifier VARCHAR(128) NOT NULL,
    request_path TEXT,
    bucketed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (experiment_id, user_identifier)
);

CREATE INDEX IF NOT EXISTS idx_experiment_assignments_lookup
    ON experiment_assignments (experiment_id, user_identifier);

CREATE TABLE IF NOT EXISTS experiment_events (
    id BIGSERIAL PRIMARY KEY,
    experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    variant_id INTEGER NOT NULL REFERENCES experiment_variants(id) ON DELETE CASCADE,
    user_identifier VARCHAR(128) NOT NULL,
    event_name VARCHAR(120) NOT NULL,
    metadata JSONB,
    occurred_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_experiment_events_lookup
    ON experiment_events (experiment_id, variant_id, event_name);

CREATE INDEX IF NOT EXISTS idx_experiment_events_user
    ON experiment_events (experiment_id, user_identifier);

INSERT INTO migration_history (migration_name, description)
VALUES ('020_create_experiments_tables.sql', 'A/B testing experiments, variants, assignments, and event tracking tables')
ON CONFLICT (migration_name) DO NOTHING;
