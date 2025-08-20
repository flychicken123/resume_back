package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"resumeai/models"
)

type ResumeDataExtractor struct {
	userModel       *models.UserModel
	resumeModel     *models.ResumeModel
	experienceModel *models.ExperienceModel
	educationModel  *models.EducationModel
}

type ExtractedUserData struct {
	// Personal Information
	FullName    string `json:"full_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	
	// Professional Summary
	Summary     string `json:"summary"`
	Skills      []string `json:"skills"`
	
	// Experience Data
	CurrentJobTitle    string   `json:"current_job_title"`
	CurrentCompany     string   `json:"current_company"`
	YearsOfExperience  int      `json:"years_of_experience"`
	PreviousCompanies  []string `json:"previous_companies"`
	JobTitles          []string `json:"job_titles"`
	
	// Education Data
	HighestDegree      string `json:"highest_degree"`
	School             string `json:"school"`
	FieldOfStudy       string `json:"field_of_study"`
	GraduationYear     int    `json:"graduation_year"`
	GPA                string `json:"gpa"`
	
	// Location Information
	City               string `json:"city"`
	State              string `json:"state"`
	Country            string `json:"country"`
	
	// Work Authorization (inferred)
	WorkAuthorization  string `json:"work_authorization"`
	
	// Availability
	StartDate          string `json:"start_date"`
	NoticePeriod       string `json:"notice_period"`
	
	// Salary Expectations (can be set to flexible initially)
	SalaryExpectation  string `json:"salary_expectation"`
	
	// Other Common Fields
	LinkedIn           string `json:"linkedin"`
	Portfolio          string `json:"portfolio"`
	GitHub             string `json:"github"`
}

func NewResumeDataExtractor(
	userModel *models.UserModel,
	resumeModel *models.ResumeModel,
	experienceModel *models.ExperienceModel,
	educationModel *models.EducationModel,
) *ResumeDataExtractor {
	return &ResumeDataExtractor{
		userModel:       userModel,
		resumeModel:     resumeModel,
		experienceModel: experienceModel,
		educationModel:  educationModel,
	}
}

func (e *ResumeDataExtractor) ExtractUserData(userID, resumeID int) (*ExtractedUserData, error) {
	// Get user basic info
	user, err := e.userModel.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %v", err)
	}
	
	// Get resume data
	resume, err := e.resumeModel.GetByID(resumeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resume: %v", err)
	}
	
	// Get experience data
	experiences, err := e.experienceModel.GetByResumeID(resumeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get experiences: %v", err)
	}
	
	// Get education data
	educations, err := e.educationModel.GetByResumeID(resumeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get education: %v", err)
	}
	
	// Extract and process all data
	extractedData := &ExtractedUserData{}
	
	// Personal Information
	e.extractPersonalInfo(user, resume, extractedData)
	
	// Professional Summary and Skills
	e.extractProfessionalInfo(resume, extractedData)
	
	// Experience Information
	e.extractExperienceInfo(experiences, extractedData)
	
	// Education Information
	e.extractEducationInfo(educations, extractedData)
	
	// Set intelligent defaults
	e.setIntelligentDefaults(extractedData)
	
	return extractedData, nil
}

func (e *ResumeDataExtractor) extractPersonalInfo(user *models.User, resume *models.Resume, data *ExtractedUserData) {
	// Use resume name if available, fallback to user name
	fullName := resume.Name
	if fullName == "" {
		fullName = user.Name
	}
	
	data.FullName = fullName
	
	// Split name into first and last
	nameParts := strings.Fields(fullName)
	if len(nameParts) >= 1 {
		data.FirstName = nameParts[0]
	}
	if len(nameParts) >= 2 {
		data.LastName = nameParts[len(nameParts)-1]
	}
	
	// Email from resume or user
	data.Email = resume.Email
	if data.Email == "" {
		data.Email = user.Email
	}
	
	// Phone from resume
	data.Phone = resume.Phone
}

func (e *ResumeDataExtractor) extractProfessionalInfo(resume *models.Resume, data *ExtractedUserData) {
	// Extract summary
	if resume.Summary != nil {
		var summaryObj map[string]interface{}
		if err := json.Unmarshal(resume.Summary, &summaryObj); err == nil {
			if summary, ok := summaryObj["text"].(string); ok {
				data.Summary = summary
			}
		} else {
			// If it's just a string, use it directly
			data.Summary = string(resume.Summary)
		}
	}
	
	// Extract skills
	if resume.Skills != nil {
		var skillsObj interface{}
		if err := json.Unmarshal(resume.Skills, &skillsObj); err == nil {
			switch skills := skillsObj.(type) {
			case []interface{}:
				for _, skill := range skills {
					if skillStr, ok := skill.(string); ok {
						data.Skills = append(data.Skills, skillStr)
					}
				}
			case map[string]interface{}:
				if skillsList, ok := skills["skills"].([]interface{}); ok {
					for _, skill := range skillsList {
						if skillStr, ok := skill.(string); ok {
							data.Skills = append(data.Skills, skillStr)
						}
					}
				}
			case string:
				// If it's a comma-separated string
				skillsList := strings.Split(skills, ",")
				for _, skill := range skillsList {
					data.Skills = append(data.Skills, strings.TrimSpace(skill))
				}
			}
		}
	}
}

func (e *ResumeDataExtractor) extractExperienceInfo(experiences []models.Experience, data *ExtractedUserData) {
	if len(experiences) == 0 {
		return
	}
	
	// Current job (most recent or currently working)
	var currentJob *models.Experience
	for i := range experiences {
		if experiences[i].CurrentlyWorking {
			currentJob = &experiences[i]
			break
		}
	}
	
	// If no current job, use the most recent one
	if currentJob == nil && len(experiences) > 0 {
		currentJob = &experiences[0]
	}
	
	if currentJob != nil {
		data.CurrentJobTitle = currentJob.JobTitle
		data.CurrentCompany = currentJob.Company
		data.City = currentJob.City
		data.State = currentJob.State
	}
	
	// Calculate years of experience
	data.YearsOfExperience = e.calculateYearsOfExperience(experiences)
	
	// Extract all companies and job titles
	companyMap := make(map[string]bool)
	titleMap := make(map[string]bool)
	
	for _, exp := range experiences {
		if exp.Company != "" {
			companyMap[exp.Company] = true
		}
		if exp.JobTitle != "" {
			titleMap[exp.JobTitle] = true
		}
	}
	
	for company := range companyMap {
		data.PreviousCompanies = append(data.PreviousCompanies, company)
	}
	
	for title := range titleMap {
		data.JobTitles = append(data.JobTitles, title)
	}
}

func (e *ResumeDataExtractor) extractEducationInfo(educations []models.Education, data *ExtractedUserData) {
	if len(educations) == 0 {
		return
	}
	
	// Get highest/most recent education
	mostRecent := educations[0]
	for _, edu := range educations {
		if edu.GraduationYear > mostRecent.GraduationYear {
			mostRecent = edu
		}
	}
	
	data.HighestDegree = mostRecent.Degree
	data.School = mostRecent.School
	data.FieldOfStudy = mostRecent.Field
	data.GraduationYear = mostRecent.GraduationYear
	data.GPA = mostRecent.GPA
	
	// Use education location if work location not available
	if data.City == "" && data.State == "" && mostRecent.Location != "" {
		locationParts := strings.Split(mostRecent.Location, ",")
		if len(locationParts) >= 1 {
			data.City = strings.TrimSpace(locationParts[0])
		}
		if len(locationParts) >= 2 {
			data.State = strings.TrimSpace(locationParts[1])
		}
	}
}

func (e *ResumeDataExtractor) setIntelligentDefaults(data *ExtractedUserData) {
	// Set country default
	if data.Country == "" {
		data.Country = "United States"
	}
	
	// Set work authorization based on experience
	if data.WorkAuthorization == "" {
		if data.YearsOfExperience > 0 {
			data.WorkAuthorization = "Authorized to work in US"
		} else {
			data.WorkAuthorization = "Will specify during interview"
		}
	}
	
	// Set start date
	if data.StartDate == "" {
		data.StartDate = "Immediately"
	}
	
	// Set notice period based on current employment
	if data.NoticePeriod == "" {
		if data.CurrentJobTitle != "" {
			data.NoticePeriod = "2 weeks"
		} else {
			data.NoticePeriod = "Immediately available"
		}
	}
	
	// Set flexible salary expectation
	if data.SalaryExpectation == "" {
		data.SalaryExpectation = "Competitive/Negotiable"
	}
}

func (e *ResumeDataExtractor) calculateYearsOfExperience(experiences []models.Experience) int {
	totalMonths := 0
	
	for _, exp := range experiences {
		months := e.calculateMonthsBetweenDates(exp.StartDate, exp.EndDate, exp.CurrentlyWorking)
		totalMonths += months
	}
	
	years := totalMonths / 12
	if totalMonths%12 >= 6 {
		years++ // Round up if 6+ months
	}
	
	return years
}

func (e *ResumeDataExtractor) calculateMonthsBetweenDates(startDate, endDate string, currentlyWorking bool) int {
	if startDate == "" {
		return 0
	}
	
	// Parse start date
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return 0
	}
	
	var endTime time.Time
	if currentlyWorking || endDate == "" {
		endTime = time.Now()
	} else {
		endTime, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			endTime = time.Now()
		}
	}
	
	// Calculate months
	years := endTime.Year() - startTime.Year()
	months := int(endTime.Month()) - int(startTime.Month())
	
	return years*12 + months
}

// Convert extracted data to map for form filling
func (e *ResumeDataExtractor) ToFormDataMap(data *ExtractedUserData) map[string]interface{} {
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
	
	return formData
}