package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Resume struct {
	ID        int             `json:"id"`
	UserID    int             `json:"user_id"`
	Name      string          `json:"name"`
	Summary   json.RawMessage `json:"summary"`
	Skills    json.RawMessage `json:"skills"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ResumeModel struct {
	DB *sql.DB
}

func NewResumeModel(db *sql.DB) *ResumeModel {
	return &ResumeModel{DB: db}
}

func (m *ResumeModel) GetByUserID(userID int) (*Resume, error) {
	resume := &Resume{}
	query := `
		SELECT id, user_id, name, summary, skills, created_at, updated_at
		FROM resumes WHERE user_id = $1
	`
	err := m.DB.QueryRow(query, userID).Scan(
		&resume.ID, &resume.UserID, &resume.Name, &resume.Summary, &resume.Skills, &resume.CreatedAt, &resume.UpdatedAt,
	)
	if err != nil {
		return nil, err
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
		_, err = m.DB.Exec("UPDATE resumes SET summary = $1, updated_at = NOW() WHERE user_id = $2",
			summary, userID)
	}

	return err
}

func (m *ResumeModel) GetSummary(userID int) (json.RawMessage, error) {
	var summary json.RawMessage
	query := `SELECT summary FROM resumes WHERE user_id = $1`
	err := m.DB.QueryRow(query, userID).Scan(&summary)
	return summary, err
}
