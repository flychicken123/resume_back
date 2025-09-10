# Database Migrations

This directory contains numbered SQL migration files for the AI Resume Builder database.

## Migration Files

- **001_initial_schema.sql** - Initial database schema with users, resumes, experiences, education, and resume_history tables
- **002_add_projects_table.sql** - Adds projects table for students without work experience
- **migration_tracker.sql** - Creates migration_history table to track applied migrations

## How to Apply Migrations

1. First, apply the migration tracker to create the history table:
```bash
psql -U your_username -d resumeai -f migration_tracker.sql
```

2. Check which migrations have been applied:
```sql
SELECT * FROM migration_history ORDER BY id;
```

3. Apply any new migrations in order:
```bash
psql -U your_username -d resumeai -f 001_initial_schema.sql
psql -U your_username -d resumeai -f 002_add_projects_table.sql
```

## Adding New Migrations

When adding new migrations:
1. Create a new file with the next number (e.g., 003_your_change.sql)
2. Include a header comment with migration number, description, and date
3. After applying, the migration_tracker will automatically record it

## Current Schema Status

As of migration 002:
- Users with OAuth support
- Resumes with all personal details
- Work experiences (optional)
- Projects (for students without work experience)
- Education entries
- Resume history for tracking generated PDFs
- All tables have proper indexes and updated_at triggers