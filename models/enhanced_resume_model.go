package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// EnhancedResume represents a complete resume with all user information
type EnhancedResume struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	
	// Resume metadata
	ResumeName string    `json:"resume_name"`
	FilePath   string    `json:"file_path,omitempty"`
	S3Key      string    `json:"s3_key,omitempty"`
	
	// Personal Information
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	FullName   string    `json:"full_name"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	
	// Additional contact info
	LinkedInURL   string `json:"linkedin_url,omitempty"`
	GitHubURL     string `json:"github_url,omitempty"`
	PortfolioURL  string `json:"portfolio_url,omitempty"`
	
	// Location information
	Address   string    `json:"address,omitempty"`
	City      string    `json:"city,omitempty"`
	State     string    `json:"state,omitempty"`
	Country   string    `json:"country,omitempty"`
	ZipCode   string    `json:"zip_code,omitempty"`
	
	// Professional information
	CurrentTitle       string `json:"current_title,omitempty"`
	YearsOfExperience  int    `json:"years_of_experience,omitempty"`
	
	// Work preferences
	WorkAuthorization    string `json:"work_authorization"`      // yes/no/needs_sponsorship
	RequiresSponsorship  bool   `json:"requires_sponsorship"`
	WillingToRelocate    bool   `json:"willing_to_relocate"`
	RemoteWorkPreference string `json:"remote_work_preference"`   // yes/no/hybrid
	
	// Demographic information (optional)
	Gender           string `json:"gender,omitempty"`
	Ethnicity        string `json:"ethnicity,omitempty"`
	VeteranStatus    string `json:"veteran_status,omitempty"`
	DisabilityStatus string `json:"disability_status,omitempty"`
	
	// Resume content
	Summary string   `json:"summary,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	
	// Related data
	Experiences []EnhancedResumeExperience `json:"experiences,omitempty"`
	Education   []EnhancedResumeEducation  `json:"education,omitempty"`
	
	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	
	// Flags
	IsActive  bool `json:"is_active"`
	IsDefault bool `json:"is_default"`
}

