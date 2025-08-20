package services

import (
	"fmt"
	"log"
	"strings"

	"github.com/playwright-community/playwright-go"
)

// HandleDemographicDropdowns handles demographic and other dropdowns generically for all job sites
// This replaces the Stripe-specific handler to work across all platforms
func HandleDemographicDropdowns(iframe playwright.FrameLocator, userData *UserProfileData) error {
	log.Println("=== HANDLING DEMOGRAPHIC DROPDOWNS (GENERIC) ===")
	
	// Track which questions we've already processed to avoid duplicates
	processedQuestions := make(map[string]bool)
	processedCount := 0
	maxToProcess := 20 // Limit to avoid processing too many duplicates
	
	// Find all visible dropdowns (React Select components)
	dropdowns, _ := iframe.Locator("div[class*='select']:visible").All()
	
	for _, dropdown := range dropdowns {
		if processedCount >= maxToProcess {
			log.Printf("Reached max dropdown limit (%d), stopping", maxToProcess)
			break
		}
		
		// Look for question text near the dropdown
		questionText := ""
		
		// Try to find a label
		label := dropdown.Locator("xpath=preceding::label[1]")
		if count, _ := label.Count(); count > 0 {
			questionText, _ = label.TextContent()
		}
		
		// If no label, look for text in parent containers
		if questionText == "" {
			parent := dropdown.Locator("xpath=ancestor::div[1]")
			if count, _ := parent.Count(); count > 0 {
				text, _ := parent.TextContent()
				// Clean up the text
				lines := strings.Split(text, "\n")
				if len(lines) > 0 {
					questionText = strings.TrimSpace(lines[0])
				}
			}
		}
		
		// Skip if we've already processed this question
		if processedQuestions[questionText] {
			continue
		}
		
		// Check if this dropdown already has a value
		selectedValue := dropdown.Locator("div[class*='singleValue']").First()
		if count, _ := selectedValue.Count(); count > 0 {
			text, _ := selectedValue.TextContent()
			if text != "" && !strings.Contains(strings.ToLower(text), "select") {
				log.Printf("Dropdown already has value: %s", text)
				continue
			}
		}
		
		// Skip non-demographic questions (these are handled by other handlers)
		questionLower := strings.ToLower(questionText)
		isDemographic := false
		
		// List of demographic keywords to look for
		demographicKeywords := []string{
			"gender", "race", "ethnic", "veteran", "disability", 
			"sexual orientation", "transgender", "pronouns",
			"diversity", "demographic", "identity", "heritage",
		}
		
		for _, keyword := range demographicKeywords {
			if strings.Contains(questionLower, keyword) {
				isDemographic = true
				break
			}
		}
		
		// Also check for common non-text field questions
		otherQuestions := []string{
			"degree", "education", "university", "school", 
			"country", "state", "location", "years of experience",
			"salary", "start date", "notice period",
		}
		
		for _, keyword := range otherQuestions {
			if strings.Contains(questionLower, keyword) {
				isDemographic = true // Treat these as questions we should handle
				break
			}
		}
		
		if !isDemographic && questionText != "" {
			// Skip questions that are likely text inputs
			continue
		}
		
		log.Printf("Processing dropdown: %s", questionText)
		processedQuestions[questionText] = true
		processedCount++
		
		// Determine the answer
		answer := determineGenericDropdownValue(questionText, userData)
		if answer == "" {
			log.Printf("  No answer available for: %s", questionText)
			continue
		}
		
		// Click the dropdown to open it
		clickable := dropdown.Locator("div[class*='control']").First()
		if count, _ := clickable.Count(); count > 0 {
			clickable.Click()
			// Small wait for dropdown to open
			
			// Try to select the option
			selectGenericOption(iframe, answer, questionText)
			
			// Small wait after selection
		}
	}
	
	log.Printf("Processed %d demographic/generic dropdowns", processedCount)
	return nil
}

func selectGenericOption(iframe playwright.FrameLocator, valueToSelect string, fieldLabel string) {
	log.Printf("  Looking for option: %s", valueToSelect)
	
	// Try exact match first
	optionSelectors := []string{
		fmt.Sprintf("div[role='option']:text-is('%s'):visible", valueToSelect),
		fmt.Sprintf("div:has-text('%s'):visible", valueToSelect),
	}
	
	// For Yes/No questions, try variations
	if strings.ToLower(valueToSelect) == "yes" {
		optionSelectors = append(optionSelectors, 
			"div[role='option']:has-text('Yes'):visible",
			"div:has-text('Yes, I'):visible",
		)
	} else if strings.ToLower(valueToSelect) == "no" {
		optionSelectors = append(optionSelectors,
			"div[role='option']:has-text('No'):visible", 
			"div:has-text('No, I'):visible",
		)
	}
	
	// For gender options
	if strings.Contains(strings.ToLower(valueToSelect), "prefer not") {
		optionSelectors = append(optionSelectors,
			"div[role='option']:has-text('Prefer not to'):visible",
			"div[role='option']:has-text('I prefer not to'):visible",
			"div[role='option']:has-text('Decline to'):visible",
		)
	}
	
	for _, selector := range optionSelectors {
		option := iframe.Locator(selector).First()
		if count, _ := option.Count(); count > 0 {
			if err := option.Click(); err == nil {
				log.Printf("  ✓ Selected: %s", valueToSelect)
				return
			}
		}
	}
	
	log.Printf("  ✗ Could not find option: %s", valueToSelect)
}

