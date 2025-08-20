package models

import (
	"database/sql"
	"time"
)

type Education struct {
	ID             int       `json:"id"`
	ResumeID       int       `json:"resume_id"`
	Degree         string    `json:"degree"`
	School         string    `json:"school"`
	Field          string    `json:"field"`
	GraduationYear int       `json:"graduation_year"`
	GPA            string    `json:"gpa"`
	Honors         string    `json:"honors"`
	Location       string    `json:"location"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type EducationModel struct {
	DB *sql.DB
}

func NewEducationModel(db *sql.DB) *EducationModel {
	return &EducationModel{DB: db}
}

func (m *EducationModel) GetByResumeID(resumeID int) ([]Education, error) {
	educations := []Education{}
	query := `
		SELECT id, resume_id, degree, school, field, 
		       COALESCE(graduation_year, 0) as graduation_year,
		       COALESCE(gpa, '') as gpa,
		       COALESCE(honors, '') as honors,
		       COALESCE(location, '') as location,
		       created_at, updated_at
		FROM education 
		WHERE resume_id = $1
		ORDER BY graduation_year DESC
	`
	rows, err := m.DB.Query(query, resumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var edu Education
		err := rows.Scan(&edu.ID, &edu.ResumeID, &edu.Degree, &edu.School, 
			&edu.Field, &edu.GraduationYear, &edu.GPA, &edu.Honors, 
			&edu.Location, &edu.CreatedAt, &edu.UpdatedAt)
		if err != nil {
			return nil, err
		}
		educations = append(educations, edu)
	}
	return educations, nil
}