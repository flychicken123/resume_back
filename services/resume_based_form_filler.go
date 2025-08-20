package services

import (
	"fmt"
	"resumeai/models"
)

// ResumeBasedFormFiller uses resume-specific data to fill forms
type ResumeBasedFormFiller struct {
	resumeModel *models.EnhancedResumeModel
}

// NewResumeBasedFormFiller creates a new form filler
func NewResumeBasedFormFiller(resumeModel *models.EnhancedResumeModel) *ResumeBasedFormFiller {
	return &ResumeBasedFormFiller{
		resumeModel: resumeModel,
	}
}

// ConvertResumeToUserProfileData converts an EnhancedResume to UserProfileData for form filling
func (f *ResumeBasedFormFiller) ConvertResumeToUserProfileData(resume *models.EnhancedResume) *UserProfileData {
	if resume == nil {
		return &UserProfileData{}
	}
	
	// Convert experiences
	var experiences []ExperienceData
	for _, exp := range resume.Experiences {
		experiences = append(experiences, ExperienceData{
			Company:     exp.CompanyName,
			Title:       exp.JobTitle,
			StartDate:   exp.StartDate.Format("2006-01"),
			EndDate:     "",
			IsCurrent:   exp.IsCurrent,
			Description: exp.Description,
		})
		if exp.EndDate != nil {
			experiences[len(experiences)-1].EndDate = exp.EndDate.Format("2006-01")
		}
	}
	
	// Convert education
	var educations []EducationData
	for _, edu := range resume.Education {
		eduData := EducationData{
			Institution: edu.InstitutionName,
			Degree:      edu.Degree,
			Field:       edu.FieldOfStudy,
		}
		if edu.StartDate != nil {
			eduData.StartDate = edu.StartDate.Format("2006")
		}
		if edu.EndDate != nil {
			eduData.EndDate = edu.EndDate.Format("2006")
		}
		educations = append(educations, eduData)
	}
	
	// Get current company and title
	currentCompany := ""
	currentTitle := resume.CurrentTitle
	if len(resume.Experiences) > 0 {
		for _, exp := range resume.Experiences {
			if exp.IsCurrent {
				currentCompany = exp.CompanyName
				currentTitle = exp.JobTitle
				break
			}
		}
		// If no current, use most recent
		if currentCompany == "" && len(resume.Experiences) > 0 {
			currentCompany = resume.Experiences[0].CompanyName
			currentTitle = resume.Experiences[0].JobTitle
		}
	}
	
	// Get most recent school and degree
	recentSchool := ""
	recentDegree := ""
	if len(resume.Education) > 0 {
		recentSchool = resume.Education[0].InstitutionName
		recentDegree = resume.Education[0].Degree
	}
	
	return &UserProfileData{
		// Basic Information
		FirstName: resume.FirstName,
		LastName:  resume.LastName,
		FullName:  resume.FullName,
		Email:     resume.Email,
		Phone:     resume.Phone,
		
		// Location
		Address: resume.Address,
		City:    resume.City,
		State:   resume.State,
		Country: resume.Country,
		ZipCode: resume.ZipCode,
		
		// Links
		LinkedIn:  resume.LinkedInURL,
		Portfolio: resume.PortfolioURL,
		
		// Work Authorization
		WorkAuthorization:   resume.WorkAuthorization,
		RequiresSponsorship: resume.RequiresSponsorship,
		
		// Demographics (for forms that require it)
		Gender:           resume.Gender,
		Ethnicity:        resume.Ethnicity,
		VeteranStatus:    resume.VeteranStatus,
		DisabilityStatus: resume.DisabilityStatus,
		
		// Content
		Summary:      resume.Summary,
		Skills:       resume.Skills,
		Experience:   experiences,
		Education:    educations,
		
		// Extended fields (stored in the struct but not in the original definition)
		YearsOfExperience: resume.YearsOfExperience,
		CurrentCompany:    currentCompany,
		CurrentTitle:      currentTitle,
		RecentSchool:      recentSchool,
		RecentDegree:      recentDegree,
	}
}

// GetUserProfileDataForResume gets the form filling data for a specific resume
func (f *ResumeBasedFormFiller) GetUserProfileDataForResume(resumeID int) (*UserProfileData, error) {
	resume, err := f.resumeModel.GetResumeByID(resumeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resume: %v", err)
	}
	
	// Update last used timestamp
	f.resumeModel.UpdateResumeLastUsed(resumeID)
	
	return f.ConvertResumeToUserProfileData(resume), nil
}

// GetUserProfileDataForUser gets the form filling data for a user's default resume
func (f *ResumeBasedFormFiller) GetUserProfileDataForUser(userID int) (*UserProfileData, error) {
	resume, err := f.resumeModel.GetUserDefaultResume(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default resume: %v", err)
	}
	
	// Update last used timestamp
	f.resumeModel.UpdateResumeLastUsed(resume.ID)
	
	return f.ConvertResumeToUserProfileData(resume), nil
}

