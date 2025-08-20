package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// UserDialogService handles interactive user prompts during form filling
type UserDialogService struct {
	page playwright.Page
}

// NewUserDialogService creates a new dialog service
func NewUserDialogService(page playwright.Page) *UserDialogService {
	return &UserDialogService{
		page: page,
	}
}

// AskUserForInput shows a JavaScript prompt to get user input for a missing field
func (s *UserDialogService) AskUserForInput(fieldName, question string, options []string) (string, error) {
	log.Printf("Asking user for input: %s", question)
	
	// Add a small delay to ensure the page is ready
	time.Sleep(500 * time.Millisecond)
	
	// For testing - use a simple browser prompt
	if len(options) > 0 {
		promptText := fmt.Sprintf("%s\n\nOptions:\n%s\n\nPlease enter one of the above options:", 
			question, strings.Join(options, "\n"))
		
		log.Printf("Showing browser prompt with text: %s", promptText)
		jsCode := fmt.Sprintf(`prompt(%q, %q)`, promptText, options[0])
		result, err := s.page.Evaluate(jsCode)
		if err != nil {
			return "", fmt.Errorf("failed to show prompt: %w", err)
		}
		
		if result == nil {
			return "", nil
		}
		
		response, ok := result.(string)
		if !ok {
			return "", fmt.Errorf("unexpected response type from prompt")
		}
		
		log.Printf("User provided answer via prompt: %s", response)
		return response, nil
	}
	
	// Fallback to regular prompt for text input
	jsCode := fmt.Sprintf(`prompt(%q)`, question)
	result, err := s.page.Evaluate(jsCode)
	if err != nil {
		return "", fmt.Errorf("failed to show prompt: %w", err)
	}
	
	if result == nil {
		return "", nil
	}
	
	response, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type from prompt")
	}
	
	log.Printf("User provided answer via prompt: %s", response)
	return response, nil
}

