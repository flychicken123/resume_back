package models

import (
	"database/sql"
	"time"
)

type ResumeHistory struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	ResumeName  string    `json:"resume_name"`
	S3Path      string    `json:"s3_path"`
	GeneratedAt time.Time `json:"generated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type ResumeHistoryModel struct {
	DB *sql.DB
}

func NewResumeHistoryModel(db *sql.DB) *ResumeHistoryModel {
	return &ResumeHistoryModel{DB: db}
}

func (m *ResumeHistoryModel) Create(userID int, resumeName, s3Path string) (*ResumeHistory, error) {
	history := &ResumeHistory{}
	query := `
		INSERT INTO resume_history (user_id, resume_name, s3_path, generated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, resume_name, s3_path, generated_at, created_at
	`
	err := m.DB.QueryRow(query, userID, resumeName, s3Path, time.Now()).Scan(
		&history.ID, &history.UserID, &history.ResumeName, &history.S3Path, &history.GeneratedAt, &history.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func (m *ResumeHistoryModel) GetByUserID(userID int) ([]*ResumeHistory, error) {
	query := `
		SELECT id, user_id, resume_name, s3_path, generated_at, created_at
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
		err := rows.Scan(
			&history.ID, &history.UserID, &history.ResumeName, &history.S3Path, &history.GeneratedAt, &history.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		histories = append(histories, history)
	}
	return histories, nil
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
