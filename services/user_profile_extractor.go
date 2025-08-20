package services

import (
	"strings"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"resumeai/models"
)

type UserProfileExtractor struct {
	db                  *sql.DB
	userModel         *models.UserModel
	resumeModel       *models.ResumeModel
	experienceModel   *models.ExperienceModel
	educationModel    *models.EducationModel
	resumeHistoryModel  *models.ResumeHistoryModel
	userJobProfileModel *models.UserJobProfileModel
}

func NewUserProfileExtractor(db *sql.DB) *UserProfileExtractor {
	return &UserProfileExtractor{
		db:                  db,
		userModel:         models.NewUserModel(db),
		resumeModel:       models.NewResumeModel(db),
		experienceModel:   models.NewExperienceModel(db),
		resumeHistoryModel: models.NewResumeHistoryModel(db),
		educationModel:    models.NewEducationModel(db),
		userJobProfileModel: models.NewUserJobProfileModel(db),
	}
}

func (e *UserProfileExtractor) ExtractUserProfileWithResume(userID int, resumeHistoryID int) (*UserProfileData, error) {
	log.Printf("Extracting user profile for user ID: %d with resume history ID: %d", userID, resumeHistoryID)
	
	// Get the base profile first
	profile, err := e.ExtractUserProfile(userID)
	if err != nil {
		return nil, err
	}
	
	// First try to get contact info from the specific resume_history entry
	historyQuery := `
		SELECT COALESCE(contact_name, ''), COALESCE(contact_email, ''), COALESCE(contact_phone, '')
		FROM resume_history
		WHERE id = $1
	`
	
	var historyName, historyEmail, historyPhone string
	err = e.db.QueryRow(historyQuery, resumeHistoryID).Scan(&historyName, &historyEmail, &historyPhone)
	if err == nil && (historyName != "" || historyEmail != "" || historyPhone != "") {
		// Use the contact info from resume_history
		log.Printf("Using contact info from resume_history ID %d", resumeHistoryID)
		if historyName != "" && !strings.HasPrefix(historyName, "Resume ") {
			profile.FullName = historyName
			// Clear FirstName and LastName so they get re-split from the new FullName
			profile.FirstName = ""
			profile.LastName = ""
			log.Printf("Using name from resume_history: %s", historyName)
		}
		if historyEmail != "" {
			profile.Email = historyEmail
			log.Printf("Using email from resume_history: %s", historyEmail)
		}
		if historyPhone != "" {
			profile.Phone = historyPhone
			log.Printf("Using phone from resume_history: %s", historyPhone)
		}
	} else {
		// Fallback to resumes table if resume_history doesn't have the info
		log.Printf("No contact info in resume_history, falling back to resumes table")
		query := `
			SELECT r.name, r.email, r.phone
			FROM resumes r
			WHERE r.user_id = $1
			ORDER BY r.updated_at DESC
			LIMIT 1
		`
		
		var resumeName, resumeEmail, resumePhone sql.NullString
		err = e.db.QueryRow(query, userID).Scan(&resumeName, &resumeEmail, &resumePhone)
		if err != nil {
			log.Printf("Warning: Could not get latest resume data: %v", err)
		} else {
			// Override with actual resume data if available
			log.Printf("DEBUG: Resume query returned - Name: '%s', Email: '%s', Phone: '%s'", resumeName.String, resumeEmail.String, resumePhone.String)
			if resumeName.Valid && resumeName.String != "" {
				// Check if this looks like a filename rather than a person's name
				if strings.HasPrefix(resumeName.String, "Resume ") || strings.Contains(resumeName.String, "2024-") || strings.Contains(resumeName.String, "2025-") {
					log.Printf("WARNING: Resume name looks like a filename: %s, ignoring", resumeName.String)
				} else {
					log.Printf("Setting FullName from resume table to: '%s' (was: '%s')", resumeName.String, profile.FullName)
					profile.FullName = resumeName.String
					// Clear FirstName and LastName so they get re-split from the new FullName
					profile.FirstName = ""
					profile.LastName = ""
					log.Printf("Using name from latest resume: %s", resumeName.String)
				}
			}
			if resumeEmail.Valid && resumeEmail.String != "" {
				profile.Email = resumeEmail.String
				log.Printf("Using email from latest resume: %s", resumeEmail.String)
			}
			if resumePhone.Valid && resumePhone.String != "" {
				profile.Phone = resumePhone.String
				log.Printf("Using phone from latest resume: %s", resumePhone.String)
			}
		}
	}
	
	// Split FullName into FirstName and LastName
	log.Printf("DEBUG: Before splitting - FullName: '%s', FirstName: '%s', LastName: '%s'", profile.FullName, profile.FirstName, profile.LastName)
	profile.FirstName, profile.LastName = splitFullName(profile.FullName)
	log.Printf("DEBUG: After splitting - FullName: '%s', FirstName: '%s', LastName: '%s'", profile.FullName, profile.FirstName, profile.LastName)
	return profile, nil
}