// AskUserForInputCustomDialog shows a custom dialog (original implementation)
func (s *UserDialogService) AskUserForInputCustomDialog(fieldName, question string, options []string) (string, error) {
	log.Printf("Asking user for input with custom dialog: %s", question)
	
	// Take a screenshot before showing the popup
	s.page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(fmt.Sprintf("/tmp/before_popup_%s.png", strings.ReplaceAll(fieldName, " ", "_"))),
	})
	
	// Escape the question for JavaScript
	questionJSON, _ := json.Marshal(question)
	optionsJSON, _ := json.Marshal(options)
	
	// Create a custom dialog using JavaScript
	// Show popup on the main page
	jsCode := fmt.Sprintf(`
		(async function() {
			console.log('Creating popup dialog for user input...');
			console.log('Question:', %s);
			console.log('Options:', %s);
			return new Promise((resolve) => {
				// Create overlay
				const overlay = document.createElement('div');
				overlay.style.cssText = 'position: fixed; top: 0; left: 0; width: 100%%; height: 100%%; background: rgba(0,0,0,0.5); z-index: 999999; display: flex; align-items: center; justify-content: center;';
				
				// Create dialog
				const dialog = document.createElement('div');
				dialog.style.cssText = 'background: white; padding: 20px; border-radius: 8px; max-width: 500px; width: 90%%; max-height: 80vh; overflow-y: auto; box-shadow: 0 4px 6px rgba(0,0,0,0.1);';
				
				// Add title
				const title = document.createElement('h3');
				title.textContent = 'Information Required';
				title.style.cssText = 'margin: 0 0 10px 0; color: #333;';
				dialog.appendChild(title);
				
				// Add question
				const questionDiv = document.createElement('div');
				questionDiv.textContent = %s;
				questionDiv.style.cssText = 'margin-bottom: 15px; color: #555; line-height: 1.5;';
				dialog.appendChild(questionDiv);
				
				// Add input based on options
				let inputElement;
				const hasOptions = %s;
				
				if (hasOptions.length > 0) {
					// Create dropdown for options
					inputElement = document.createElement('select');
					inputElement.style.cssText = 'width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px;';
					
					// Add empty option
					const emptyOption = document.createElement('option');
					emptyOption.value = '';
					emptyOption.textContent = '-- Please select --';
					inputElement.appendChild(emptyOption);
					
					// Add all options
					hasOptions.forEach(opt => {
						const option = document.createElement('option');
						option.value = opt;
						option.textContent = opt;
						inputElement.appendChild(option);
					});
				} else {
					// Create text input
					inputElement = document.createElement('input');
					inputElement.type = 'text';
					inputElement.style.cssText = 'width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px;';
					inputElement.placeholder = 'Enter your answer';
				}
				
				dialog.appendChild(inputElement);
				
				// Add buttons
				const buttonContainer = document.createElement('div');
				buttonContainer.style.cssText = 'margin-top: 20px; display: flex; gap: 10px; justify-content: flex-end;';
				
				const cancelButton = document.createElement('button');
				cancelButton.textContent = 'Skip';
				cancelButton.style.cssText = 'padding: 8px 16px; background: #f0f0f0; border: none; border-radius: 4px; cursor: pointer;';
				cancelButton.onclick = () => {
					document.body.removeChild(overlay);
					resolve('');
				};
				
				const submitButton = document.createElement('button');
				submitButton.textContent = 'Submit';
				submitButton.style.cssText = 'padding: 8px 16px; background: #4CAF50; color: white; border: none; border-radius: 4px; cursor: pointer;';
				submitButton.onclick = () => {
					const value = inputElement.value;
					document.body.removeChild(overlay);
					resolve(value);
				};
				
				buttonContainer.appendChild(cancelButton);
				buttonContainer.appendChild(submitButton);
				dialog.appendChild(buttonContainer);
				
				// Add dialog to overlay
				overlay.appendChild(dialog);
				document.body.appendChild(overlay);
				
				// Focus the input
				inputElement.focus();
				
				// Handle Enter key
				inputElement.addEventListener('keypress', (e) => {
					if (e.key === 'Enter') {
						submitButton.click();
					}
				});
			});
		})();
	`, string(questionJSON), string(optionsJSON))
	
	// Execute the JavaScript and wait for user response
	result, err := s.page.Evaluate(jsCode)
	if err != nil {
		return "", fmt.Errorf("failed to show user dialog: %w", err)
	}
	
	// Take a screenshot after popup is shown
	s.page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(fmt.Sprintf("/tmp/after_popup_%s.png", strings.ReplaceAll(fieldName, " ", "_"))),
	})
	
	response, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type from dialog")
	}
	
	log.Printf("User provided answer: %s", response)
	return response, nil
}

// AskUserForMissingFields prompts the user for all missing required fields
func (s *UserDialogService) AskUserForMissingFields(missingFields []MissingFieldInfo) (map[string]string, error) {
	responses := make(map[string]string)
	
	// First, show an overview dialog
	overviewMessage := fmt.Sprintf("This application requires %d additional fields. You will be prompted for each one.", len(missingFields))
	s.showInfoDialog("Additional Information Required", overviewMessage)
	
	// Ask for each field
	for _, field := range missingFields {
		// Skip optional fields if user doesn't want to provide them
		if !field.Required {
			// Ask if they want to provide this optional field
			wantToProvide, err := s.AskUserForInput(
				field.FieldName,
				fmt.Sprintf("(Optional) %s\n\nWould you like to provide this information?", field.Question),
				[]string{"Yes", "No", "Prefer not to answer"},
			)
			if err != nil || wantToProvide != "Yes" {
				if wantToProvide == "Prefer not to answer" {
					responses[field.FieldName] = "Prefer not to answer"
				}
				continue
			}
		}
		
		// Get the actual answer
		answer, err := s.AskUserForInput(field.FieldName, field.Question, field.Options)
		if err != nil {
			return nil, fmt.Errorf("failed to get user input for %s: %w", field.FieldName, err)
		}
		
		if answer == "" && field.Required {
			return nil, fmt.Errorf("required field %s was not provided", field.FieldName)
		}
		
		if answer != "" {
			responses[field.FieldName] = answer
		}
	}
	
	return responses, nil
}

