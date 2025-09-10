package models

import (
	"database/sql"
	"time"
)

// Project represents a project entry in a resume
type Project struct {
	ID           int        `json:"id"`
	ResumeID     int        `json:"resume_id"`
	ProjectName  string     `json:"project_name"`
	Description  string     `json:"description"`
	Technologies string     `json:"technologies"`
	ProjectURL   string     `json:"project_url"`
	Role         string     `json:"role"`
	StartDate    *time.Time `json:"start_date"`
	EndDate      *time.Time `json:"end_date"`
	IsCurrent    bool       `json:"is_current"`
	DisplayOrder int        `json:"display_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ProjectModel handles database operations for projects
type ProjectModel struct {
	db *sql.DB
}

// NewProjectModel creates a new project model
func NewProjectModel(db *sql.DB) *ProjectModel {
	return &ProjectModel{db: db}
}

// Create inserts a new project into the database
func (m *ProjectModel) Create(project *Project) error {
	query := `
		INSERT INTO projects (
			resume_id, project_name, description, technologies,
			project_url, role, start_date, end_date, is_current, display_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`
	
	err := m.db.QueryRow(
		query,
		project.ResumeID,
		project.ProjectName,
		project.Description,
		project.Technologies,
		project.ProjectURL,
		project.Role,
		project.StartDate,
		project.EndDate,
		project.IsCurrent,
		project.DisplayOrder,
	).Scan(&project.ID, &project.CreatedAt, &project.UpdatedAt)
	
	return err
}

// GetByResumeID retrieves all projects for a specific resume
func (m *ProjectModel) GetByResumeID(resumeID int) ([]*Project, error) {
	query := `
		SELECT id, resume_id, project_name, description, technologies,
			   project_url, role, start_date, end_date, is_current,
			   display_order, created_at, updated_at
		FROM projects
		WHERE resume_id = $1
		ORDER BY display_order, start_date DESC
	`
	
	rows, err := m.db.Query(query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var projects []*Project
	for rows.Next() {
		project := &Project{}
		err := rows.Scan(
			&project.ID,
			&project.ResumeID,
			&project.ProjectName,
			&project.Description,
			&project.Technologies,
			&project.ProjectURL,
			&project.Role,
			&project.StartDate,
			&project.EndDate,
			&project.IsCurrent,
			&project.DisplayOrder,
			&project.CreatedAt,
			&project.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	
	return projects, nil
}

// Update modifies an existing project
func (m *ProjectModel) Update(project *Project) error {
	query := `
		UPDATE projects
		SET project_name = $2, description = $3, technologies = $4,
			project_url = $5, role = $6, start_date = $7, end_date = $8,
			is_current = $9, display_order = $10, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING updated_at
	`
	
	err := m.db.QueryRow(
		query,
		project.ID,
		project.ProjectName,
		project.Description,
		project.Technologies,
		project.ProjectURL,
		project.Role,
		project.StartDate,
		project.EndDate,
		project.IsCurrent,
		project.DisplayOrder,
	).Scan(&project.UpdatedAt)
	
	return err
}

// Delete removes a project from the database
func (m *ProjectModel) Delete(id int) error {
	query := `DELETE FROM projects WHERE id = $1`
	_, err := m.db.Exec(query, id)
	return err
}

// DeleteByResumeID removes all projects for a specific resume
func (m *ProjectModel) DeleteByResumeID(resumeID int) error {
	query := `DELETE FROM projects WHERE resume_id = $1`
	_, err := m.db.Exec(query, resumeID)
	return err
}