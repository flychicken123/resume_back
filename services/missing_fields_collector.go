package services

import (
	"fmt"
	"log"
	"strings"
)

// MissingFieldInfo represents a field that needs user input
type MissingFieldInfo struct {
	FieldName    string `json:"field_name"`
	Question     string `json:"question"`
	FieldType    string `json:"field_type"` // text, dropdown, checkbox
	Options      []string `json:"options,omitempty"` // For dropdowns
	Required     bool   `json:"required"`
}

// CollectMissingFields analyzes the form and returns fields that need user input
func CollectMissingFields(missingFields []string, dropdownOptions map[string][]string) []MissingFieldInfo {
	var result []MissingFieldInfo
	
	for _, field := range missingFields {
		fieldLower := strings.ToLower(field)
		info := MissingFieldInfo{
			FieldName: field,
			Question:  field,
			Required:  true,
		}
		
		// Determine field type and better question phrasing
		switch {
		case strings.Contains(fieldLower, "sexual orientation"):
			info.Question = "What is your sexual orientation? (This information is for demographic purposes only)"
			info.FieldType = "dropdown"
			info.Options = []string{"Heterosexual or straight", "Gay or lesbian", "Bisexual", "Prefer not to answer"}
			
		case strings.Contains(fieldLower, "transgender"):
			info.Question = "Do you identify as transgender?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes", "No", "Prefer not to answer"}
			
		case strings.Contains(fieldLower, "whatsapp"):
			info.Question = "Do you opt-in to receive WhatsApp messages from the recruiter?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes", "No"}
			
		case strings.Contains(fieldLower, "previously employed") || strings.Contains(fieldLower, "ever been employed"):
			info.Question = "Have you ever been employed by this company or its affiliates?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes", "No"}
			
		case strings.Contains(fieldLower, "remote"):
			info.Question = "Do you plan to work remotely if this role offers that option?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes, I intend to work remotely", "No, I prefer to work in office", "Hybrid"}
			
		case strings.Contains(fieldLower, "authorized") && strings.Contains(fieldLower, "work"):
			info.Question = "Are you legally authorized to work in the country where this position is located?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes", "No"}
			
		case strings.Contains(fieldLower, "sponsor"):
			info.Question = "Will you require sponsorship for a work permit/visa now or in the future?"
			info.FieldType = "dropdown"
			info.Options = []string{"Yes", "No"}
			
		default:
			// For unknown fields, check if we have dropdown options
			if options, ok := dropdownOptions[field]; ok && len(options) > 0 {
				info.FieldType = "dropdown"
				info.Options = options
			} else {
				info.FieldType = "text"
			}
		}
		
		result = append(result, info)
	}
	
	return result
}

// PromptUserForMissingFields returns an error with details about missing fields
func PromptUserForMissingFields(missingFields []MissingFieldInfo) error {
	if len(missingFields) == 0 {
		return nil
	}
	
	var messages []string
	messages = append(messages, fmt.Sprintf("The application form requires %d additional fields that need your input:", len(missingFields)))
	messages = append(messages, "")
	
	for i, field := range missingFields {
		msg := fmt.Sprintf("%d. %s", i+1, field.Question)
		if field.FieldType == "dropdown" && len(field.Options) > 0 {
			msg += fmt.Sprintf("\n   Options: %s", strings.Join(field.Options, ", "))
		}
		messages = append(messages, msg)
	}
	
	messages = append(messages, "")
	messages = append(messages, "Please provide this information in your user profile or job preferences before attempting to submit applications.")
	
	log.Printf("Missing required information:\n%s", strings.Join(messages, "\n"))
	
	return fmt.Errorf("missing required fields: %d fields need user input", len(missingFields))
}

// UpdateUserDataWithResponses updates the UserProfileData with user-provided responses
func UpdateUserDataWithResponses(userData *UserProfileData, responses map[string]string) {
	for field, value := range responses {
		fieldLower := strings.ToLower(field)
		
		switch {
		case strings.Contains(fieldLower, "sexual orientation"):
			userData.SexualOrientation = value
			
		case strings.Contains(fieldLower, "transgender"):
			userData.TransgenderStatus = value
			
		case strings.Contains(fieldLower, "whatsapp"):
			// Store in a generic field or preferences
			if userData.ExtraQA == nil {
				userData.ExtraQA = make(map[string]string)
			}
			userData.ExtraQA["whatsapp_opt_in"] = value
			
		case strings.Contains(fieldLower, "previously employed") || strings.Contains(fieldLower, "ever been employed"):
			if userData.ExtraQA == nil {
				userData.ExtraQA = make(map[string]string)
			}
			userData.ExtraQA["previously_employed"] = value
			
		case strings.Contains(fieldLower, "remote"):
			userData.RemoteWorkPreference = value
			
		case strings.Contains(fieldLower, "authorized") && strings.Contains(fieldLower, "work"):
			userData.WorkAuthorization = strings.ToLower(value)
			
		case strings.Contains(fieldLower, "sponsor"):
			userData.RequiresSponsorship = strings.ToLower(value) == "yes"
			
		default:
			// Store in ExtraQA for unknown fields
			if userData.ExtraQA == nil {
				userData.ExtraQA = make(map[string]string)
			}
			userData.ExtraQA[field] = value
		}
	}
}