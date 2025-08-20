package models

import (
	"database/sql"
	"time"
)

type Experience struct {
	ID               int       `json:"id"`
	ResumeID         int       `json:"resume_id"`
	JobTitle         string    `json:"job_title"`
	Company          string    `json:"company"`
	City             string    `json:"city"`
	State            string    `json:"state"`
	StartDate        string    `json:"start_date"`
	EndDate          string    `json:"end_date"`
	CurrentlyWorking bool      `json:"currently_working"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ExperienceModel struct {
	DB *sql.DB
}

func NewExperienceModel(db *sql.DB) *ExperienceModel {
	return &ExperienceModel{DB: db}
}

func (m *ExperienceModel) GetByResumeID(resumeID int) ([]Experience, error) {
	experiences := []Experience{}
	query := `
		SELECT id, resume_id, job_title, company, city, state, 
		       COALESCE(start_date::text, '') as start_date,
		       COALESCE(end_date::text, '') as end_date,
		       currently_working, description, created_at, updated_at
		FROM experiences 
		WHERE resume_id = $1
		ORDER BY start_date DESC
	`
	rows, err := m.DB.Query(query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var exp Experience
		err := rows.Scan(&exp.ID, &exp.ResumeID, &exp.JobTitle, &exp.Company, 
			&exp.City, &exp.State, &exp.StartDate, &exp.EndDate, 
			&exp.CurrentlyWorking, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
		if err != nil {
			return nil, err
		}
		experiences = append(experiences, exp)
	}
	return experiences, nil
}