// showInfoDialog shows an informational dialog to the user
func (s *UserDialogService) showInfoDialog(title, message string) error {
	jsCode := `
		(() => {
			const overlay = document.createElement('div');
			overlay.style.cssText = 'position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.5); z-index: 999999; display: flex; align-items: center; justify-content: center;';
			
			const dialog = document.createElement('div');
			dialog.style.cssText = 'background: white; padding: 20px; border-radius: 8px; max-width: 500px; width: 90%; box-shadow: 0 4px 6px rgba(0,0,0,0.1);';
			
			const titleEl = document.createElement('h3');
			titleEl.textContent = %q;
			titleEl.style.cssText = 'margin: 0 0 10px 0; color: #333;';
			dialog.appendChild(titleEl);
			
			const messageEl = document.createElement('div');
			messageEl.textContent = %q;
			messageEl.style.cssText = 'margin-bottom: 15px; color: #555; line-height: 1.5;';
			dialog.appendChild(messageEl);
			
			const button = document.createElement('button');
			button.textContent = 'Continue';
			button.style.cssText = 'padding: 8px 16px; background: #4CAF50; color: white; border: none; border-radius: 4px; cursor: pointer;';
			button.onclick = () => document.body.removeChild(overlay);
			dialog.appendChild(button);
			
			overlay.appendChild(dialog);
			document.body.appendChild(overlay);
			button.focus();
		})()
	`
	
	jsCode = fmt.Sprintf(jsCode, title, message)
	_, err := s.page.Evaluate(jsCode)
	
	// Give user time to read
	time.Sleep(2 * time.Second)
	
	return err
}

// CollectMissingFieldsFromDropdownError parses the dropdown error and collects missing fields
func CollectMissingFieldsFromDropdownError(errorMessage string) []MissingFieldInfo {
	var fields []MissingFieldInfo
	
	// Parse the error message to extract field names
	if strings.Contains(errorMessage, "Unable to fill") {
		// Extract the fields from the error message
		parts := strings.SplitN(errorMessage, ": ", 2)
		if len(parts) > 1 {
			fieldList := strings.Split(parts[1], " | ")
			for _, fieldName := range fieldList {
				fieldName = strings.TrimSpace(fieldName)
				if fieldName != "" {
					fields = append(fields, createMissingFieldInfo(fieldName))
				}
			}
		}
	}
	
	return fields
}

