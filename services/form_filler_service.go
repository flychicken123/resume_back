package services

import (
	"fmt"
	"log"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// FormFillerService handles form field detection and filling
type FormFillerService struct{}

// FillBasicFields fills standard form fields like name, email, phone
func (s *FormFillerService) FillBasicFields(page playwright.Page, userData *UserProfileData) int {
	filledCount := 0
	
	// Name fields
	filledCount += s.fillNameFields(page, userData)
	
	// Email fields
	filledCount += s.fillEmailFields(page, userData)
	
	// Phone fields  
	filledCount += s.fillPhoneFields(page, userData)
	
	// LinkedIn fields
	filledCount += s.fillLinkedInFields(page, userData)
	
	return filledCount
}

func (s *FormFillerService) fillNameFields(page playwright.Page, userData *UserProfileData) int {
	count := 0
	
	// First name selectors
	firstNameSelectors := []string{
		"input[name='first_name']",
		"input[name='firstName']",
		"input[id='first_name']",
		"input[placeholder*='First']",
		"input[aria-label*='First Name']",
		"label:has-text('First Name') + input",
	}
	
	for _, selector := range firstNameSelectors {
		if filled := s.tryFillField(page, selector, userData.FirstName); filled {
			count++
			break
		}
	}
	
	// Last name selectors
	lastNameSelectors := []string{
		"input[name='last_name']",
		"input[name='lastName']",
		"input[id='last_name']",
		"input[placeholder*='Last']",
		"input[aria-label*='Last Name']",
		"label:has-text('Last Name') + input",
	}
	
	for _, selector := range lastNameSelectors {
		if filled := s.tryFillField(page, selector, userData.LastName); filled {
			count++
			break
		}
	}
	
	// Full name selectors
	fullNameSelectors := []string{
		"input[name='name']",
		"input[name='full_name']",
		"input[name='fullName']",
		"input[placeholder*='Full Name']",
		"input[placeholder*='Your Name']",
		"input[aria-label*='Name']",
	}
	
	for _, selector := range fullNameSelectors {
		if filled := s.tryFillField(page, selector, userData.FullName); filled {
			count++
			break
		}
	}
	
	return count
}

func (s *FormFillerService) fillEmailFields(page playwright.Page, userData *UserProfileData) int {
	emailSelectors := []string{
		"input[type='email']",
		"input[name='email']",
		"input[name='email_address']",
		"input[id='email']",
		"input[placeholder*='Email']",
		"input[placeholder*='@']",
		"input[aria-label*='Email']",
		"label:has-text('Email') + input",
	}
	
	for _, selector := range emailSelectors {
		if filled := s.tryFillField(page, selector, userData.Email); filled {
			return 1
		}
	}
	
	return 0
}

func (s *FormFillerService) fillPhoneFields(page playwright.Page, userData *UserProfileData) int {
	phoneSelectors := []string{
		"input[type='tel']",
		"input[name='phone']",
		"input[name='phone_number']",
		"input[id='phone']",
		"input[placeholder*='Phone']",
		"input[aria-label*='Phone']",
		"label:has-text('Phone') + input",
	}
	
	for _, selector := range phoneSelectors {
		if filled := s.tryFillField(page, selector, userData.Phone); filled {
			return 1
		}
	}
	
	return 0
}

func (s *FormFillerService) fillLinkedInFields(page playwright.Page, userData *UserProfileData) int {
	linkedInSelectors := []string{
		"input[name='linkedin']",
		"input[name*='linkedin']",
		"input[placeholder*='LinkedIn']",
		"input[aria-label*='LinkedIn']",
		"label:has-text('LinkedIn') + input",
	}
	
	for _, selector := range linkedInSelectors {
		if filled := s.tryFillField(page, selector, userData.LinkedIn); filled {
			return 1
		}
	}
	
	return 0
}

func (s *FormFillerService) tryFillField(page playwright.Page, selector string, value string) bool {
	if value == "" {
		return false
	}
	
	element := page.Locator(selector).First()
	if visible, _ := element.IsVisible(); visible {
		// Clear and fill
		_ = element.Clear()
		if err := element.Fill(value); err == nil {
			log.Printf("✓ Filled field with selector '%s'", selector)
			return true
		}
	}
	
	return false
}

// CheckRequiredFields checks for any required fields that are still empty
func (s *FormFillerService) CheckRequiredFields(page playwright.Page) ([]string, map[string]string) {
	missingFields := []string{}
	fieldDescriptions := make(map[string]string)
	
	// Check required inputs
	requiredInputs, _ := page.Locator("input[required]:visible, input[aria-required='true']:visible").All()
	for _, input := range requiredInputs {
		value, _ := input.InputValue()
		if value == "" {
			fieldId, _ := input.GetAttribute("id")
			fieldName, _ := input.GetAttribute("name")
			placeholder, _ := input.GetAttribute("placeholder")
			ariaLabel, _ := input.GetAttribute("aria-label")
			
			// Create a unique identifier
			identifier := fieldId
			if identifier == "" {
				identifier = fieldName
			}
			if identifier == "" {
				identifier = fmt.Sprintf("field_%d", len(missingFields))
			}
			
			// Create description
			description := ariaLabel
			if description == "" {
				description = placeholder
			}
			if description == "" {
				description = fieldName
			}
			if description == "" {
				description = "Required field"
			}
			
			missingFields = append(missingFields, identifier)
			fieldDescriptions[identifier] = description
		}
	}
	
	// Check required selects
	requiredSelects, _ := page.Locator("select[required]:visible, select[aria-required='true']:visible").All()
	for _, sel := range requiredSelects {
		selectedOptions, _ := sel.Locator("option:checked").All()
		hasValidSelection := false
		for _, opt := range selectedOptions {
			value, _ := opt.GetAttribute("value")
			if value != "" && value != "0" && !strings.Contains(strings.ToLower(value), "select") {
				hasValidSelection = true
				break
			}
		}
		
		if !hasValidSelection {
			fieldId, _ := sel.GetAttribute("id")
			fieldName, _ := sel.GetAttribute("name")
			ariaLabel, _ := sel.GetAttribute("aria-label")
			
			identifier := fieldId
			if identifier == "" {
				identifier = fieldName
			}
			if identifier == "" {
				identifier = fmt.Sprintf("select_%d", len(missingFields))
			}
			
			description := ariaLabel
			if description == "" {
				description = fieldName
			}
			if description == "" {
				description = "Required dropdown"
			}
			
			missingFields = append(missingFields, identifier)
			fieldDescriptions[identifier] = description
		}
	}
	
	// Check required textareas
	requiredTextareas, _ := page.Locator("textarea[required]:visible, textarea[aria-required='true']:visible").All()
	for _, textarea := range requiredTextareas {
		value, _ := textarea.InputValue()
		if value == "" {
			fieldId, _ := textarea.GetAttribute("id")
			fieldName, _ := textarea.GetAttribute("name")
			placeholder, _ := textarea.GetAttribute("placeholder")
			ariaLabel, _ := textarea.GetAttribute("aria-label")
			
			identifier := fieldId
			if identifier == "" {
				identifier = fieldName
			}
			if identifier == "" {
				identifier = fmt.Sprintf("textarea_%d", len(missingFields))
			}
			
			description := ariaLabel
			if description == "" {
				description = placeholder
			}
			if description == "" {
				description = fieldName
			}
			if description == "" {
				description = "Required text field"
			}
			
			missingFields = append(missingFields, identifier)
			fieldDescriptions[identifier] = description
		}
	}
	
	return missingFields, fieldDescriptions
}