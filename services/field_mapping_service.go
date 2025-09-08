package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// FieldMapping represents a mapping from question patterns to database fields
type FieldMapping struct {
	QuestionPatterns []string `json:"patterns"`    // Patterns to match in questions
	FieldName        string   `json:"field_name"`  // Database column name
	TableName        string   `json:"table_name"`  // "profile" or "resume"
	FieldType        string   `json:"field_type"`  // "text", "select", "radio", etc.
}

// QuestionMapper handles mapping form questions to database fields
type QuestionMapper struct {
	ProfileMappings []FieldMapping
	ResumeMappings  []FieldMapping
}

// NewQuestionMapper creates a new question mapper with predefined mappings
func NewQuestionMapper() *QuestionMapper {
	return &QuestionMapper{
		ProfileMappings: getProfileMappings(),
		ResumeMappings:  getResumeMappings(),
	}
}

// getProfileMappings returns all mappings for user_profiles table
func getProfileMappings() []FieldMapping {
	return []FieldMapping{
		// Basic Information
		{
			QuestionPatterns: []string{"first name", "given name", "firstname", "fname"},
			FieldName:        "first_name",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"last name", "surname", "family name", "lastname", "lname"},
			FieldName:        "last_name",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"full name", "your name", "name", "fullname"},
			FieldName:        "full_name",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"email", "e-mail", "email address", "e-mail address"},
			FieldName:        "email",
			TableName:        "user_profiles",
			FieldType:        "email",
		},
		{
			QuestionPatterns: []string{"phone", "telephone", "mobile", "cell", "phone number", "contact number"},
			FieldName:        "phone",
			TableName:        "user_profiles",
			FieldType:        "tel",
		},
		
		// Address Information
		{
			QuestionPatterns: []string{"address", "street address", "home address", "residential address"},
			FieldName:        "address",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"city", "town", "municipality"},
			FieldName:        "city",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"state", "province", "region"},
			FieldName:        "state",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"zip", "postal", "postcode", "zip code", "postal code"},
			FieldName:        "zip_code",
			TableName:        "user_profiles",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"country", "nation", "country of residence"},
			FieldName:        "country",
			TableName:        "user_profiles",
			FieldType:        "select",
		},
		
		// Professional Links
		{
			QuestionPatterns: []string{"linkedin", "linkedin profile", "linkedin url"},
			FieldName:        "linkedin",
			TableName:        "user_profiles",
			FieldType:        "url",
		},
		{
			QuestionPatterns: []string{"github", "github profile", "github url"},
			FieldName:        "github",
			TableName:        "user_profiles",
			FieldType:        "url",
		},
		{
			QuestionPatterns: []string{"portfolio", "website", "personal website", "personal site"},
			FieldName:        "portfolio",
			TableName:        "user_profiles",
			FieldType:        "url",
		},
		
		// Demographics (for job applications)
		{
			QuestionPatterns: []string{"gender", "sex", "gender identity"},
			FieldName:        "gender",
			TableName:        "user_profiles",
			FieldType:        "select",
		},
		{
			QuestionPatterns: []string{"ethnicity", "race", "ethnic background", "racial background"},
			FieldName:        "ethnicity",
			TableName:        "user_profiles",
			FieldType:        "select",
		},
		{
			QuestionPatterns: []string{"veteran", "military", "veteran status", "military service"},
			FieldName:        "veteran_status",
			TableName:        "user_profiles",
			FieldType:        "select",
		},
		{
			QuestionPatterns: []string{"disability", "disabled", "disability status"},
			FieldName:        "disability_status",
			TableName:        "user_profiles",
			FieldType:        "select",
		},
		
		// Work Authorization
		{
			QuestionPatterns: []string{
				"authorized to work", "work authorization", "legally authorized",
				"eligible to work", "work eligibility", "right to work",
			},
			FieldName:        "work_authorization",
			TableName:        "user_profiles",
			FieldType:        "radio",
		},
		{
			QuestionPatterns: []string{
				"require sponsorship", "visa sponsorship", "need sponsorship",
				"sponsorship required", "require visa",
			},
			FieldName:        "requires_sponsorship",
			TableName:        "user_profiles",
			FieldType:        "radio",
		},
	}
}

