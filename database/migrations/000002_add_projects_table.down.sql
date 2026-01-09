-- Down migration: 000002_add_projects_table

DROP TRIGGER IF EXISTS update_projects_updated_at ON projects;
DROP INDEX IF EXISTS idx_projects_resume_id;
DROP TABLE IF EXISTS projects;