// EnhancedResumeExperience represents work experience for a specific resume
type EnhancedResumeExperience struct {
	ID          int       `json:"id"`
	ResumeID    int       `json:"resume_id"`
	CompanyName string    `json:"company_name"`
	JobTitle    string    `json:"job_title"`
	StartDate   time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	IsCurrent   bool      `json:"is_current"`
	Description string    `json:"description"`
	Location    string    `json:"location,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// EnhancedResumeEducation represents education for a specific resume
type EnhancedResumeEducation struct {
	ID              int       `json:"id"`
	ResumeID        int       `json:"resume_id"`
	InstitutionName string    `json:"institution_name"`
	Degree          string    `json:"degree"`
	FieldOfStudy    string    `json:"field_of_study,omitempty"`
	StartDate       *time.Time `json:"start_date,omitempty"`
	EndDate         *time.Time `json:"end_date,omitempty"`
	GPA             string    `json:"gpa,omitempty"`
	Description     string    `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// EnhancedResumeModel handles database operations for enhanced resumes
type EnhancedResumeModel struct {
	db *sql.DB
}

// NewEnhancedResumeModel creates a new EnhancedResumeModel
func NewEnhancedResumeModel(db *sql.DB) *EnhancedResumeModel {
	return &EnhancedResumeModel{db: db}
}

// CreateResume creates a new resume in the database
func (m *EnhancedResumeModel) CreateResume(resume *EnhancedResume) error {
	// Convert skills slice to JSON
	skillsJSON, err := json.Marshal(resume.Skills)
	if err != nil {
		return err
	}
	
	query := `
		INSERT INTO resumes (
			user_id, resume_name, file_path, s3_key,
			first_name, last_name, full_name, email, phone,
			linkedin_url, github_url, portfolio_url,
			address, city, state, country, zip_code,
			current_title, years_of_experience,
			work_authorization, requires_sponsorship, willing_to_relocate, remote_work_preference,
			gender, ethnicity, veteran_status, disability_status,
			summary, skills, is_active, is_default
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
			$24, $25, $26, $27, $28, $29, $30, $31
		) RETURNING id, created_at, updated_at`
	
	err = m.db.QueryRow(query,
		resume.UserID, resume.ResumeName, resume.FilePath, resume.S3Key,
		resume.FirstName, resume.LastName, resume.FullName, resume.Email, resume.Phone,
		resume.LinkedInURL, resume.GitHubURL, resume.PortfolioURL,
		resume.Address, resume.City, resume.State, resume.Country, resume.ZipCode,
		resume.CurrentTitle, resume.YearsOfExperience,
		resume.WorkAuthorization, resume.RequiresSponsorship, resume.WillingToRelocate, resume.RemoteWorkPreference,
		resume.Gender, resume.Ethnicity, resume.VeteranStatus, resume.DisabilityStatus,
		resume.Summary, string(skillsJSON), resume.IsActive, resume.IsDefault,
	).Scan(&resume.ID, &resume.CreatedAt, &resume.UpdatedAt)
	
	return err
}

// GetResumeByID retrieves a resume by its ID
func (m *EnhancedResumeModel) GetResumeByID(resumeID int) (*EnhancedResume, error) {
	resume := &EnhancedResume{}
	var skillsJSON string
	
	query := `
		SELECT 
			id, user_id, resume_name, file_path, s3_key,
			first_name, last_name, full_name, email, phone,
			linkedin_url, github_url, portfolio_url,
			address, city, state, country, zip_code,
			current_title, years_of_experience,
			work_authorization, requires_sponsorship, willing_to_relocate, remote_work_preference,
			gender, ethnicity, veteran_status, disability_status,
			summary, skills, is_active, is_default,
			created_at, updated_at, last_used_at
		FROM resumes
		WHERE id = $1`
	
	err := m.db.QueryRow(query, resumeID).Scan(
		&resume.ID, &resume.UserID, &resume.ResumeName, &resume.FilePath, &resume.S3Key,
		&resume.FirstName, &resume.LastName, &resume.FullName, &resume.Email, &resume.Phone,
		&resume.LinkedInURL, &resume.GitHubURL, &resume.PortfolioURL,
		&resume.Address, &resume.City, &resume.State, &resume.Country, &resume.ZipCode,
		&resume.CurrentTitle, &resume.YearsOfExperience,
		&resume.WorkAuthorization, &resume.RequiresSponsorship, &resume.WillingToRelocate, &resume.RemoteWorkPreference,
		&resume.Gender, &resume.Ethnicity, &resume.VeteranStatus, &resume.DisabilityStatus,
		&resume.Summary, &skillsJSON, &resume.IsActive, &resume.IsDefault,
		&resume.CreatedAt, &resume.UpdatedAt, &resume.LastUsedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	// Parse skills JSON
	if skillsJSON != "" {
		json.Unmarshal([]byte(skillsJSON), &resume.Skills)
	}
	
	// Load experiences and education
	resume.Experiences, _ = m.GetResumeExperiences(resumeID)
	resume.Education, _ = m.GetResumeEducation(resumeID)
	
	return resume, nil
}

// GetUserDefaultResume gets the user's default resume
func (m *EnhancedResumeModel) GetUserDefaultResume(userID int) (*EnhancedResume, error) {
	var resumeID int
	
	query := `SELECT id FROM resumes WHERE user_id = $1 AND is_default = true AND is_active = true LIMIT 1`
	err := m.db.QueryRow(query, userID).Scan(&resumeID)
	if err != nil {
		// If no default, get the most recently used active resume
		query = `SELECT id FROM resumes WHERE user_id = $1 AND is_active = true ORDER BY last_used_at DESC NULLS LAST, created_at DESC LIMIT 1`
		err = m.db.QueryRow(query, userID).Scan(&resumeID)
		if err != nil {
			return nil, err
		}
	}
	
	return m.GetResumeByID(resumeID)
}

// UpdateResumeLastUsed updates the last_used_at timestamp
func (m *EnhancedResumeModel) UpdateResumeLastUsed(resumeID int) error {
	query := `UPDATE resumes SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`
	_, err := m.db.Exec(query, resumeID)
	return err
}

// GetResumeExperiences gets all experiences for a resume
func (m *EnhancedResumeModel) GetResumeExperiences(resumeID int) ([]EnhancedResumeExperience, error) {
	query := `
		SELECT id, resume_id, company_name, job_title, start_date, end_date, 
		       is_current, description, location, created_at
		FROM resume_experiences
		WHERE resume_id = $1
		ORDER BY is_current DESC, start_date DESC`
	
	rows, err := m.db.Query(query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var experiences []EnhancedResumeExperience
	for rows.Next() {
		var exp EnhancedResumeExperience
		err := rows.Scan(
			&exp.ID, &exp.ResumeID, &exp.CompanyName, &exp.JobTitle,
			&exp.StartDate, &exp.EndDate, &exp.IsCurrent,
			&exp.Description, &exp.Location, &exp.CreatedAt,
		)
		if err != nil {
			continue
		}
		experiences = append(experiences, exp)
	}
	
	return experiences, nil
}

// GetResumeEducation gets all education entries for a resume
func (m *EnhancedResumeModel) GetResumeEducation(resumeID int) ([]EnhancedResumeEducation, error) {
	query := `
		SELECT id, resume_id, institution_name, degree, field_of_study,
		       start_date, end_date, gpa, description, created_at
		FROM resume_education
		WHERE resume_id = $1
		ORDER BY end_date DESC NULLS FIRST`
	
	rows, err := m.db.Query(query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var education []EnhancedResumeEducation
	for rows.Next() {
		var edu EnhancedResumeEducation
		err := rows.Scan(
			&edu.ID, &edu.ResumeID, &edu.InstitutionName, &edu.Degree,
			&edu.FieldOfStudy, &edu.StartDate, &edu.EndDate,
			&edu.GPA, &edu.Description, &edu.CreatedAt,
		)
		if err != nil {
			continue
		}
		education = append(education, edu)
	}
	
	return education, nil
}

// SetDefaultResume sets a resume as the default for a user
func (m *EnhancedResumeModel) SetDefaultResume(userID, resumeID int) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Remove default from other resumes
	_, err = tx.Exec(`UPDATE resumes SET is_default = false WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	
	// Set new default
	_, err = tx.Exec(`UPDATE resumes SET is_default = true WHERE id = $1 AND user_id = $2`, resumeID, userID)
	if err != nil {
		return err
	}
	
	return tx.Commit()
}