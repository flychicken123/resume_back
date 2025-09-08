package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// FormDataService handles fetching form data from the database
type FormDataService struct {
	db     *sql.DB
	mapper *QuestionMapper
}

// NewFormDataService creates a new form data service
func NewFormDataService(db *sql.DB) *FormDataService {
	return &FormDataService{
		db:     db,
		mapper: NewQuestionMapper(),
	}
}

// FormData represents all the data needed to fill a form
type FormData struct {
	ProfileData map[string]string `json:"profile_data"`
	ResumeData  map[string]string `json:"resume_data"`
	ExtraQA     map[string]string `json:"extra_qa"`
}

// GetFormDataForUser fetches all form data for a user
func (s *FormDataService) GetFormDataForUser(userID int, resumeHistoryID int) (*FormData, error) {
	formData := &FormData{
		ProfileData: make(map[string]string),
		ResumeData:  make(map[string]string),
		ExtraQA:     make(map[string]string),
	}
	
	// Fetch profile data
	if err := s.fetchProfileData(userID, formData); err != nil {
		log.Printf("Error fetching profile data: %v", err)
	}
	
	// Fetch resume data
	if err := s.fetchResumeData(userID, resumeHistoryID, formData); err != nil {
		log.Printf("Error fetching resume data: %v", err)
	}
	
	// Fetch extra QA data
	if err := s.fetchExtraQAData(userID, formData); err != nil {
		log.Printf("Error fetching extra QA data: %v", err)
	}
	
	return formData, nil
}

// fetchProfileData fetches data from user_profiles table
func (s *FormDataService) fetchProfileData(userID int, formData *FormData) error {
	query := `
		SELECT 
			COALESCE(first_name, ''),
			COALESCE(last_name, ''),
			COALESCE(full_name, ''),
			COALESCE(email, ''),
			COALESCE(phone, ''),
			COALESCE(address, ''),
			COALESCE(city, ''),
			COALESCE(state, ''),
			COALESCE(zip_code, ''),
			COALESCE(country, ''),
			COALESCE(linkedin, ''),
			COALESCE(github, ''),
			COALESCE(portfolio, ''),
			COALESCE(gender, ''),
			COALESCE(ethnicity, ''),
			COALESCE(veteran_status, ''),
			COALESCE(disability_status, ''),
			COALESCE(work_authorization, ''),
			COALESCE(requires_sponsorship::text, 'false'),
			COALESCE(extra_qa, '{}')
		FROM user_profiles
		WHERE user_id = $1
	`
	
	// Use temporary variables for scanning
	var (
		firstName, lastName, fullName, email, phone string
		address, city, state, zipCode, country string
		linkedin, github, portfolio string
		gender, ethnicity, veteranStatus, disabilityStatus string
		workAuthorization, requiresSponsorship string
		extraQA string
	)
	
	err := s.db.QueryRow(query, userID).Scan(
		&firstName, &lastName, &fullName, &email, &phone,
		&address, &city, &state, &zipCode, &country,
		&linkedin, &github, &portfolio,
		&gender, &ethnicity, &veteranStatus, &disabilityStatus,
		&workAuthorization, &requiresSponsorship,
		&extraQA,
	)
	
	if err == nil {
		// Assign to map after scanning
		formData.ProfileData["first_name"] = firstName
		formData.ProfileData["last_name"] = lastName
		formData.ProfileData["full_name"] = fullName
		formData.ProfileData["email"] = email
		formData.ProfileData["phone"] = phone
		formData.ProfileData["address"] = address
		formData.ProfileData["city"] = city
		formData.ProfileData["state"] = state
		formData.ProfileData["zip_code"] = zipCode
		formData.ProfileData["country"] = country
		formData.ProfileData["linkedin"] = linkedin
		formData.ProfileData["github"] = github
		formData.ProfileData["portfolio"] = portfolio
		formData.ProfileData["gender"] = gender
		formData.ProfileData["ethnicity"] = ethnicity
		formData.ProfileData["veteran_status"] = veteranStatus
		formData.ProfileData["disability_status"] = disabilityStatus
		formData.ProfileData["work_authorization"] = workAuthorization
		formData.ProfileData["requires_sponsorship"] = requiresSponsorship
	}
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error fetching profile data: %v", err)
	}
	
	// Parse extra_qa JSON from profile
	if extraQA != "" && extraQA != "{}" {
		s.parseExtraQA(extraQA, formData.ExtraQA)
	}
	
	// Generate full name if not set
	if formData.ProfileData["full_name"] == "" && 
	   (formData.ProfileData["first_name"] != "" || formData.ProfileData["last_name"] != "") {
		formData.ProfileData["full_name"] = strings.TrimSpace(
			formData.ProfileData["first_name"] + " " + formData.ProfileData["last_name"])
	}
	
	return nil
}