// getResumeMappings returns all mappings for resume_history table
func getResumeMappings() []FieldMapping {
	return []FieldMapping{
		// Education
		{
			QuestionPatterns: []string{
				"school", "university", "college", "institution",
				"where did you study", "where did you graduate",
				"education institution", "alma mater",
			},
			FieldName:        "school",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"degree", "qualification", "education level",
				"highest degree", "highest education", "degree type",
				"bachelor", "master", "phd", "doctorate",
			},
			FieldName:        "degree",
			TableName:        "resume_history",
			FieldType:        "select",
		},
		{
			QuestionPatterns: []string{
				"major", "field of study", "specialization",
				"concentration", "discipline", "subject",
			},
			FieldName:        "major",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{"gpa", "grade point average", "grades", "academic performance"},
			FieldName:        "gpa",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"graduation date", "graduation year", "when did you graduate",
				"completion date", "degree date",
			},
			FieldName:        "graduation_date",
			TableName:        "resume_history",
			FieldType:        "date",
		},
		
		// Experience
		{
			QuestionPatterns: []string{
				"current employer", "current company", "present employer",
				"where do you work", "current organization",
			},
			FieldName:        "current_company",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"current position", "current role", "current job title",
				"present position", "current title",
			},
			FieldName:        "current_position",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"years of experience", "experience years", "how many years",
				"total experience", "professional experience",
			},
			FieldName:        "years_of_experience",
			TableName:        "resume_history",
			FieldType:        "number",
		},
		
		// Skills
		{
			QuestionPatterns: []string{
				"skills", "technical skills", "competencies",
				"expertise", "proficiencies", "capabilities",
			},
			FieldName:        "skills",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"programming languages", "languages", "coding languages",
				"development languages", "tech stack",
			},
			FieldName:        "programming_languages",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		
		// Other
		{
			QuestionPatterns: []string{
				"summary", "about", "bio", "introduction",
				"professional summary", "profile summary",
			},
			FieldName:        "summary",
			TableName:        "resume_history",
			FieldType:        "textarea",
		},
		{
			QuestionPatterns: []string{
				"salary expectation", "expected salary", "salary requirement",
				"compensation expectation", "desired salary",
			},
			FieldName:        "salary_expectation",
			TableName:        "resume_history",
			FieldType:        "text",
		},
		{
			QuestionPatterns: []string{
				"notice period", "availability", "when can you start",
				"start date", "available from",
			},
			FieldName:        "notice_period",
			TableName:        "resume_history",
			FieldType:        "text",
		},
	}
}

// FindMapping finds the appropriate database field for a given question
func (qm *QuestionMapper) FindMapping(question string) (*FieldMapping, bool) {
	questionLower := strings.ToLower(question)
	
	// Check profile mappings first
	for _, mapping := range qm.ProfileMappings {
		for _, pattern := range mapping.QuestionPatterns {
			if strings.Contains(questionLower, pattern) {
				return &mapping, true
			}
		}
	}
	
	// Then check resume mappings
	for _, mapping := range qm.ResumeMappings {
		for _, pattern := range mapping.QuestionPatterns {
			if strings.Contains(questionLower, pattern) {
				return &mapping, true
			}
		}
	}
	
	return nil, false
}

// GetFieldValue retrieves the value for a mapped field from the database
func (qm *QuestionMapper) GetFieldValue(mapping *FieldMapping, userID int, db interface{}) (string, error) {
	// This will be implemented to query the actual database
	// For now, returning a placeholder
	return "", nil
}

// HandleUnmappedQuestion handles questions that don't have a direct mapping
func (qm *QuestionMapper) HandleUnmappedQuestion(question string, userID int, db interface{}) (string, error) {
	// This will check the extra_qa column for this specific question
	// The extra_qa column should store a JSON object with question-answer pairs
	return "", nil
}

// ExtractFieldsFromExtraQA extracts specific fields from the extra_qa JSON column
func ExtractFieldsFromExtraQA(extraQAJSON string, fieldName string) (string, error) {
	if extraQAJSON == "" {
		return "", nil
	}
	
	var extraQA map[string]interface{}
	if err := json.Unmarshal([]byte(extraQAJSON), &extraQA); err != nil {
		log.Printf("Error parsing extra_qa JSON: %v", err)
		return "", nil
	}
	
	// Try to find the field in various formats
	patterns := []string{
		fieldName,
		strings.ToLower(fieldName),
		strings.ReplaceAll(strings.ToLower(fieldName), "_", " "),
		strings.ReplaceAll(strings.ToLower(fieldName), "_", ""),
	}
	
	for _, pattern := range patterns {
		if value, exists := extraQA[pattern]; exists {
			switch v := value.(type) {
			case string:
				return v, nil
			case float64:
				return fmt.Sprintf("%.0f", v), nil
			default:
				return "", nil
			}
		}
	}
	
	return "", nil
}