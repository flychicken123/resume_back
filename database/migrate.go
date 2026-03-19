package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations executes all pending database migrations.
// It uses golang-migrate to apply migrations from the migrations directory.
// If the database is in a dirty state (from a failed migration), it will
// attempt to detect if the schema is already applied and fix the state.
func RunMigrations(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Check current state
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	// Handle dirty state - this happens when a previous migration failed mid-way
	if dirty {
		log.Printf("Database is dirty at version %d, attempting to fix...", version)

		// Check if the schema for this version is actually complete
		// by verifying key tables exist
		if schemaAppearsComplete(db, version) {
			log.Printf("Schema appears complete for version %d, forcing clean state", version)
			if err := m.Force(int(version)); err != nil {
				return fmt.Errorf("failed to force version %d: %w", version, err)
			}
		} else {
			// Schema incomplete - try to force to previous version and re-run
			prevVersion := int(version) - 1
			if prevVersion < 0 {
				prevVersion = -1 // No migrations applied
			}
			log.Printf("Schema incomplete, forcing to version %d and retrying", prevVersion)
			if err := m.Force(prevVersion); err != nil {
				return fmt.Errorf("failed to force version %d: %w", prevVersion, err)
			}
		}
	}

	// Run all pending migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Get final version
	version, dirty, err = m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		log.Printf("Warning: Database migration state is still dirty at version %d", version)
	} else if err == migrate.ErrNilVersion {
		log.Println("Database migrations: no migrations applied yet")
	} else {
		log.Printf("Database migrations: at version %d", version)
	}

	return nil
}

// schemaAppearsComplete checks if key tables exist for a given migration version.
// This is used to determine if a "dirty" migration actually completed successfully.
func schemaAppearsComplete(db *sql.DB, version uint) bool {
	// Define key tables that should exist at each version
	tableChecks := map[uint][]string{
		1:  {"users", "resumes", "experiences", "education", "resume_history"},
		2:  {"projects"},
		3:  {"subscription_plans", "user_subscriptions", "resume_usage", "payment_history"},
		4:  {"exit_events"},
		5:  {"contact_requests"},
		6:  {"exit_events"}, // Just needs to exist, columns added
		7:  {"feedback", "feedback_followups"},
		8:  {"job_companies", "job_postings", "job_sync_runs"},
		9:  {"resume_job_matches"},
		10: {"job_postings"}, // seniority_level column
		11: {"job_companies"}, // failure tracking columns
		12: {"assistant_chat_messages"},
		13: {"users"}, // marketing columns
		14: {"experiments", "experiment_variants", "experiment_assignments", "experiment_events"},
		15: {"resumes"}, // profile columns
	}

	// Version 20 adds the embedding column to job_postings — check for it explicitly.
	if version == 20 {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'job_postings' AND column_name = 'embedding'
			)
		`).Scan(&exists)
		return err == nil && exists
	}

	tables, ok := tableChecks[version]
	if !ok {
		// Unknown version, assume complete
		return true
	}

	for _, table := range tables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, table).Scan(&exists)
		if err != nil || !exists {
			return false
		}
	}

	return true
}

// MigrateDown rolls back the last migration.
func MigrateDown(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

// MigrateToVersion migrates to a specific version.
func MigrateToVersion(db *sql.DB, migrationsPath string, version uint) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration to version %d failed: %w", version, err)
	}

	return nil
}

// ForceVersion forces the migration version without running migrations.
// Use this to fix a dirty database state after manually verifying the schema.
func ForceVersion(db *sql.DB, migrationsPath string, version int) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Force(version); err != nil {
		return fmt.Errorf("force version %d failed: %w", version, err)
	}

	log.Printf("Forced migration version to %d", version)
	return nil
}
