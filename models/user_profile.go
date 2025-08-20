package models

import (
	"database/sql"
	"time"
)

type UserProfile struct {
	ID                   int       `json:"id"`
	UserID               int       `json:"user_id"`
	FullName             string    `json:"full_name"`
	Phone                string    `json:"phone"`
	LocationCity         string    `json:"location_city"`
	LocationState        string    `json:"location_state"`
	LocationCountry      string    `json:"location_country"`
	CurrentJobTitle      string    `json:"current_job_title"`
	CurrentCompany       string    `json:"current_company"`
	YearsOfExperience    int       `json:"years_of_experience"`
	Skills               string    `json:"skills"` // JSON array
	ProfessionalSummary  string    `json:"professional_summary"`
	HighestDegree        string    `json:"highest_degree"`
	School               string    `json:"school"`
	FieldOfStudy         string    `json:"field_of_study"`
	GraduationYear       int       `json:"graduation_year"`
	GPA                  string    `json:"gpa"`
	WorkAuthorization    string    `json:"work_authorization"`
	StartDatePreference  string    `json:"start_date_preference"`
	NoticePeriod         string    `json:"notice_period"`
	SalaryExpectation    string    `json:"salary_expectation"`
	LinkedinURL          string    `json:"linkedin_url"`
	PortfolioURL         string    `json:"portfolio_url"`
	GithubURL            string    `json:"github_url"`
	LatestResumeS3Path   string    `json:"latest_resume_s3_path"`
	LastExtractedAt      time.Time `json:"last_extracted_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UserProfileModel struct {
	DB *sql.DB
}

func NewUserProfileModel(db *sql.DB) *UserProfileModel {
	return &UserProfileModel{DB: db}
}

func (m *UserProfileModel) GetByUserID(userID int) (*UserProfile, error) {
	profile := &UserProfile{}
	query := `
		SELECT id, user_id, COALESCE(full_name, '') as full_name, 
		       COALESCE(phone, '') as phone,
		       COALESCE(location_city, '') as location_city,
		       COALESCE(location_state, '') as location_state,
		       COALESCE(location_country, 'United States') as location_country,
		       COALESCE(current_job_title, '') as current_job_title,
		       COALESCE(current_company, '') as current_company,
		       COALESCE(years_of_experience, 0) as years_of_experience,
		       COALESCE(skills, '') as skills,
		       COALESCE(professional_summary, '') as professional_summary,
		       COALESCE(highest_degree, '') as highest_degree,
		       COALESCE(school, '') as school,
		       COALESCE(field_of_study, '') as field_of_study,
		       COALESCE(graduation_year, 0) as graduation_year,
		       COALESCE(gpa, '') as gpa,
		       COALESCE(work_authorization, 'Authorized to work in US') as work_authorization,
		       COALESCE(start_date_preference, 'Immediately') as start_date_preference,
		       COALESCE(notice_period, '2 weeks') as notice_period,
		       COALESCE(salary_expectation, 'Competitive') as salary_expectation,
		       COALESCE(linkedin_url, '') as linkedin_url,
		       COALESCE(portfolio_url, '') as portfolio_url,
		       COALESCE(github_url, '') as github_url,
		       COALESCE(latest_resume_s3_path, '') as latest_resume_s3_path,
		       COALESCE(last_extracted_at, '1970-01-01'::timestamp) as last_extracted_at,
		       created_at, updated_at
		FROM user_profiles WHERE user_id = $1
	`
	err := m.DB.QueryRow(query, userID).Scan(
		&profile.ID, &profile.UserID, &profile.FullName, &profile.Phone,
		&profile.LocationCity, &profile.LocationState, &profile.LocationCountry,
		&profile.CurrentJobTitle, &profile.CurrentCompany, &profile.YearsOfExperience,
		&profile.Skills, &profile.ProfessionalSummary, &profile.HighestDegree,
		&profile.School, &profile.FieldOfStudy, &profile.GraduationYear, &profile.GPA,
		&profile.WorkAuthorization, &profile.StartDatePreference, &profile.NoticePeriod,
		&profile.SalaryExpectation, &profile.LinkedinURL, &profile.PortfolioURL,
		&profile.GithubURL, &profile.LatestResumeS3Path, &profile.LastExtractedAt,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func (m *UserProfileModel) Upsert(profile *UserProfile) error {
	query := `
		INSERT INTO user_profiles (
			user_id, full_name, phone, location_city, location_state, location_country,
			current_job_title, current_company, years_of_experience, skills, professional_summary,
			highest_degree, school, field_of_study, graduation_year, gpa,
			work_authorization, start_date_preference, notice_period, salary_expectation,
			linkedin_url, portfolio_url, github_url, latest_resume_s3_path, last_extracted_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $26
		)
		ON CONFLICT (user_id) DO UPDATE SET
			full_name = $2, phone = $3, location_city = $4, location_state = $5,
			location_country = $6, current_job_title = $7, current_company = $8,
			years_of_experience = $9, skills = $10, professional_summary = $11,
			highest_degree = $12, school = $13, field_of_study = $14, graduation_year = $15,
			gpa = $16, work_authorization = $17, start_date_preference = $18,
			notice_period = $19, salary_expectation = $20, linkedin_url = $21,
			portfolio_url = $22, github_url = $23, latest_resume_s3_path = $24,
			last_extracted_at = $25, updated_at = $26
		RETURNING id, created_at, updated_at
	`
	
	now := time.Now()
	err := m.DB.QueryRow(query,
		profile.UserID, profile.FullName, profile.Phone, profile.LocationCity,
		profile.LocationState, profile.LocationCountry, profile.CurrentJobTitle,
		profile.CurrentCompany, profile.YearsOfExperience, profile.Skills,
		profile.ProfessionalSummary, profile.HighestDegree, profile.School,
		profile.FieldOfStudy, profile.GraduationYear, profile.GPA,
		profile.WorkAuthorization, profile.StartDatePreference, profile.NoticePeriod,
		profile.SalaryExpectation, profile.LinkedinURL, profile.PortfolioURL,
		profile.GithubURL, profile.LatestResumeS3Path, profile.LastExtractedAt,
		now,
	).Scan(&profile.ID, &profile.CreatedAt, &profile.UpdatedAt)
	
	return err
}

func (m *UserProfileModel) UpdateResumeS3Path(userID int, s3Path string) error {
	query := `
		UPDATE user_profiles 
		SET latest_resume_s3_path = $1, last_extracted_at = $2, updated_at = $2
		WHERE user_id = $3
	`
	_, err := m.DB.Exec(query, s3Path, time.Now(), userID)
	return err
}