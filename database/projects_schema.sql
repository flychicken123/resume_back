-- Projects table for students and professionals to showcase their work
-- This is especially useful for students without work experience

CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
    project_name VARCHAR(255) NOT NULL,
    description TEXT,
    technologies TEXT, -- Comma-separated list or JSON array
    project_url VARCHAR(500), -- GitHub, demo link, etc.
    role VARCHAR(255), -- e.g., "Team Lead", "Solo Developer", "Frontend Developer"
    start_date DATE,
    end_date DATE,
    is_current BOOLEAN DEFAULT FALSE,
    display_order INTEGER DEFAULT 0, -- For ordering projects
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for better performance
CREATE INDEX IF NOT EXISTS idx_projects_resume_id ON projects(resume_id);
CREATE INDEX IF NOT EXISTS idx_projects_display_order ON projects(display_order);

-- Add trigger to update updated_at timestamp
CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add projects to resume_history for storing parsed projects data
ALTER TABLE resume_history ADD COLUMN IF NOT EXISTS projects_json JSONB DEFAULT '[]'::jsonb;

-- Comments for documentation
COMMENT ON TABLE projects IS 'Stores project information for resumes, especially useful for students and those without extensive work experience';
COMMENT ON COLUMN projects.technologies IS 'Technologies used in the project (e.g., "React, Node.js, PostgreSQL")';
COMMENT ON COLUMN projects.project_url IS 'Link to project demo, GitHub repository, or portfolio';
COMMENT ON COLUMN projects.role IS 'The person''s role in the project if it was a team effort';