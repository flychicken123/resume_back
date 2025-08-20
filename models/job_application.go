package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

type JobApplication struct {
	ID                      int       `json:"id"`
	ApplicationCode         string    `json:"application_code"`  // 8-character unique code
	UserID                  int       `json:"user_id"`
	ResumeID                int       `json:"resume_id"`
	JobURL                  string    `json:"job_url"`
	CompanyName             string    `json:"company_name"`
	PositionTitle           string    `json:"position_title"`
	ApplicationStatus       string    `json:"application_status"`
	AppliedAt               time.Time `json:"applied_at"`
	Notes                   string    `json:"notes"`
	JobPageScreenshotURL    string `json:"job_page_screenshot_url,omitempty"`
	ApplicationScreenshotURL string `json:"application_screenshot_url,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type JobApplicationModel struct {
	DB *sql.DB
}

func NewJobApplicationModel(db *sql.DB) *JobApplicationModel {
	return &JobApplicationModel{DB: db}
}

// generateApplicationCode generates a unique 8-character alphanumeric code
func generateApplicationCode() string {
	bytes := make([]byte, 4) // 4 bytes = 8 hex characters
	rand.Read(bytes)
	return strings.ToUpper(hex.EncodeToString(bytes))
}

func (m *JobApplicationModel) Create(userID, resumeID int, jobURL, companyName, positionTitle, notes string) (*JobApplication, error) {
	application := &JobApplication{}
	
	// Generate a unique application code
	applicationCode := generateApplicationCode()
	
	// Check if code already exists and regenerate if needed
	for {
		var exists bool
		err := m.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM job_applications WHERE application_code = $1)", applicationCode).Scan(&exists)
		if err != nil || !exists {
			break
		}
		applicationCode = generateApplicationCode()
	}
	
	query := `
		INSERT INTO job_applications (application_code, user_id, resume_id, job_url, company_name, position_title, notes, applied_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $8)
		RETURNING id, application_code, user_id, resume_id, job_url, company_name, position_title, application_status, applied_at, notes, job_page_screenshot_url, application_screenshot_url, created_at, updated_at
	`
	var jobPageURL, applicationURL sql.NullString
	err := m.DB.QueryRow(query, applicationCode, userID, resumeID, jobURL, companyName, positionTitle, notes, time.Now()).Scan(
		&application.ID, &application.ApplicationCode, &application.UserID, &application.ResumeID, &application.JobURL, 
		&application.CompanyName, &application.PositionTitle, &application.ApplicationStatus,
		&application.AppliedAt, &application.Notes, &jobPageURL, &applicationURL, &application.CreatedAt, &application.UpdatedAt,
	)
	
	// Convert NullString to string
	if jobPageURL.Valid {
		application.JobPageScreenshotURL = jobPageURL.String
	}
	if applicationURL.Valid {
		application.ApplicationScreenshotURL = applicationURL.String
	}
	if err != nil {
		return nil, err
	}
	return application, nil
}

func (m *JobApplicationModel) GetByUserID(userID int, limit, offset int) ([]JobApplication, error) {
	applications := []JobApplication{}
	query := `
		SELECT id, application_code, user_id, resume_id, job_url, company_name, position_title, 
		       application_status, applied_at, notes, job_page_screenshot_url, application_screenshot_url, created_at, updated_at
		FROM job_applications 
		WHERE user_id = $1 
		ORDER BY applied_at DESC 
		LIMIT $2 OFFSET $3
	`
	rows, err := m.DB.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var app JobApplication
		var jobPageURL, applicationURL sql.NullString
		err := rows.Scan(
			&app.ID, &app.ApplicationCode, &app.UserID, &app.ResumeID, &app.JobURL,
			&app.CompanyName, &app.PositionTitle, &app.ApplicationStatus,
			&app.AppliedAt, &app.Notes, &jobPageURL, &applicationURL, &app.CreatedAt, &app.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		// Convert NullString to string
		if jobPageURL.Valid {
			app.JobPageScreenshotURL = jobPageURL.String
		}
		if applicationURL.Valid {
			app.ApplicationScreenshotURL = applicationURL.String
		}
		
		applications = append(applications, app)
	}
	return applications, nil
}

func (m *JobApplicationModel) GetByID(id int) (*JobApplication, error) {
	application := &JobApplication{}
	query := `
		SELECT id, application_code, user_id, resume_id, job_url, company_name, position_title, 
		       application_status, applied_at, notes, job_page_screenshot_url, application_screenshot_url, created_at, updated_at
		FROM job_applications WHERE id = $1
	`
	var jobPageURL, applicationURL sql.NullString
	err := m.DB.QueryRow(query, id).Scan(
		&application.ID, &application.ApplicationCode, &application.UserID, &application.ResumeID, &application.JobURL,
		&application.CompanyName, &application.PositionTitle, &application.ApplicationStatus,
		&application.AppliedAt, &application.Notes, &jobPageURL, &applicationURL, &application.CreatedAt, &application.UpdatedAt,
	)
	
	// Convert NullString to string
	if jobPageURL.Valid {
		application.JobPageScreenshotURL = jobPageURL.String
	}
	if applicationURL.Valid {
		application.ApplicationScreenshotURL = applicationURL.String
	}
	
	if err != nil {
		return nil, err
	}
	return application, nil
}

// GetByApplicationCode retrieves a job application by its unique code
func (m *JobApplicationModel) GetByApplicationCode(applicationCode string) (*JobApplication, error) {
	application := &JobApplication{}
	query := `
		SELECT id, application_code, user_id, resume_id, job_url, company_name, position_title, 
		       application_status, applied_at, notes, job_page_screenshot_url, application_screenshot_url, created_at, updated_at
		FROM job_applications WHERE application_code = $1
	`
	var jobPageURL, applicationURL sql.NullString
	err := m.DB.QueryRow(query, applicationCode).Scan(
		&application.ID, &application.ApplicationCode, &application.UserID, &application.ResumeID, &application.JobURL,
		&application.CompanyName, &application.PositionTitle, &application.ApplicationStatus,
		&application.AppliedAt, &application.Notes, &jobPageURL, &applicationURL, &application.CreatedAt, &application.UpdatedAt,
	)
	
	// Convert NullString to string
	if jobPageURL.Valid {
		application.JobPageScreenshotURL = jobPageURL.String
	}
	if applicationURL.Valid {
		application.ApplicationScreenshotURL = applicationURL.String
	}
	
	if err != nil {
		return nil, err
	}
	return application, nil
}

func (m *JobApplicationModel) UpdateStatus(id int, status string) error {
	query := `UPDATE job_applications SET application_status = $1, updated_at = $2 WHERE id = $3`
	_, err := m.DB.Exec(query, status, time.Now(), id)
	return err
}

func (m *JobApplicationModel) UpdateScreenshots(id int, jobPageURL, applicationURL string) error {
	query := `UPDATE job_applications SET job_page_screenshot_url = $1, application_screenshot_url = $2, updated_at = $3 WHERE id = $4`
	_, err := m.DB.Exec(query, jobPageURL, applicationURL, time.Now(), id)
	return err
}

func (m *JobApplicationModel) Delete(id int) error {
	query := `DELETE FROM job_applications WHERE id = $1`
	_, err := m.DB.Exec(query, id)
	return err
}