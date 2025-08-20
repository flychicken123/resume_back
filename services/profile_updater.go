package services

import (
	"database/sql"
	"encoding/json"
	"log"
	"resumeai/models"
)

// ProfileUpdaterService updates user_job_profiles from resume data
type ProfileUpdaterService struct {
	db                  *sql.DB
	userJobProfileModel *models.UserJobProfileModel
	resumeModel         *models.ResumeModel
}

func NewProfileUpdaterService(db *sql.DB) *ProfileUpdaterService {
	return &ProfileUpdaterService{
		db:                  db,
		userJobProfileModel: models.NewUserJobProfileModel(db),
		resumeModel:         models.NewResumeModel(db),
	}
}

// UpdateProfileFromResume updates the user_job_profiles table with data extracted from resume
func (s *ProfileUpdaterService) UpdateProfileFromResume(userID int, resumeID int) error {
	log.Printf("Updating user profile from resume for user %d, resume %d", userID, resumeID)
	
	// Get the resume data
	resume, err := s.resumeModel.GetByID(resumeID)
	if err != nil {
		return err
	}
	
	// Get existing profile or create new one
	existingProfile, err := s.userJobProfileModel.GetByUserID(userID)
	
	profile := &models.UserJobProfile{}
	if err == nil && existingProfile != nil {
		// Start with existing profile
		*profile = *existingProfile
	}
	
	// Update with resume data (only update if resume has the data)
	if resume.Name != "" {
		profile.FullName = resume.Name
		log.Printf("Updated profile name: %s", resume.Name)
	}
	
	if resume.Email != "" {
		profile.Email = resume.Email
		log.Printf("Updated profile email: %s", resume.Email)
	}
	
	if resume.Phone != "" {
		profile.PhoneNumber = resume.Phone
		log.Printf("Updated profile phone: %s", resume.Phone)
	}
	
	// Extract location from resume summary if available
	if resume.Summary != nil {
		var summaryData map[string]interface{}
		if err := json.Unmarshal(resume.Summary, &summaryData); err == nil {
			// Try to extract location info from summary
			if location, ok := summaryData["location"].(string); ok && profile.City == "" {
				// Simple parsing - could be enhanced
				profile.City = location
			}
		}
	}
	
	// Set defaults if not already set
	if profile.Country == "" {
		profile.Country = "United States"
	}
	if profile.WorkAuthorization == "" {
		profile.WorkAuthorization = "yes"
	}
	if profile.AvailableStartDate == "" {
		profile.AvailableStartDate = "immediately"
	}
	
	// Save or update the profile
	err = s.userJobProfileModel.CreateOrUpdate(userID, profile)
	if err != nil {
		log.Printf("Failed to update user profile: %v", err)
		return err
	}
	
	log.Printf("Successfully updated user profile from resume")
	return nil
}

// GetMissingRequiredFields returns a list of required fields that are missing from the profile
func (s *ProfileUpdaterService) GetMissingRequiredFields(userID int) ([]string, error) {
	profile, err := s.userJobProfileModel.GetByUserID(userID)
	if err != nil {
		// No profile exists - all fields are missing
		return []string{
			"full_name",
			"email", 
			"phone_number",
			"linkedin_url",
			"work_authorization",
			"city",
			"country",
		}, nil
	}
	
	missing := []string{}
	
	// Check required fields
	if profile.FullName == "" {
		missing = append(missing, "full_name")
	}
	if profile.Email == "" {
		missing = append(missing, "email")
	}
	if profile.PhoneNumber == "" {
		missing = append(missing, "phone_number")
	}
	if profile.LinkedInURL == "" {
		missing = append(missing, "linkedin_url")
	}
	if profile.WorkAuthorization == "" {
		missing = append(missing, "work_authorization")
	}
	if profile.City == "" {
		missing = append(missing, "city")
	}
	if profile.Country == "" {
		missing = append(missing, "country")
	}
	
	return missing, nil
}