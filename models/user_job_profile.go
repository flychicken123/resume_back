package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type UserJobProfile struct {
	ID                    int       `json:"id"`
	UserID                int       `json:"user_id"`
	FullName              string    `json:"full_name"`
	Email                 string    `json:"email"`
	PhoneNumber           string    `json:"phone_number"`
	Country               string    `json:"country"`
	City                  string    `json:"city"`
	State                 string    `json:"state"`
	ZipCode               string    `json:"zip_code"`
	Address               string    `json:"address"`
	LinkedInURL           string    `json:"linkedin_url"`
	PortfolioURL          string    `json:"portfolio_url"`
	WorkAuthorization     string    `json:"work_authorization"`     // "yes", "no", "requires_sponsorship"
	RequiresSponsorship   bool      `json:"requires_sponsorship"`
	WillingToRelocate     bool      `json:"willing_to_relocate"`
	SalaryExpectationMin  int       `json:"salary_expectation_min"`
	SalaryExpectationMax  int       `json:"salary_expectation_max"`
	PreferredLocations    string    `json:"preferred_locations"`    // JSON array of cities
	AvailableStartDate    string    `json:"available_start_date"`   // "immediately", "2_weeks", "1_month", etc.
	YearsOfExperience     int       `json:"years_of_experience"`
	Gender                string    `json:"gender"`                 // male, female, other, prefer_not_to_say
	Ethnicity             string    `json:"ethnicity"`
	VeteranStatus         string    `json:"veteran_status"`         // yes, no, prefer_not_to_say
	DisabilityStatus      string    `json:"disability_status"`      // yes, no, prefer_not_to_say
	SexualOrientation     string    `json:"sexual_orientation"`     // New field for demographic questions
	TransgenderStatus     string    `json:"transgender_status"`     // New field: Yes, No, Prefer not to answer
	MostRecentDegree      string    `json:"most_recent_degree"`     // New field: Bachelor's, Master's, PhD, etc.
	GraduationYear        int       `json:"graduation_year"`        // New field: Year of graduation
	University            string    `json:"university"`             // New field: University/Institution name
	Major                 string              `json:"major"`                  // New field: Major/Field of study
	ExtraQA               map[string]string   `json:"extra_qa"`              // Additional Q&A from job applications
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
}

type UserJobProfileModel struct {
	DB *sql.DB
}

func NewUserJobProfileModel(db *sql.DB) *UserJobProfileModel {
	return &UserJobProfileModel{DB: db}
}