// createMissingFieldInfo creates a MissingFieldInfo from a field name
func createMissingFieldInfo(fieldName string) MissingFieldInfo {
	fieldLower := strings.ToLower(fieldName)
	
	info := MissingFieldInfo{
		FieldName: fieldName,
		Question:  fieldName,
		Required:  strings.Contains(fieldName, "*"),
		FieldType: "dropdown",
	}
	
	// Determine appropriate options based on field name
	switch {
	case strings.Contains(fieldLower, "remote"):
		info.Question = "Do you plan to work remotely if this role offers that option?"
		info.Options = []string{"Yes, I intend to work remotely", "No, I prefer to work in office", "Hybrid"}
		
	case strings.Contains(fieldLower, "previously employed") || strings.Contains(fieldLower, "ever been employed"):
		info.Question = "Have you ever been employed by this company or its affiliates?"
		info.Options = []string{"Yes", "No"}
		
	case strings.Contains(fieldLower, "whatsapp"):
		info.Question = "Do you opt-in to receive WhatsApp messages from the recruiter?"
		info.Options = []string{"Yes", "No"}
		
	case strings.Contains(fieldLower, "sexual orientation"):
		info.Question = "How would you describe your sexual orientation?"
		info.Options = []string{"Heterosexual", "Gay", "Lesbian", "Bisexual and/or pansexual", "Asexual", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "transgender"):
		info.Question = "Do you identify as transgender?"
		info.Options = []string{"Yes", "No", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "disability"):
		info.Question = "Do you have a disability or chronic condition?"
		info.Options = []string{"Yes", "No", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "veteran"):
		info.Question = "Are you a veteran or active member of the Armed Forces?"
		info.Options = []string{"Yes", "No", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "sponsor") || strings.Contains(fieldLower, "work permit") || strings.Contains(fieldLower, "sponsorship"):
		info.Question = "Will you require sponsorship for a work permit?"
		info.Options = []string{"Yes", "No"}
		
	case strings.Contains(fieldLower, "legally authorized") || strings.Contains(fieldLower, "authorized to work"):
		info.Question = "Are you legally authorized to work in this country?"
		info.Options = []string{"Yes", "No"}
		
	case strings.Contains(fieldLower, "willing to relocate") || strings.Contains(fieldLower, "relocation"):
		info.Question = "Are you willing to relocate for this position?"
		info.Options = []string{"Yes", "No", "Maybe"}
		
	case strings.Contains(fieldLower, "travel") && strings.Contains(fieldLower, "willing"):
		info.Question = "Are you willing to travel for this position?"
		info.Options = []string{"Yes", "No", "Occasionally"}
		
	case strings.Contains(fieldLower, "clearance") || strings.Contains(fieldLower, "security clearance"):
		info.Question = "Do you have security clearance?"
		info.Options = []string{"Yes", "No", "In process"}
		
	case strings.Contains(fieldLower, "citizen"):
		info.Question = "Are you a citizen of this country?"
		info.Options = []string{"Yes", "No"}
		
	case strings.Contains(fieldLower, "country") && (strings.Contains(fieldLower, "reside") || strings.Contains(fieldLower, "currently")):
		info.Question = "Please select the country where you currently reside"
		info.FieldType = "dropdown"
		// Common countries list - this should ideally be extracted from the job site
		info.Options = []string{"United States", "Canada", "United Kingdom", "Australia", "Germany", "France", "India", "China", "Japan", "Brazil", "Mexico", "Other"}
		
	case strings.Contains(fieldLower, "gender"):
		info.Question = "What is your gender?"
		info.Options = []string{"Male", "Female", "Non-binary", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "race") || strings.Contains(fieldLower, "ethnicity"):
		info.Question = "What is your race/ethnicity?"
		info.Options = []string{"White", "Black or African American", "Hispanic or Latino", "Asian", "Native American", "Pacific Islander", "Two or more races", "Prefer not to answer"}
		info.Required = false
		
	case strings.Contains(fieldLower, "hear about") || strings.Contains(fieldLower, "how did you hear"):
		info.Question = "How did you hear about this position?"
		info.Options = []string{"Company Website", "LinkedIn", "Indeed", "Referral", "Recruiter", "Job Board", "Other"}
		
	case strings.Contains(fieldLower, "salary") || strings.Contains(fieldLower, "compensation"):
		info.Question = fieldName
		info.FieldType = "text"  // Salary should be text input for flexibility
		info.Options = nil
		
	case strings.Contains(fieldLower, "start date") || strings.Contains(fieldLower, "available to start"):
		info.Question = "When are you available to start?"
		info.FieldType = "text"  // Date should be text input
		info.Options = nil
		
	case strings.Contains(fieldLower, "years of experience") || strings.Contains(fieldLower, "experience"):
		info.Question = fieldName
		info.FieldType = "text"  // Years of experience should be text/number input
		info.Options = nil
		
	default:
		// For unknown fields, check if they look like yes/no questions
		if strings.Contains(fieldLower, "do you") || strings.Contains(fieldLower, "are you") || 
		   strings.Contains(fieldLower, "have you") || strings.Contains(fieldLower, "will you") ||
		   strings.Contains(fieldLower, "can you") || strings.Contains(fieldLower, "would you") {
			info.Options = []string{"Yes", "No"}
		} else {
			// Default to text input for truly unknown fields
			info.FieldType = "text"
			info.Options = nil
		}
	}
	
	return info
}