func determineGenericDropdownValue(labelText string, userData *UserProfileData) string {
	labelLower := strings.ToLower(labelText)
	
	// First check ExtraQA for exact matches
	if userData.ExtraQA != nil {
		if value, exists := userData.ExtraQA[labelText]; exists {
			return value
		}
		if value, exists := userData.ExtraQA[labelLower]; exists {
			return value
		}
		
		// Check for partial matches
		for question, answer := range userData.ExtraQA {
			questionLower := strings.ToLower(question)
			
			// Match based on key terms
			if (strings.Contains(labelLower, "gender") && strings.Contains(questionLower, "gender")) ||
			   (strings.Contains(labelLower, "race") && strings.Contains(questionLower, "race")) ||
			   (strings.Contains(labelLower, "ethnic") && strings.Contains(questionLower, "ethnic")) ||
			   (strings.Contains(labelLower, "veteran") && strings.Contains(questionLower, "veteran")) ||
			   (strings.Contains(labelLower, "disability") && strings.Contains(questionLower, "disability")) ||
			   (strings.Contains(labelLower, "sexual orientation") && strings.Contains(questionLower, "sexual orientation")) ||
			   (strings.Contains(labelLower, "transgender") && strings.Contains(questionLower, "transgender")) {
				return answer
			}
		}
	}
	
	// Generic mappings for common questions
	
	// Gender questions
	if strings.Contains(labelLower, "gender") && !strings.Contains(labelLower, "transgender") {
		if userData.Gender != "" && userData.Gender != "prefer_not_to_say" {
			switch strings.ToLower(userData.Gender) {
			case "male":
				return "Man"
			case "female":
				return "Woman"
			default:
				return userData.Gender
			}
		}
		return "Prefer not to disclose"
	}
	
	// Race/Ethnicity questions
	if strings.Contains(labelLower, "race") || strings.Contains(labelLower, "ethnic") {
		if userData.Ethnicity != "" && userData.Ethnicity != "prefer_not_to_say" {
			return userData.Ethnicity
		}
		return "Prefer not to disclose"
	}
	
	// Veteran status
	if strings.Contains(labelLower, "veteran") {
		if userData.VeteranStatus == "yes" {
			return "Yes"
		} else if userData.VeteranStatus == "no" {
			return "No"
		}
		return "Prefer not to disclose"
	}
	
	// Disability status
	if strings.Contains(labelLower, "disability") || strings.Contains(labelLower, "chronic condition") {
		if userData.DisabilityStatus == "yes" {
			return "Yes"
		} else if userData.DisabilityStatus == "no" {
			return "No"
		}
		return "Prefer not to disclose"
	}
	
	// Sexual orientation
	if strings.Contains(labelLower, "sexual orientation") {
		if userData.SexualOrientation != "" {
			return userData.SexualOrientation
		}
		return "Prefer not to disclose"
	}
	
	// Transgender status
	if strings.Contains(labelLower, "transgender") {
		if userData.TransgenderStatus != "" {
			return userData.TransgenderStatus
		}
		return "Prefer not to disclose"
	}
	
	// Education level
	if strings.Contains(labelLower, "degree") || strings.Contains(labelLower, "education level") {
		if userData.MostRecentDegree != "" {
			return userData.MostRecentDegree
		}
		if len(userData.Education) > 0 && userData.Education[0].Degree != "" {
			return userData.Education[0].Degree
		}
		return ""
	}
	
	// University/School
	if strings.Contains(labelLower, "university") || strings.Contains(labelLower, "college") || strings.Contains(labelLower, "school") {
		if userData.University != "" {
			return userData.University
		}
		if len(userData.Education) > 0 && userData.Education[0].Institution != "" {
			return userData.Education[0].Institution
		}
		return ""
	}
	
	// Country questions
	if strings.Contains(labelLower, "country") {
		if userData.Country != "" {
			return userData.Country
		}
		return "United States"
	}
	
	// Work authorization
	if strings.Contains(labelLower, "authorized to work") || strings.Contains(labelLower, "work authorization") {
		if userData.WorkAuthorization == "yes" {
			return "Yes"
		} else if userData.WorkAuthorization == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// Sponsorship
	if strings.Contains(labelLower, "sponsor") {
		if userData.RequiresSponsorship {
			return "Yes"
		}
		return "No"
	}
	
	// Years of experience - try to infer from user data
	if strings.Contains(labelLower, "years of experience") || strings.Contains(labelLower, "experience") {
		// Calculate from work experience if available
		if len(userData.Experience) > 0 {
			// This would need more logic to calculate actual years
			return "3-5 years" // Default for now
		}
		return ""
	}
	
	// Salary expectations
	if strings.Contains(labelLower, "salary") || strings.Contains(labelLower, "compensation") {
		// Would need ExpectedSalary field in UserProfileData
		return ""
	}
	
	// Start date
	if strings.Contains(labelLower, "start date") || strings.Contains(labelLower, "available to start") {
		if userData.AvailableStartDate != "" {
			return userData.AvailableStartDate
		}
		return "Immediately"
	}
	
	// Notice period
	if strings.Contains(labelLower, "notice period") {
		// Would need NoticePeriod field in UserProfileData
		return "2 weeks"
	}
	
	// Remote work preference
	if strings.Contains(labelLower, "remote") {
		if userData.RemoteWorkPreference == "yes" || userData.RemoteWorkPreference == "remote" {
			return "Yes"
		} else if userData.RemoteWorkPreference == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// Previous employment
	if strings.Contains(labelLower, "previously employed") || strings.Contains(labelLower, "worked at") {
		return "No"
	}
	
	// Default: return empty to skip unknown questions
	return ""
}