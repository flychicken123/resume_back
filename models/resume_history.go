package models

import (
	"database/sql"
	"time"
)

type ResumeHistory struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	ResumeID     *int      `json:"resume_id"`      // Reference to resumes table
	ResumeName   string    `json:"resume_name"`
	S3Path       string    `json:"s3_path"`
	ContactName  string    `json:"contact_name"`   // Cached from resume
	ContactEmail string    `json:"contact_email"`  // Cached from resume
	ContactPhone string    `json:"contact_phone"`  // Cached from resume
	GeneratedAt  time.Time `json:"generated_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type ResumeHistoryModel struct {
	DB *sql.DB
}

func NewResumeHistoryModel(db *sql.DB) *ResumeHistoryModel {
	return &ResumeHistoryModel{DB: db}
}

func (m *ResumeHistoryModel) Create(userID int, resumeName, s3Path string) (*ResumeHistory, error) {
	// Get the most recent resume ID for this user to link
	var resumeID sql.NullInt64
	var contactName, contactEmail, contactPhone sql.NullString
	
	resumeQuery := `
		SELECT id, COALESCE(name, ''), COALESCE(email, ''), COALESCE(phone, '')
		FROM resumes 
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	err := m.DB.QueryRow(resumeQuery, userID).Scan(&resumeID, &contactName, &contactEmail, &contactPhone)
	if err != nil && err != sql.ErrNoRows {
		// Log but don't fail - we can still create history without resume link
		resumeID.Valid = false
	}
	
	history := &ResumeHistory{}
	query := `
		INSERT INTO resume_history (user_id, resume_id, resume_name, s3_path, contact_name, contact_email, contact_phone, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, resume_id, resume_name, s3_path, contact_name, contact_email, contact_phone, generated_at, created_at
	`
	
	var nullResumeID sql.NullInt64
	if resumeID.Valid {
		nullResumeID = resumeID
	}
	
	err = m.DB.QueryRow(query, userID, nullResumeID, resumeName, s3Path, 
		contactName.String, contactEmail.String, contactPhone.String, time.Now()).Scan(
		&history.ID, &history.UserID, &nullResumeID, &history.ResumeName, &history.S3Path, 
		&history.ContactName, &history.ContactEmail, &history.ContactPhone,
		&history.GeneratedAt, &history.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	if nullResumeID.Valid {
		resumeIDInt := int(nullResumeID.Int64)
		history.ResumeID = &resumeIDInt
	}
	
	return history, nil
}

// CreateWithResumeID creates a resume history entry linked to a specific resume
func (m *ResumeHistoryModel) CreateWithResumeID(userID int, resumeID int, resumeName, s3Path string) (*ResumeHistory, error) {
	// Get contact info from the specific resume
	var contactName, contactEmail, contactPhone sql.NullString
	
	resumeQuery := `
		SELECT COALESCE(name, ''), COALESCE(email, ''), COALESCE(phone, '')
		FROM resumes 
		WHERE id = $1 AND user_id = $2
	`
	err := m.DB.QueryRow(resumeQuery, resumeID, userID).Scan(&contactName, &contactEmail, &contactPhone)
	if err != nil && err != sql.ErrNoRows {
		// Log but don't fail - we can still create history without contact info
		contactName.Valid = false
		contactEmail.Valid = false
		contactPhone.Valid = false
	}
	
	history := &ResumeHistory{}
	query := `
		INSERT INTO resume_history (user_id, resume_id, resume_name, s3_path, contact_name, contact_email, contact_phone, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, resume_id, resume_name, s3_path, contact_name, contact_email, contact_phone, generated_at, created_at
	`
	
	err = m.DB.QueryRow(query, userID, resumeID, resumeName, s3Path, 
		contactName.String, contactEmail.String, contactPhone.String, time.Now()).Scan(
		&history.ID, &history.UserID, &resumeID, &history.ResumeName, &history.S3Path, 
		&history.ContactName, &history.ContactEmail, &history.ContactPhone,
		&history.GeneratedAt, &history.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	history.ResumeID = &resumeID
	
	return history, nil
}

func (m *ResumeHistoryModel) GetByUserID(userID int) ([]*ResumeHistory, error) {
	query := `
		SELECT id, user_id, resume_id, resume_name, s3_path, 
		       COALESCE(contact_name, ''), COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
		       generated_at, created_at
		FROM resume_history
		WHERE user_id = $1
		ORDER BY generated_at DESC
	`
	rows, err := m.DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var histories []*ResumeHistory
	for rows.Next() {
		history := &ResumeHistory{}
		var nullResumeID sql.NullInt64
		err := rows.Scan(
			&history.ID, &history.UserID, &nullResumeID, &history.ResumeName, &history.S3Path,
			&history.ContactName, &history.ContactEmail, &history.ContactPhone,
			&history.GeneratedAt, &history.CreatedAt,
		)
		if nullResumeID.Valid {
			resumeIDInt := int(nullResumeID.Int64)
			history.ResumeID = &resumeIDInt
		}
		if err != nil {
			return nil, err
		}
		histories = append(histories, history)
	}
	return histories, nil
}

func (m *ResumeHistoryModel) GetByID(id int) (*ResumeHistory, error) {
	history := &ResumeHistory{}
	query := `
		SELECT id, user_id, resume_id, resume_name, s3_path, 
		       COALESCE(contact_name, ''), COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
		       generated_at, created_at
		FROM resume_history
		WHERE id = $1
	`
	var nullResumeID sql.NullInt64
	err := m.DB.QueryRow(query, id).Scan(
		&history.ID, &history.UserID, &nullResumeID, &history.ResumeName, &history.S3Path,
		&history.ContactName, &history.ContactEmail, &history.ContactPhone,
		&history.GeneratedAt, &history.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	if nullResumeID.Valid {
		resumeIDInt := int(nullResumeID.Int64)
		history.ResumeID = &resumeIDInt
	}
	
	return history, nil
}

func (m *ResumeHistoryModel) DeleteByID(id, userID int) error {
	query := `DELETE FROM resume_history WHERE id = $1 AND user_id = $2`
	result, err := m.DB.Exec(query, id, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (m *ResumeHistoryModel) CleanupOldResumes(userID int, keepCount int) error {
	query := `
		DELETE FROM resume_history
		WHERE user_id = $1
		AND id NOT IN (
			SELECT id FROM resume_history
			WHERE user_id = $1
			ORDER BY generated_at DESC
			LIMIT $2
		)
	`
	_, err := m.DB.Exec(query, userID, keepCount)
	return err
}