// fetchResumeData fetches data from resume_history table
func (s *FormDataService) fetchResumeData(userID int, resumeHistoryID int, formData *FormData) error {
	var query string
	var args []interface{}
	
	if resumeHistoryID > 0 {
		// Use specific resume
		query = `
			SELECT 
				COALESCE(resume_json, '{}'),
				COALESCE(parsed_content, '{}'),
				COALESCE(extra_qa, '{}')
			FROM resume_history
			WHERE id = $1 AND user_id = $2
		`
		args = []interface{}{resumeHistoryID, userID}
	} else {
		// Use most recent resume
		query = `
			SELECT 
				COALESCE(resume_json, '{}'),
				COALESCE(parsed_content, '{}'),
				COALESCE(extra_qa, '{}')
			FROM resume_history
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT 1
		`
		args = []interface{}{userID}
	}
	
	var resumeJSON, parsedContent, extraQA string
	err := s.db.QueryRow(query, args...).Scan(&resumeJSON, &parsedContent, &extraQA)
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error fetching resume data: %v", err)
	}
	
	// Parse resume JSON to extract education and experience
	if resumeJSON != "" && resumeJSON != "{}" {
		s.parseResumeJSON(resumeJSON, formData.ResumeData)
	}
	
	// Parse extra_qa JSON from resume
	if extraQA != "" && extraQA != "{}" {
		s.parseExtraQA(extraQA, formData.ExtraQA)
	}
	
	return nil
}

// fetchExtraQAData fetches additional Q&A data
func (s *FormDataService) fetchExtraQAData(userID int, formData *FormData) error {
	// Additional query for any other tables if needed
	// For now, extra_qa is fetched from both profile and resume tables
	return nil
}

// parseResumeJSON extracts relevant fields from resume JSON
func (s *FormDataService) parseResumeJSON(resumeJSON string, resumeData map[string]string) {
	var resume map[string]interface{}
	if err := json.Unmarshal([]byte(resumeJSON), &resume); err != nil {
		log.Printf("Error parsing resume JSON: %v", err)
		return
	}
	
	// Extract education information
	if education, ok := resume["education"].([]interface{}); ok && len(education) > 0 {
		if firstEdu, ok := education[0].(map[string]interface{}); ok {
			if school, ok := firstEdu["school"].(string); ok {
				resumeData["school"] = school
			}
			if degree, ok := firstEdu["degree"].(string); ok {
				resumeData["degree"] = degree
			}
			if major, ok := firstEdu["major"].(string); ok {
				resumeData["major"] = major
			}
			if gpa, ok := firstEdu["gpa"].(string); ok {
				resumeData["gpa"] = gpa
			}
			if gradDate, ok := firstEdu["graduation_date"].(string); ok {
				resumeData["graduation_date"] = gradDate
			}
		}
	}
	
	// Extract experience information
	if experience, ok := resume["experience"].([]interface{}); ok && len(experience) > 0 {
		// Get most recent (first) experience
		if firstExp, ok := experience[0].(map[string]interface{}); ok {
			if company, ok := firstExp["company"].(string); ok {
				resumeData["current_company"] = company
			}
			if position, ok := firstExp["position"].(string); ok {
				resumeData["current_position"] = position
			}
		}
		
		// Calculate total years of experience
		totalYears := 0
		for _, exp := range experience {
			if _, ok := exp.(map[string]interface{}); ok {
				// You might need to parse dates and calculate duration
				// For now, just count the number of experiences
				totalYears++
			}
		}
		resumeData["years_of_experience"] = fmt.Sprintf("%d", totalYears)
	}
	
	// Extract skills
	if skills, ok := resume["skills"].([]interface{}); ok {
		skillsList := []string{}
		for _, skill := range skills {
			if skillStr, ok := skill.(string); ok {
				skillsList = append(skillsList, skillStr)
			}
		}
		resumeData["skills"] = strings.Join(skillsList, ", ")
	}
	
	// Extract summary
	if summary, ok := resume["summary"].(string); ok {
		resumeData["summary"] = summary
	}
}

// parseExtraQA parses the extra_qa JSON column
func (s *FormDataService) parseExtraQA(extraQAJSON string, extraQA map[string]string) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(extraQAJSON), &data); err != nil {
		log.Printf("Error parsing extra_qa JSON: %v", err)
		return
	}
	
	for key, value := range data {
		switch v := value.(type) {
		case string:
			extraQA[key] = v
		case float64:
			extraQA[key] = fmt.Sprintf("%.0f", v)
		case bool:
			extraQA[key] = fmt.Sprintf("%v", v)
		default:
			// Convert to JSON string for complex types
			if jsonBytes, err := json.Marshal(v); err == nil {
				extraQA[key] = string(jsonBytes)
			}
		}
	}
}

// GetAnswerForQuestion gets the answer for a specific question
func (s *FormDataService) GetAnswerForQuestion(question string, formData *FormData) (string, bool) {
	// First, try to find a mapping for this question
	if mapping, found := s.mapper.FindMapping(question); found {
		// Check which table the field belongs to
		switch mapping.TableName {
		case "user_profiles":
			if value, exists := formData.ProfileData[mapping.FieldName]; exists && value != "" {
				return value, true
			}
		case "resume_history":
			if value, exists := formData.ResumeData[mapping.FieldName]; exists && value != "" {
				return value, true
			}
		}
	}
	
	// If no mapping found or no value, check extra_qa
	// Try different variations of the question as key
	questionLower := strings.ToLower(question)
	questionKey := strings.ReplaceAll(questionLower, " ", "_")
	
	// Check exact match first
	if value, exists := formData.ExtraQA[question]; exists && value != "" {
		return value, true
	}
	
	// Check lowercase version
	if value, exists := formData.ExtraQA[questionLower]; exists && value != "" {
		return value, true
	}
	
	// Check with underscores
	if value, exists := formData.ExtraQA[questionKey]; exists && value != "" {
		return value, true
	}
	
	// Check if question contains any key from extra_qa
	for key, value := range formData.ExtraQA {
		if strings.Contains(questionLower, strings.ToLower(key)) ||
		   strings.Contains(strings.ToLower(key), questionLower) {
			return value, true
		}
	}
	
	return "", false
}