package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MissingFieldsError represents an error with missing required fields
type MissingFieldsError struct {
	Fields []MissingFieldInfo `json:"fields"`
	Message string `json:"message"`
}

func (e *MissingFieldsError) Error() string {
	return e.Message
}

// CheckRequiredFieldsAndPrompt checks if all required fields are filled and returns an error if not
func CheckRequiredFieldsAndPrompt(userData *UserProfileData, missingFieldNames []string) error {
	var missingFields []MissingFieldInfo
	
	for _, fieldName := range missingFieldNames {
		fieldLower := strings.ToLower(fieldName)
		
		// Check if we already have this data
		hasData := false
		
		switch {
		case strings.Contains(fieldLower, "remote"):
			hasData = userData.RemoteWorkPreference != ""
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "remote_work_preference",
					Question:  "Do you plan to work remotely if this role offers that option?",
					FieldType: "dropdown",
					Options:   []string{"Yes, I intend to work remotely", "No, I prefer to work in office", "Hybrid"},
					Required:  true,
				})
			}
			
		case strings.Contains(fieldLower, "previously employed") || strings.Contains(fieldLower, "ever been employed"):
			if userData.ExtraQA != nil {
				_, hasData = userData.ExtraQA["previously_employed"]
			}
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "previously_employed",
					Question:  "Have you ever been employed by Stripe or a Stripe affiliate?",
					FieldType: "dropdown",
					Options:   []string{"Yes", "No"},
					Required:  true,
				})
			}
			
		case strings.Contains(fieldLower, "whatsapp"):
			if userData.ExtraQA != nil {
				_, hasData = userData.ExtraQA["whatsapp_opt_in"]
			}
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "whatsapp_opt_in",
					Question:  "Do you opt-in to receive WhatsApp messages from Stripe Recruiting?",
					FieldType: "dropdown",
					Options:   []string{"Yes", "No"},
					Required:  true,
				})
			}
			
		case strings.Contains(fieldLower, "sexual orientation"):
			hasData = userData.SexualOrientation != ""
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "sexual_orientation",
					Question:  "How would you describe your sexual orientation? (optional)",
					FieldType: "dropdown",
					Options:   []string{"Heterosexual", "Gay", "Lesbian", "Bisexual and/or pansexual", "Asexual", "Queer", "Prefer to self-describe", "Prefer not to answer"},
					Required:  false,
				})
			}
			
		case strings.Contains(fieldLower, "transgender"):
			hasData = userData.TransgenderStatus != ""
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "transgender_status",
					Question:  "Do you identify as transgender? (optional)",
					FieldType: "dropdown",
					Options:   []string{"Yes", "No", "Prefer not to answer"},
					Required:  false,
				})
			}
			
		case strings.Contains(fieldLower, "disability"):
			hasData = userData.DisabilityStatus != ""
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "disability_status",
					Question:  "Do you have a disability or chronic condition? (optional)",
					FieldType: "dropdown",
					Options:   []string{"Yes", "No", "Prefer not to answer"},
					Required:  false,
				})
			}
			
		case strings.Contains(fieldLower, "veteran"):
			hasData = userData.VeteranStatus != ""
			if !hasData {
				missingFields = append(missingFields, MissingFieldInfo{
					FieldName: "veteran_status",
					Question:  "Are you a veteran or active member of the United States Armed Forces? (optional)",
					FieldType: "dropdown",
					Options:   []string{"Yes, I am a veteran or active member", "No, I am not a veteran or active member", "Prefer not to answer"},
					Required:  false,
				})
			}
		}
	}
	
	if len(missingFields) > 0 {
		// Create detailed error message
		var requiredCount, optionalCount int
		for _, field := range missingFields {
			if field.Required {
				requiredCount++
			} else {
				optionalCount++
			}
		}
		
		message := fmt.Sprintf("Application requires %d additional fields (%d required, %d optional). Please update your job profile with the following information:\n\n",
			len(missingFields), requiredCount, optionalCount)
		
		for i, field := range missingFields {
			requiredText := ""
			if field.Required {
				requiredText = " *REQUIRED*"
			}
			message += fmt.Sprintf("%d. %s%s\n", i+1, field.Question, requiredText)
			if len(field.Options) > 0 {
				message += fmt.Sprintf("   Options: %s\n", strings.Join(field.Options, ", "))
			}
			message += "\n"
		}
		
		message += "To provide this information:\n"
		message += "1. Update your job profile with the missing information\n"
		message += "2. For optional fields, you can select 'Prefer not to answer' if you don't wish to disclose\n"
		message += "3. Retry the application after updating your profile\n"
		
		return &MissingFieldsError{
			Fields: missingFields,
			Message: message,
		}
	}
	
	return nil
}

// CreateUserResponsesJSON creates a JSON structure for the user to fill out
func CreateUserResponsesJSON(missingFields []MissingFieldInfo) (string, error) {
	responses := make(map[string]interface{})
	
	for _, field := range missingFields {
		fieldData := map[string]interface{}{
			"question": field.Question,
			"required": field.Required,
			"value":    "", // User needs to fill this
		}
		
		if len(field.Options) > 0 {
			fieldData["options"] = field.Options
		}
		
		responses[field.FieldName] = fieldData
	}
	
	jsonBytes, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}