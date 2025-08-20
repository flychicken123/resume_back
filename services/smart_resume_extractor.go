package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"resumeai/models"
)

type SmartResumeExtractor struct {
	userModel        *models.UserModel
	userProfileModel *models.UserProfileModel
	s3Extractor      *S3ResumeExtractor
}

type SmartExtractedData struct {
	// Personal Information
	FullName    string `json:"full_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	
	// Location
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	
	// Professional Info
	CurrentJobTitle    string   `json:"current_job_title"`
	CurrentCompany     string   `json:"current_company"`
	YearsOfExperience  int      `json:"years_of_experience"`
	Summary            string   `json:"summary"`
	Skills             []string `json:"skills"`
	
	// Education
	HighestDegree      string `json:"highest_degree"`
	School             string `json:"school"`
	FieldOfStudy       string `json:"field_of_study"`
	GraduationYear     int    `json:"graduation_year"`
	GPA                string `json:"gpa"`
	
	// Work Authorization & Preferences
	WorkAuthorization  string `json:"work_authorization"`
	StartDate          string `json:"start_date"`
	NoticePeriod       string `json:"notice_period"`
	SalaryExpectation  string `json:"salary_expectation"`
	
	// Social Links
	LinkedIn           string `json:"linkedin"`
	Portfolio          string `json:"portfolio"`
	GitHub             string `json:"github"`
	
	// Experience Details (from S3 if needed)
	DetailedExperience []ExtractedExperience `json:"detailed_experience,omitempty"`
	
	// Data Source Info
	DataSource         string `json:"data_source"` // "profile", "s3_extraction", "hybrid"
	LastUpdated        time.Time `json:"last_updated"`
}

func NewSmartResumeExtractor(
	userModel *models.UserModel,
	userProfileModel *models.UserProfileModel,
	s3Extractor *S3ResumeExtractor,
) *SmartResumeExtractor {
	return &SmartResumeExtractor{
		userModel:        userModel,
		userProfileModel: userProfileModel,
		s3Extractor:      s3Extractor,
	}
}

func (e *SmartResumeExtractor) ExtractUserData(userID, resumeID int) (*SmartExtractedData, error) {
	// First, try to get data from user profile (fast)
	profile, err := e.userProfileModel.GetByUserID(userID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get user profile: %v", err)
	}

	// Get user basic info
	user, err := e.userModel.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %v", err)
	}

	extractedData := &SmartExtractedData{}

	if profile != nil && profile.LastExtractedAt.After(time.Now().AddDate(0, 0, -30)) {
		// Use cached profile data (less than 30 days old)
		e.populateFromProfile(profile, user, extractedData)
		extractedData.DataSource = "profile"
		extractedData.LastUpdated = profile.UpdatedAt
	} else {
		// Need to extract from S3 or create new profile
		if profile != nil && profile.LatestResumeS3Path != "" {
			// Extract from latest S3 resume
			s3Data, err := e.s3Extractor.ExtractFromS3(profile.LatestResumeS3Path)
			if err == nil {
				e.populateFromS3Data(s3Data, user, extractedData)
				extractedData.DataSource = "s3_extraction"
				
				// Update profile with extracted data
				e.updateProfileFromS3Data(profile, s3Data, userID)
			} else {
				// Fallback to profile data if S3 extraction fails
				if profile != nil {
					e.populateFromProfile(profile, user, extractedData)
					extractedData.DataSource = "profile_fallback"
				} else {
					return nil, fmt.Errorf("no profile data and S3 extraction failed: %v", err)
				}
			}
		} else {
			// No S3 resume available, use basic user data and create minimal profile
			e.populateFromUserOnly(user, extractedData)
			extractedData.DataSource = "user_basic"
			
			// Create minimal profile for future use
			e.createMinimalProfile(user, userID)
		}
		extractedData.LastUpdated = time.Now()
	}

	return extractedData, nil
}

func (e *SmartResumeExtractor) ExtractWithExperience(userID, resumeID int) (*SmartExtractedData, error) {
	// Get basic data first
	data, err := e.ExtractUserData(userID, resumeID)
	if err != nil {
		return nil, err
	}

	// Always extract detailed experience from S3 if available
	profile, _ := e.userProfileModel.GetByUserID(userID)
	if profile != nil && profile.LatestResumeS3Path != "" {
		s3Data, err := e.s3Extractor.ExtractFromS3(profile.LatestResumeS3Path)
		if err == nil {
			data.DetailedExperience = s3Data.Experience
			data.DataSource = "hybrid"
		}
	}

	return data, nil
}

func (e *SmartResumeExtractor) populateFromProfile(profile *models.UserProfile, user *models.User, data *SmartExtractedData) {
	// Personal info
	data.FullName = profile.FullName
	if data.FullName == "" {
		data.FullName = user.Name
	}
	e.splitName(data.FullName, data)
	data.Email = user.Email
	data.Phone = profile.Phone
	
	// Location
	data.City = profile.LocationCity
	data.State = profile.LocationState
	data.Country = profile.LocationCountry
	
	// Professional
	data.CurrentJobTitle = profile.CurrentJobTitle
	data.CurrentCompany = profile.CurrentCompany
	data.YearsOfExperience = profile.YearsOfExperience
	data.Summary = profile.ProfessionalSummary
	
	// Skills
	if profile.Skills != "" {
		json.Unmarshal([]byte(profile.Skills), &data.Skills)
	}
	
	// Education
	data.HighestDegree = profile.HighestDegree
	data.School = profile.School
	data.FieldOfStudy = profile.FieldOfStudy
	data.GraduationYear = profile.GraduationYear
	data.GPA = profile.GPA
	
	// Preferences
	data.WorkAuthorization = profile.WorkAuthorization
	data.StartDate = profile.StartDatePreference
	data.NoticePeriod = profile.NoticePeriod
	data.SalaryExpectation = profile.SalaryExpectation
	
	// Social links
	data.LinkedIn = profile.LinkedinURL
	data.Portfolio = profile.PortfolioURL
	data.GitHub = profile.GithubURL
}

func (e *SmartResumeExtractor) populateFromS3Data(s3Data *S3ResumeData, user *models.User, data *SmartExtractedData) {
	// Personal info
	data.FullName = s3Data.PersonalInfo.Name
	if data.FullName == "" {
		data.FullName = user.Name
	}
	e.splitName(data.FullName, data)
	
	data.Email = s3Data.PersonalInfo.Email
	if data.Email == "" {
		data.Email = user.Email
	}
	data.Phone = s3Data.PersonalInfo.Phone
	
	// Parse location
	if s3Data.PersonalInfo.Location != "" {
		locationParts := strings.Split(s3Data.PersonalInfo.Location, ",")
		if len(locationParts) >= 1 {
			data.City = strings.TrimSpace(locationParts[0])
		}
		if len(locationParts) >= 2 {
			data.State = strings.TrimSpace(locationParts[1])
		}
	}
	data.Country = "United States"
	
	// Professional info from experience
	if len(s3Data.Experience) > 0 {
		// Get current job (first in list or currently working)
		var currentJob *ExtractedExperience
		for i := range s3Data.Experience {
			if s3Data.Experience[i].CurrentlyWorking {
				currentJob = &s3Data.Experience[i]
				break
			}
		}
		if currentJob == nil && len(s3Data.Experience) > 0 {
			currentJob = &s3Data.Experience[0]
		}
		
		if currentJob != nil {
			data.CurrentJobTitle = currentJob.JobTitle
			data.CurrentCompany = currentJob.Company
			if currentJob.Location != "" && data.City == "" {
				locationParts := strings.Split(currentJob.Location, ",")
				if len(locationParts) >= 1 {
					data.City = strings.TrimSpace(locationParts[0])
				}
				if len(locationParts) >= 2 {
					data.State = strings.TrimSpace(locationParts[1])
				}
			}
		}
		
		// Calculate years of experience
		data.YearsOfExperience = e.calculateTotalExperience(s3Data.Experience)
	}
	
	data.Summary = s3Data.Summary
	data.Skills = s3Data.Skills
	
	// Education
	if len(s3Data.Education) > 0 {
		edu := s3Data.Education[0] // Use first/most recent
		data.HighestDegree = edu.Degree
		data.School = edu.School
		data.FieldOfStudy = edu.Field
		data.GraduationYear = edu.GraduationYear
		data.GPA = edu.GPA
	}
	
	// Social links
	data.LinkedIn = s3Data.PersonalInfo.LinkedIn
	data.Portfolio = s3Data.PersonalInfo.Portfolio
	data.GitHub = s3Data.PersonalInfo.GitHub
	
	// Set intelligent defaults
	e.setDefaults(data)
}

func (e *SmartResumeExtractor) populateFromUserOnly(user *models.User, data *SmartExtractedData) {
	data.FullName = user.Name
	e.splitName(data.FullName, data)
	data.Email = user.Email
	data.Country = "United States"
	
	// Set defaults
	e.setDefaults(data)
}

func (e *SmartResumeExtractor) createMinimalProfile(user *models.User, userID int) {
	profile := &models.UserProfile{
		UserID:              userID,
		FullName:           user.Name,
		LocationCountry:    "United States",
		WorkAuthorization:  "Authorized to work in US",
		StartDatePreference: "Immediately",
		NoticePeriod:       "2 weeks",
		SalaryExpectation:  "Competitive",
		LastExtractedAt:    time.Now(),
	}
	
	e.userProfileModel.Upsert(profile)
}

func (e *SmartResumeExtractor) updateProfileFromS3Data(profile *models.UserProfile, s3Data *S3ResumeData, userID int) {
	if profile == nil {
		profile = &models.UserProfile{UserID: userID}
	}
	
	profile.FullName = s3Data.PersonalInfo.Name
	profile.Phone = s3Data.PersonalInfo.Phone
	profile.ProfessionalSummary = s3Data.Summary
	profile.LinkedinURL = s3Data.PersonalInfo.LinkedIn
	profile.PortfolioURL = s3Data.PersonalInfo.Portfolio
	profile.GithubURL = s3Data.PersonalInfo.GitHub
	
	// Parse location
	if s3Data.PersonalInfo.Location != "" {
		locationParts := strings.Split(s3Data.PersonalInfo.Location, ",")
		if len(locationParts) >= 1 {
			profile.LocationCity = strings.TrimSpace(locationParts[0])
		}
		if len(locationParts) >= 2 {
			profile.LocationState = strings.TrimSpace(locationParts[1])
		}
	}
	
	// Experience info
	if len(s3Data.Experience) > 0 {
		currentJob := s3Data.Experience[0]
		profile.CurrentJobTitle = currentJob.JobTitle
		profile.CurrentCompany = currentJob.Company
		profile.YearsOfExperience = e.calculateTotalExperience(s3Data.Experience)
	}
	
	// Skills
	if len(s3Data.Skills) > 0 {
		skillsJSON, _ := json.Marshal(s3Data.Skills)
		profile.Skills = string(skillsJSON)
	}
	
	// Education
	if len(s3Data.Education) > 0 {
		edu := s3Data.Education[0]
		profile.HighestDegree = edu.Degree
		profile.School = edu.School
		profile.FieldOfStudy = edu.Field
		profile.GraduationYear = edu.GraduationYear
		profile.GPA = edu.GPA
	}
	
	profile.LastExtractedAt = time.Now()
	e.userProfileModel.Upsert(profile)
}

func (e *SmartResumeExtractor) splitName(fullName string, data *SmartExtractedData) {
	nameParts := strings.Fields(fullName)
	if len(nameParts) >= 1 {
		data.FirstName = nameParts[0]
	}
	if len(nameParts) >= 2 {
		data.LastName = nameParts[len(nameParts)-1]
	}
}

func (e *SmartResumeExtractor) setDefaults(data *SmartExtractedData) {
	if data.Country == "" {
		data.Country = "United States"
	}
	if data.WorkAuthorization == "" {
		if data.YearsOfExperience > 0 {
			data.WorkAuthorization = "Authorized to work in US"
		} else {
			data.WorkAuthorization = "Will specify during interview"
		}
	}
	if data.StartDate == "" {
		data.StartDate = "Immediately"
	}
	if data.NoticePeriod == "" {
		if data.CurrentJobTitle != "" {
			data.NoticePeriod = "2 weeks"
		} else {
			data.NoticePeriod = "Immediately available"
		}
	}
	if data.SalaryExpectation == "" {
		data.SalaryExpectation = "Competitive/Negotiable"
	}
}

func (e *SmartResumeExtractor) calculateTotalExperience(experiences []ExtractedExperience) int {
	totalYears := 0
	for _, exp := range experiences {
		if exp.StartDate != "" {
			startYear, _ := strconv.Atoi(exp.StartDate)
			var endYear int
			if exp.CurrentlyWorking {
				endYear = time.Now().Year()
			} else if exp.EndDate != "" {
				endYear, _ = strconv.Atoi(exp.EndDate)
			} else {
				continue
			}
			years := endYear - startYear
			if years > 0 {
				totalYears += years
			}
		}
	}
	return totalYears
}

// Convert to form data map for automation
func (e *SmartResumeExtractor) ToFormDataMap(data *SmartExtractedData) map[string]interface{} {
	formData := make(map[string]interface{})
	
	// Personal
	formData["first_name"] = data.FirstName
	formData["last_name"] = data.LastName
	formData["full_name"] = data.FullName
	formData["email"] = data.Email
	formData["phone"] = data.Phone
	formData["phone_number"] = data.Phone
	
	// Location
	formData["city"] = data.City
	formData["state"] = data.State
	formData["country"] = data.Country
	
	// Professional
	formData["current_job_title"] = data.CurrentJobTitle
	formData["current_company"] = data.CurrentCompany
	formData["years_experience"] = strconv.Itoa(data.YearsOfExperience)
	formData["years_of_experience"] = strconv.Itoa(data.YearsOfExperience)
	
	// Education
	formData["degree"] = data.HighestDegree
	formData["university"] = data.School
	formData["school"] = data.School
	formData["field_of_study"] = data.FieldOfStudy
	formData["graduation_year"] = strconv.Itoa(data.GraduationYear)
	formData["gpa"] = data.GPA
	
	// Work Authorization
	formData["work_authorization"] = data.WorkAuthorization
	formData["authorized_to_work"] = data.WorkAuthorization
	
	// Availability
	formData["start_date"] = data.StartDate
	formData["available_start_date"] = data.StartDate
	formData["notice_period"] = data.NoticePeriod
	
	// Salary
	formData["salary_expectation"] = data.SalaryExpectation
	formData["expected_salary"] = data.SalaryExpectation
	
	// Skills
	if len(data.Skills) > 0 {
		formData["skills"] = strings.Join(data.Skills, ", ")
	}
	
	// Summary
	formData["summary"] = data.Summary
	formData["cover_letter"] = data.Summary
	
	// Social links
	formData["linkedin"] = data.LinkedIn
	formData["portfolio"] = data.Portfolio
	formData["github"] = data.GitHub
	
	return formData
}