-- Down migration: 000014_create_experiments_tables

DROP INDEX IF EXISTS idx_experiment_events_user;
DROP INDEX IF EXISTS idx_experiment_events_lookup;
DROP INDEX IF EXISTS idx_experiment_assignments_lookup;

DROP TABLE IF EXISTS experiment_events;
DROP TABLE IF EXISTS experiment_assignments;
DROP TABLE IF EXISTS experiment_variants;
DROP TABLE IF EXISTS experiments;