func (e *UserProfileExtractor) ExtractUserProfile(userID int) (*UserProfileData, error) {
	log.Printf("Extracting user profile for user ID: %d", userID)

	// Get user basic info
	user, err := e.userModel.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get resume data
	resume, err := e.resumeModel.GetLatestByUserID(userID)
	if err != nil {
		log.Printf("No resume found for user %d, using basic user data", userID)
		// Continue with just user data
	}

	profile := &UserProfileData{
		FullName: user.Name, // Default to user account name
		Email:    user.Email,
	}

	// Extract data from resume if available
	if resume != nil {
		// Use resume name if available (this is the actual applicant's name)
		if resume.Name != "" {
			// Check if this looks like a filename rather than a person's name
			if strings.HasPrefix(resume.Name, "Resume ") || strings.Contains(resume.Name, "2024-") || strings.Contains(resume.Name, "2025-") {
				log.Printf("WARNING: Resume name looks like a filename: %s, using user name instead", resume.Name)
			} else {
				profile.FullName = resume.Name
			log.Printf("Using name from resume: %s", resume.Name)
			}
		}
		if resume.Email != "" {
			profile.Email = resume.Email
		}
		if resume.Phone != "" {
			profile.Phone = resume.Phone
		}

		// Extract summary from JSON
		if resume.Summary != nil {
			var summaryData map[string]interface{}
			if err := json.Unmarshal(resume.Summary, &summaryData); err == nil {
				if summaryText, ok := summaryData["summary"].(string); ok {
					profile.Summary = summaryText
				}
			}
		}

		// Extract skills from JSON
		if resume.Skills != nil {
			var skillsData interface{}
			if err := json.Unmarshal(resume.Skills, &skillsData); err == nil {
				profile.Skills = extractSkillsFromJSON(skillsData)
			}
		}
	}

	// Get experience and education data from resume JSON columns
	if resume != nil {
		// Extract experiences from JSON column
		if resume.Experiences != nil && len(resume.Experiences) > 0 {
			var experiences []ExperienceData
			if err := json.Unmarshal(resume.Experiences, &experiences); err == nil {
				profile.Experience = experiences
				log.Printf("Loaded %d experiences from resume JSON", len(experiences))
			} else {
				log.Printf("Failed to unmarshal experiences: %v", err)
			}
		}

		// Extract educations from JSON column
		if resume.Educations != nil && len(resume.Educations) > 0 {
			var educations []EducationData
			if err := json.Unmarshal(resume.Educations, &educations); err == nil {
				profile.Education = educations
				log.Printf("Loaded %d educations from resume JSON", len(educations))
			} else {
				log.Printf("Failed to unmarshal educations: %v", err)
			}
		}

		// Fallback to old method if JSON columns are empty
		if len(profile.Experience) == 0 {
			experiences, err := e.experienceModel.GetByResumeID(resume.ID)
			if err == nil && len(experiences) > 0 {
				profile.Experience = convertToExperienceData(experiences)
				log.Printf("Loaded %d experiences from experiences table (fallback)", len(experiences))
			}
		}

		if len(profile.Education) == 0 {
			educations, err := e.educationModel.GetByResumeID(resume.ID)
			if err == nil && len(educations) > 0 {
				profile.Education = convertToEducationData(educations)
				log.Printf("Loaded %d educations from educations table (fallback)", len(educations))
			}
		}
	}

	// Get job profile data (this is the most up-to-date source)
	jobProfile, err := e.userJobProfileModel.GetByUserID(userID)
	if err == nil {
		// Use name and email from job profile if available (most recent/accurate)
		log.Printf("Job profile data - FullName: '%s', Gender: '%s', Ethnicity: '%s'", jobProfile.FullName, jobProfile.Gender, jobProfile.Ethnicity)
		if jobProfile.FullName != "" {
			profile.FullName = jobProfile.FullName
		}
		if jobProfile.Email != "" {
			profile.Email = jobProfile.Email
		}
		// Use phone from job profile if available, otherwise keep resume phone
		if jobProfile.PhoneNumber != "" {
			profile.Phone = jobProfile.PhoneNumber
		}
		// Location data
		if jobProfile.Country != "" {
			profile.Country = jobProfile.Country
		}
		if jobProfile.City != "" {
			profile.City = jobProfile.City
		}
		if jobProfile.State != "" {
			profile.State = jobProfile.State
		}
		if jobProfile.ZipCode != "" {
			profile.ZipCode = jobProfile.ZipCode
		}
		if jobProfile.Address != "" {
			profile.Address = jobProfile.Address
		}
		// Professional info
		profile.LinkedIn = jobProfile.LinkedInURL
		profile.Portfolio = jobProfile.PortfolioURL
		profile.WorkAuthorization = jobProfile.WorkAuthorization
		profile.RequiresSponsorship = jobProfile.RequiresSponsorship
		profile.WillingToRelocate = jobProfile.WillingToRelocate
		profile.SalaryExpectationMin = jobProfile.SalaryExpectationMin
		profile.SalaryExpectationMax = jobProfile.SalaryExpectationMax
		profile.PreferredLocations = jobProfile.PreferredLocations
		profile.AvailableStartDate = jobProfile.AvailableStartDate
		profile.YearsOfExperience = jobProfile.YearsOfExperience
		// Demographic info
		profile.Gender = jobProfile.Gender
		profile.Ethnicity = jobProfile.Ethnicity
		profile.VeteranStatus = jobProfile.VeteranStatus
		profile.DisabilityStatus = jobProfile.DisabilityStatus
		// New demographic and education fields
		profile.SexualOrientation = jobProfile.SexualOrientation
		profile.TransgenderStatus = jobProfile.TransgenderStatus
		profile.MostRecentDegree = jobProfile.MostRecentDegree
		profile.GraduationYear = jobProfile.GraduationYear
		profile.University = jobProfile.University
		profile.Major = jobProfile.Major
		// Extra Q&A from user preferences
		profile.ExtraQA = jobProfile.ExtraQA
	} else {
		log.Printf("No job profile found for user %d, using defaults", userID)
		// Set default job profile values
		profile.WorkAuthorization = "yes"
		profile.RequiresSponsorship = false
		profile.WillingToRelocate = false
		profile.AvailableStartDate = "immediately"
		profile.YearsOfExperience = 2
	}

	// Set default values if missing
	if profile.FullName == "" {
		profile.FullName = "John Doe"
	}
	if profile.Phone == "" {
		profile.Phone = "(555) 123-4567"
	}
	if profile.Country == "" {
		profile.Country = "United States"
	}
	if profile.Address == "" && profile.City == "" {
		profile.Address = "123 Main St"
		profile.City = "Anytown"
		profile.State = "CA"
		profile.ZipCode = "12345"
	}

	// Split FullName into FirstName and LastName only if they're not already set
	if profile.FirstName == "" && profile.LastName == "" {
		log.Printf("DEBUG: Splitting name - FullName: '%s'", profile.FullName)
		profile.FirstName, profile.LastName = splitFullName(profile.FullName)
		log.Printf("DEBUG: After splitting - FirstName: '%s', LastName: '%s'", profile.FirstName, profile.LastName)
	}
	
	log.Printf("Successfully extracted profile for user: %s", profile.FullName)
	log.Printf("Job profile - Work auth: %s, Sponsorship: %v, LinkedIn: %s", 
		profile.WorkAuthorization, profile.RequiresSponsorship, profile.LinkedIn)
	return profile, nil
}