func (m *UserJobProfileModel) GetByUserID(userID int) (*UserJobProfile, error) {
	profile := &UserJobProfile{}
	query := `
		SELECT id, user_id, 
		       COALESCE(full_name, '') as full_name,
		       COALESCE(email, '') as email,
		       COALESCE(phone_number, '') as phone_number,
		       COALESCE(country, 'United States') as country,
		       COALESCE(city, '') as city,
		       COALESCE(state, '') as state,
		       COALESCE(zip_code, '') as zip_code,
		       COALESCE(address, '') as address,
		       COALESCE(linkedin_url, '') as linkedin_url, 
		       COALESCE(portfolio_url, '') as portfolio_url,
		       COALESCE(work_authorization, 'yes') as work_authorization,
		       COALESCE(requires_sponsorship, false) as requires_sponsorship,
		       COALESCE(willing_to_relocate, false) as willing_to_relocate,
		       COALESCE(salary_expectation_min, 0) as salary_expectation_min,
		       COALESCE(salary_expectation_max, 0) as salary_expectation_max,
		       COALESCE(preferred_locations, '') as preferred_locations,
		       COALESCE(available_start_date, 'immediately') as available_start_date,
		       COALESCE(years_of_experience, 0) as years_of_experience,
		       COALESCE(gender, '') as gender,
		       COALESCE(ethnicity, '') as ethnicity,
		       COALESCE(veteran_status, '') as veteran_status,
		       COALESCE(disability_status, '') as disability_status,
		       COALESCE(extra_qa, '{}')::jsonb as extra_qa,
		       created_at, updated_at
		FROM user_job_profiles WHERE user_id = $1
	`
	var extraQAJSON []byte
	err := m.DB.QueryRow(query, userID).Scan(
		&profile.ID, &profile.UserID, 
		&profile.FullName, &profile.Email, &profile.PhoneNumber,
		&profile.Country, &profile.City, &profile.State, &profile.ZipCode, &profile.Address,
		&profile.LinkedInURL, &profile.PortfolioURL,
		&profile.WorkAuthorization, &profile.RequiresSponsorship, &profile.WillingToRelocate,
		&profile.SalaryExpectationMin, &profile.SalaryExpectationMax, &profile.PreferredLocations,
		&profile.AvailableStartDate, &profile.YearsOfExperience,
		&profile.Gender, &profile.Ethnicity, &profile.VeteranStatus, &profile.DisabilityStatus,
		&extraQAJSON,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err == nil && len(extraQAJSON) > 0 {
		json.Unmarshal(extraQAJSON, &profile.ExtraQA)
	}
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func (m *UserJobProfileModel) CreateOrUpdate(userID int, profile *UserJobProfile) error {
	// Check if profile exists
	var existingID int
	err := m.DB.QueryRow("SELECT id FROM user_job_profiles WHERE user_id = $1", userID).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Create new profile with defaults for empty fields
		if profile.Country == "" {
			profile.Country = "United States"
		}
		query := `
			INSERT INTO user_job_profiles (
				user_id, full_name, email, phone_number, country, city, state, zip_code, address,
				linkedin_url, portfolio_url, work_authorization, 
				requires_sponsorship, willing_to_relocate, salary_expectation_min, 
				salary_expectation_max, preferred_locations, available_start_date, 
				years_of_experience, gender, ethnicity, veteran_status, disability_status,
				sexual_orientation, transgender_status, most_recent_degree, graduation_year,
				university, major, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, NOW(), NOW())
		`
		_, err = m.DB.Exec(query, userID, profile.FullName, profile.Email, profile.PhoneNumber, profile.Country, profile.City, profile.State,
			profile.ZipCode, profile.Address, profile.LinkedInURL, profile.PortfolioURL,
			profile.WorkAuthorization, profile.RequiresSponsorship, profile.WillingToRelocate,
			profile.SalaryExpectationMin, profile.SalaryExpectationMax, profile.PreferredLocations,
			profile.AvailableStartDate, profile.YearsOfExperience, profile.Gender, profile.Ethnicity,
			profile.VeteranStatus, profile.DisabilityStatus, profile.SexualOrientation, profile.TransgenderStatus,
			profile.MostRecentDegree, profile.GraduationYear, profile.University, profile.Major)
	} else if err == nil {
		// Update existing profile
		query := `
			UPDATE user_job_profiles SET 
				full_name = $2, email = $3, phone_number = $4, country = $5, city = $6, state = $7, zip_code = $8, address = $9,
				linkedin_url = $10, portfolio_url = $11, work_authorization = $12,
				requires_sponsorship = $13, willing_to_relocate = $14, 
				salary_expectation_min = $15, salary_expectation_max = $16,
				preferred_locations = $17, available_start_date = $18,
				years_of_experience = $19, gender = $20, ethnicity = $21,
				veteran_status = $22, disability_status = $23, 
				sexual_orientation = $24, transgender_status = $25,
				most_recent_degree = $26, graduation_year = $27,
				university = $28, major = $29, updated_at = NOW()
			WHERE user_id = $1
		`
		_, err = m.DB.Exec(query, userID, profile.FullName, profile.Email, profile.PhoneNumber, profile.Country, profile.City, profile.State,
			profile.ZipCode, profile.Address, profile.LinkedInURL, profile.PortfolioURL,
			profile.WorkAuthorization, profile.RequiresSponsorship, profile.WillingToRelocate,
			profile.SalaryExpectationMin, profile.SalaryExpectationMax, profile.PreferredLocations,
			profile.AvailableStartDate, profile.YearsOfExperience, profile.Gender, profile.Ethnicity,
			profile.VeteranStatus, profile.DisabilityStatus, profile.SexualOrientation, profile.TransgenderStatus,
			profile.MostRecentDegree, profile.GraduationYear, profile.University, profile.Major)
	}
	
	return err
}

// FieldAnswer represents a saved answer with its field type
type FieldAnswer struct {
	Answer    string `json:"answer"`
	FieldType string `json:"field_type,omitempty"` // text, dropdown, checkbox, etc.
}

// UpdateExtraQA updates the extra_qa JSONB field with new Q&A pairs
func (m *UserJobProfileModel) UpdateExtraQA(userID int, newQA map[string]string) error {
	// First, get existing ExtraQA
	var existingQAJSON []byte
	err := m.DB.QueryRow("SELECT COALESCE(extra_qa, '{}')::jsonb FROM user_job_profiles WHERE user_id = $1", userID).Scan(&existingQAJSON)
	
	existingQA := make(map[string]string)
	if err == nil && len(existingQAJSON) > 0 {
		json.Unmarshal(existingQAJSON, &existingQA)
	} else if err == sql.ErrNoRows {
		// Profile doesn't exist, create a minimal one
		_, err = m.DB.Exec(`
			INSERT INTO user_job_profiles (user_id, extra_qa, created_at, updated_at) 
			VALUES ($1, $2::jsonb, NOW(), NOW())`,
			userID, "{}")
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	
	// Merge new Q&A with existing, new values override old ones
	for question, answer := range newQA {
		existingQA[question] = answer
	}
	
	// Convert back to JSON
	updatedQAJSON, err := json.Marshal(existingQA)
	if err != nil {
		return err
	}
	
	// Update the database
	_, err = m.DB.Exec(`
		UPDATE user_job_profiles 
		SET extra_qa = $2::jsonb, updated_at = NOW() 
		WHERE user_id = $1`,
		userID, string(updatedQAJSON))
	
	return err
}

// UpdateExtraQAWithTypes updates the extra_qa JSONB field with new Q&A pairs including field types
func (m *UserJobProfileModel) UpdateExtraQAWithTypes(userID int, newQA map[string]FieldAnswer) error {
	// First, get existing ExtraQA (could be old format or new format)
	var existingQAJSON []byte
	err := m.DB.QueryRow("SELECT COALESCE(extra_qa, '{}')::jsonb FROM user_job_profiles WHERE user_id = $1", userID).Scan(&existingQAJSON)
	
	// Try to unmarshal as new format first
	existingQA := make(map[string]FieldAnswer)
	var oldFormatQA map[string]string
	
	if err == nil && len(existingQAJSON) > 0 {
		// Try new format
		if err := json.Unmarshal(existingQAJSON, &existingQA); err != nil {
			// Fall back to old format
			if err := json.Unmarshal(existingQAJSON, &oldFormatQA); err == nil {
				// Convert old format to new format
				for q, a := range oldFormatQA {
					existingQA[q] = FieldAnswer{Answer: a, FieldType: "text"}
				}
			}
		}
	} else if err == sql.ErrNoRows {
		// Profile doesn't exist, create a minimal one
		_, err = m.DB.Exec(`
			INSERT INTO user_job_profiles (user_id, extra_qa, created_at, updated_at) 
			VALUES ($1, $2::jsonb, NOW(), NOW())`,
			userID, "{}")
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	
	// Merge new Q&A with existing, new values override old ones
	for question, fieldAnswer := range newQA {
		existingQA[question] = fieldAnswer
	}
	
	// Convert back to JSON
	updatedQAJSON, err := json.Marshal(existingQA)
	if err != nil {
		return err
	}
	
	// Update the database
	_, err = m.DB.Exec(`
		UPDATE user_job_profiles 
		SET extra_qa = $2::jsonb, updated_at = NOW() 
		WHERE user_id = $1`,
		userID, string(updatedQAJSON))
	
	return err
}