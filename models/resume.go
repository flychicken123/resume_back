package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Resume struct {
	ID             int             `json:"id"`
	UserID         int             `json:"user_id"`
	ResumeName     string          `json:"resume_name"`
	Name           string          `json:"name"`
	Email          string          `json:"email"`
	Phone          string          `json:"phone"`
	Summary        json.RawMessage `json:"summary"`
	Skills         json.RawMessage `json:"skills"`
	Experiences    json.RawMessage `json:"experiences"`
	Educations     json.RawMessage `json:"educations"`
	SelectedFormat string          `json:"selected_format"`
	IsActive       bool            `json:"is_active"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ResumeModel struct {
	DB *sql.DB
}

func NewResumeModel(db *sql.DB) *ResumeModel {
	return &ResumeModel{DB: db}
}

// GetLatestByUserID gets the most recent resume for a user
func (m *ResumeModel) GetLatestByUserID(userID int) (*Resume, error) {
	resume := &Resume{}
	var summaryStr, skillsStr sql.NullString
	
	query := `
		SELECT id, user_id, COALESCE(resume_name, '') as resume_name, name, COALESCE(email, '') as email, COALESCE(phone, '') as phone, 
		       summary::text, skills::text, 
		       COALESCE(experiences, '[]'::jsonb) as experiences, COALESCE(educations, '[]'::jsonb) as educations,
		       COALESCE(selected_format, 'temp1') as selected_format, COALESCE(is_active, true) as is_active,
		       created_at, updated_at
		FROM resumes WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	err := m.DB.QueryRow(query, userID).Scan(
		&resume.ID, &resume.UserID, &resume.ResumeName, &resume.Name, &resume.Email, &resume.Phone,
		&summaryStr, &skillsStr, &resume.Experiences, &resume.Educations, 
		&resume.SelectedFormat, &resume.IsActive, &resume.CreatedAt, &resume.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	// Convert NULL strings to empty JSON
	if summaryStr.Valid && summaryStr.String != "" {
		resume.Summary = json.RawMessage(summaryStr.String)
	} else {
		resume.Summary = json.RawMessage("{}")
	}
	
	if skillsStr.Valid && skillsStr.String != "" {
		resume.Skills = json.RawMessage(skillsStr.String)
	} else {
		resume.Skills = json.RawMessage("{}")
	}
	
	return resume, nil
}

func (m *ResumeModel) Save(userID int, name string, summary, skills json.RawMessage) error {
	// Check if resume exists for this user
	var existingResumeID int
	err := m.DB.QueryRow("SELECT id FROM resumes WHERE user_id = $1", userID).Scan(&existingResumeID)

	if err == sql.ErrNoRows {
		// Create new resume
		_, err = m.DB.Exec("INSERT INTO resumes (user_id, name, summary, skills, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW())",
			userID, name, summary, skills)
	} else if err == nil {
		// Update existing resume
		_, err = m.DB.Exec("UPDATE resumes SET name = , summary = , skills = , updated_at = NOW() WHERE user_id = ",
			name, summary, skills, userID)
	}

	return err
}

func (m *ResumeModel) GetByID(id int) (*Resume, error) {
	resume := &Resume{}
	var summaryStr, skillsStr sql.NullString
	
	query := `
		SELECT id, user_id, COALESCE(resume_name, '') as resume_name, name, COALESCE(email, '') as email, COALESCE(phone, '') as phone,
		       summary::text, skills::text, 
		       COALESCE(experiences, '[]'::jsonb) as experiences, COALESCE(educations, '[]'::jsonb) as educations,
		       COALESCE(selected_format, 'temp1') as selected_format, COALESCE(is_active, true) as is_active,
		       created_at, updated_at
		FROM resumes WHERE id = $1
	`
	err := m.DB.QueryRow(query, id).Scan(
		&resume.ID, &resume.UserID, &resume.ResumeName, &resume.Name, &resume.Email, &resume.Phone,
		&summaryStr, &skillsStr, &resume.Experiences, &resume.Educations,
		&resume.SelectedFormat, &resume.IsActive, &resume.CreatedAt, &resume.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	// Convert NULL strings to empty JSON
	if summaryStr.Valid && summaryStr.String != "" {
		resume.Summary = json.RawMessage(summaryStr.String)
	} else {
		resume.Summary = json.RawMessage("{}")
	}
	
	if skillsStr.Valid && skillsStr.String != "" {
		resume.Skills = json.RawMessage(skillsStr.String)
	} else {
		resume.Skills = json.RawMessage("{}")
	}
	
	return resume, nil
}

func (m *ResumeModel) GetSummary(userID int) (json.RawMessage, error) {
	var summary json.RawMessage
	query := `SELECT summary FROM resumes WHERE user_id = $1`
	err := m.DB.QueryRow(query, userID).Scan(&summary)
	return summary, err
}

// CreateWithContactInfo creates a new resume with contact info
func (m *ResumeModel) CreateWithContactInfo(userID int, resumeName, name, email, phone string) (*Resume, error) {
	if resumeName == "" {
		resumeName = "Resume " + time.Now().Format("2006-01-02 15:04")
	}

	query := `
		INSERT INTO resumes (user_id, resume_name, name, email, phone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, user_id, resume_name, name, email, phone, created_at, updated_at
	`
	
	resume := &Resume{}
	err := m.DB.QueryRow(query, userID, resumeName, name, email, phone).Scan(
		&resume.ID, &resume.UserID, &resume.ResumeName, &resume.Name, 
		&resume.Email, &resume.Phone, &resume.CreatedAt, &resume.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	// Set defaults for empty fields
	resume.Summary = json.RawMessage("{}")
	resume.Skills = json.RawMessage("{}")
	resume.SelectedFormat = "temp1"
	resume.IsActive = true
	
	return resume, nil
}

// GetAllByUserID gets all resumes for a user
func (m *ResumeModel) GetAllByUserID(userID int, limit int) ([]Resume, error) {
	query := `
		SELECT id, user_id, COALESCE(resume_name, '') as resume_name, name, 
		       COALESCE(email, '') as email, COALESCE(phone, '') as phone,
		       COALESCE(selected_format, 'temp1') as selected_format, COALESCE(is_active, true) as is_active,
		       created_at, updated_at
		FROM resumes 
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	
	rows, err := m.DB.Query(query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var resumes []Resume
	for rows.Next() {
		var resume Resume
		err := rows.Scan(
			&resume.ID, &resume.UserID, &resume.ResumeName, &resume.Name,
			&resume.Email, &resume.Phone, &resume.SelectedFormat, &resume.IsActive,
			&resume.CreatedAt, &resume.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		// Set empty JSON for fields not selected
		resume.Summary = json.RawMessage("{}")
		resume.Skills = json.RawMessage("{}")
		resumes = append(resumes, resume)
	}
	
	return resumes, nil
}