func extractSkillsFromJSON(skillsData interface{}) []string {
	skills := []string{}

	switch v := skillsData.(type) {
	case []interface{}:
		for _, skill := range v {
			if skillStr, ok := skill.(string); ok {
				skills = append(skills, skillStr)
			}
		}
	case map[string]interface{}:
		if skillsList, ok := v["skills"].([]interface{}); ok {
			for _, skill := range skillsList {
				if skillStr, ok := skill.(string); ok {
					skills = append(skills, skillStr)
				}
			}
		}
	case string:
		// If skills is a comma-separated string
		skillsStr := strings.TrimSpace(v)
		if skillsStr != "" {
			skills = strings.Split(skillsStr, ",")
			for i, skill := range skills {
				skills[i] = strings.TrimSpace(skill)
			}
		}
	}

	return skills
}

func convertToExperienceData(experiences []models.Experience) []ExperienceData {
	result := []ExperienceData{}
	
	for _, exp := range experiences {
		experience := ExperienceData{
			Title:       exp.JobTitle,
			Company:     exp.Company,
			StartDate:   exp.StartDate, // Already a string
			Description: exp.Description,
			IsCurrent:   exp.CurrentlyWorking,
		}
		
		if exp.CurrentlyWorking {
			experience.EndDate = "Present"
		} else {
			experience.EndDate = exp.EndDate // Already a string
		}
		
		result = append(result, experience)
	}
	
	return result
}

func convertToEducationData(educations []models.Education) []EducationData {
	result := []EducationData{}
	
	for _, edu := range educations {
		education := EducationData{
			Degree:      edu.Degree,
			Institution: edu.School,
			Field:       edu.Field,
			StartDate:   fmt.Sprintf("%d", edu.GraduationYear-4), // Estimate start year
			EndDate:     fmt.Sprintf("%d", edu.GraduationYear),
			GPA:         edu.GPA,
			Description: fmt.Sprintf("%s in %s", edu.Degree, edu.Field),
		}
		
		result = append(result, education)
	}
	
	return result
}
// splitFullName splits a full name into first and last name
func splitFullName(fullName string) (firstName, lastName string) {
	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return "", ""
	}
	firstName = parts[0]
	if len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}
	return firstName, lastName
}
