package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// HandleIframeDropdowns handles dropdowns inside an iframe (like Stripe/Greenhouse forms)
func HandleIframeDropdowns(iframe playwright.FrameLocator, userData *UserProfileData) error {
	log.Println("=== HANDLING DROPDOWNS INSIDE IFRAME ===")
	
	// Give iframe time to load
	time.Sleep(2 * time.Second)
	
	// Track filled dropdowns to avoid infinite loops
	filledDropdowns := make(map[string]bool)
	unknownFields := []string{}
	
	// Method 1: Find React Select components in iframe
	unknownFields = append(unknownFields, handleIframeReactSelects(iframe, userData, filledDropdowns)...)
	
	// Method 2: Find standard select elements in iframe
	unknownFields = append(unknownFields, handleIframeStandardSelects(iframe, userData, filledDropdowns)...)
	
	// Method 3: Click on "Select..." placeholders in iframe
	unknownFields = append(unknownFields, handleIframePlaceholders(iframe, userData, filledDropdowns)...)
	
	// If we have unknown fields, return an error to stop the process
	if len(unknownFields) > 0 {
		log.Printf("⚠️ STOPPING: Found %d unknown fields that require user input:", len(unknownFields))
		for _, field := range unknownFields {
			log.Printf("  - %s", field)
		}
		return fmt.Errorf("Unable to fill %d fields. Please provide answers for: %v", len(unknownFields), unknownFields)
	}
	
	return nil
}

func handleIframeReactSelects(iframe playwright.FrameLocator, userData *UserProfileData, filledDropdowns map[string]bool) []string {
	log.Println("=== Looking for React Select components in iframe ===")
	unknownFields := []string{}
	
	// Find all combobox inputs in the iframe
	comboboxes, err := iframe.Locator("input[role='combobox']").All()
	if err != nil {
		log.Printf("Error finding comboboxes in iframe: %v", err)
		return unknownFields
	}
	
	log.Printf("Found %d combobox inputs in iframe", len(comboboxes))
	
	for i, combobox := range comboboxes {
		// Check if visible
		isVisible, _ := combobox.IsVisible()
		if !isVisible {
			continue
		}
		
		// Get the label
		labelId, _ := combobox.GetAttribute("aria-labelledby")
		labelText := ""
		if labelId != "" {
			label := iframe.Locator(fmt.Sprintf("#%s", labelId))
			if count, _ := label.Count(); count > 0 {
				labelText, _ = label.TextContent()
			}
		}
		
		// Check current value
		currentValue, _ := combobox.InputValue()
		if currentValue != "" {
			log.Printf("Combobox %d already has value: %s", i, currentValue)
			continue
		}
		
		log.Printf("Combobox %d in iframe: %s", i, labelText)
		
		// Check if we've already processed this field
		fieldKey := fmt.Sprintf("combobox_%s", labelText)
		if filledDropdowns[fieldKey] {
			log.Printf("  Already processed this field, skipping")
			continue
		}
		filledDropdowns[fieldKey] = true
		
		// Determine value to select
		valueToSelect := determineIframeDropdownValue(labelText, userData)
		if valueToSelect == "" {
			log.Printf("  ⚠️ Unknown field - need user input: %s", labelText)
			if labelText != "" { // Only add if we have a label
				unknownFields = append(unknownFields, labelText)
			}
			// Don't try to fill it - continue to next field
			continue
		}
		
		log.Printf("  Will select: %s", valueToSelect)
		
		// Click to open dropdown
		if err := combobox.Click(); err != nil {
			log.Printf("  Failed to click combobox: %v", err)
			continue
		}
		
		time.Sleep(500 * time.Millisecond)
		
		// Try to find and click the option
		selectIframeOption(iframe, valueToSelect, labelText)
		
		time.Sleep(500 * time.Millisecond)
	}
	
	return unknownFields
}

func handleIframeStandardSelects(iframe playwright.FrameLocator, userData *UserProfileData, filledDropdowns map[string]bool) []string {
	log.Println("=== Looking for standard SELECT elements in iframe ===")
	unknownFields := []string{}
	
	// Find all select elements
	selects, err := iframe.Locator("select").All()
	if err != nil {
		log.Printf("Error finding selects in iframe: %v", err)
		return unknownFields
	}
	
	log.Printf("Found %d select elements in iframe", len(selects))
	
	for i, selectElem := range selects {
		// Check if visible
		isVisible, _ := selectElem.IsVisible()
		if !isVisible {
			continue
		}
		
		// Get current value
		currentValue, _ := selectElem.InputValue()
		if currentValue != "" && !strings.Contains(strings.ToLower(currentValue), "select") {
			log.Printf("Select %d already has value: %s", i, currentValue)
			continue
		}
		
		// Get context
		selectId, _ := selectElem.GetAttribute("id")
		selectName, _ := selectElem.GetAttribute("name")
		
		// Try to find label
		labelText := ""
		if selectId != "" {
			label := iframe.Locator(fmt.Sprintf("label[for='%s']", selectId))
			if count, _ := label.Count(); count > 0 {
				labelText, _ = label.TextContent()
			}
		}
		
		log.Printf("Select %d in iframe: id='%s', name='%s', label='%s'", i, selectId, selectName, labelText)
		
		// Check if we've already processed this field
		fieldKey := fmt.Sprintf("select_%s_%s_%s", selectId, selectName, labelText)
		if filledDropdowns[fieldKey] {
			log.Printf("  Already processed this field, skipping")
			continue
		}
		filledDropdowns[fieldKey] = true
		
		// Determine value
		valueToSelect := determineIframeDropdownValue(labelText, userData)
		if valueToSelect == "" {
			// Try with name/id as context
			valueToSelect = determineIframeDropdownValue(selectName+" "+selectId, userData)
		}
		
		if valueToSelect == "" {
			fieldDescription := labelText
			if fieldDescription == "" {
				fieldDescription = fmt.Sprintf("Select field: %s %s", selectName, selectId)
			}
			log.Printf("  ⚠️ Unknown field - need user input: %s", fieldDescription)
			unknownFields = append(unknownFields, fieldDescription)
			continue
		}
		
		log.Printf("  Will select: %s", valueToSelect)
		
		// Try to select the option
		_, err := selectElem.SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{valueToSelect},
		})
		if err != nil {
			// Try by value
			_, err = selectElem.SelectOption(playwright.SelectOptionValues{
				Values: &[]string{valueToSelect},
			})
		}
		
		if err != nil {
			log.Printf("  Failed to select: %v", err)
		} else {
			log.Printf("  ✓ Selected: %s", valueToSelect)
		}
	}
	
	return unknownFields
}

func handleIframePlaceholders(iframe playwright.FrameLocator, userData *UserProfileData, filledDropdowns map[string]bool) []string {
	log.Println("=== Looking for 'Select...' placeholders in iframe ===")
	unknownFields := []string{}
	unknownFieldsMap := make(map[string]bool) // Track unique unknown fields
	
	// Find all elements with "Select..." text
	placeholders, err := iframe.Locator("div:has-text('Select...'):visible, span:has-text('Select...'):visible").All()
	if err != nil {
		log.Printf("Error finding placeholders in iframe: %v", err)
		return unknownFields
	}
	
	log.Printf("Found %d 'Select...' placeholders in iframe", len(placeholders))
	
	// Process placeholders one by one, but skip duplicates
	for i, placeholder := range placeholders {
		// Get parent context to understand what this dropdown is for
		parent := placeholder.Locator("xpath=ancestor::div[contains(@class, 'select')]").First()
		labelText := ""
		
		if count, _ := parent.Count(); count > 0 {
			// Look for label within parent
			label := parent.Locator("label").First()
			if labelCount, _ := label.Count(); labelCount > 0 {
				labelText, _ = label.TextContent()
			}
		}
		
		// If no label found, try preceding label
		if labelText == "" {
			precedingLabel := placeholder.Locator("xpath=preceding::label[1]").First()
			if count, _ := precedingLabel.Count(); count > 0 {
				labelText, _ = precedingLabel.TextContent()
			}
		}
		
		log.Printf("Placeholder %d in iframe: %s", i, labelText)
		
		// Check if we've already processed this question
		if filledDropdowns[labelText] {
			log.Printf("  Already processed this question, skipping")
			continue
		}
		filledDropdowns[labelText] = true
		
		// Determine value
		valueToSelect := determineIframeDropdownValue(labelText, userData)
		if valueToSelect == "" {
			log.Printf("  ⚠️ Unknown field - need user input: %s", labelText)
			// Add to unknown fields if not already there
			if !unknownFieldsMap[labelText] {
				unknownFieldsMap[labelText] = true
				unknownFields = append(unknownFields, labelText)
			}
			// Don't try to fill it - just continue to next field
			continue
		}
		
		log.Printf("  Will select: %s", valueToSelect)
		
		// Click placeholder to open dropdown
		if err := placeholder.Click(); err != nil {
			log.Printf("  Failed to click placeholder: %v", err)
			continue
		}
		
		time.Sleep(500 * time.Millisecond)
		
		// Try to select option
		selectIframeOption(iframe, valueToSelect, labelText)
		
		time.Sleep(500 * time.Millisecond)
	}
	
	return unknownFields
}

func selectIframeOption(iframe playwright.FrameLocator, valueToSelect string, fieldLabel string) {
	log.Printf("  Looking for option: %s", valueToSelect)
	
	// Try various selectors for options
	maxLen := len(valueToSelect)
	if maxLen > 5 {
		maxLen = 5
	}
	optionSelectors := []string{
		fmt.Sprintf("div[role='option']:text-is('%s'):visible", valueToSelect),
		fmt.Sprintf("div[role='option']:has-text('%s'):visible", valueToSelect),
		fmt.Sprintf("div[class*='option']:has-text('%s'):visible", valueToSelect),
		fmt.Sprintf("li:has-text('%s'):visible", valueToSelect),
		fmt.Sprintf("div:has-text('%s'):visible", valueToSelect[:maxLen]),
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
	
	// Special handling for country fields
	if strings.Contains(strings.ToLower(fieldLabel), "country") {
		alternatives := []string{"United States", "USA", "US", "United States of America"}
		for _, alt := range alternatives {
			option := iframe.Locator(fmt.Sprintf("div[role='option']:has-text('%s'):visible", alt)).First()
			if count, _ := option.Count(); count > 0 {
				if err := option.Click(); err == nil {
					log.Printf("  ✓ Selected: %s (country alternative)", alt)
					return
				}
			}
		}
	}
	
	// Last resort: select first available option
	firstOption := iframe.Locator("div[role='option']:visible").First()
	if count, _ := firstOption.Count(); count > 0 {
		text, _ := firstOption.TextContent()
		if err := firstOption.Click(); err == nil {
			log.Printf("  ✓ Selected first option: %s", text)
			return
		}
	}
	
	log.Printf("  ✗ Could not select any option")
}

func determineIframeDropdownValue(labelText string, userData *UserProfileData) string {
	labelLower := strings.ToLower(labelText)
	
	// Debug logging
	// Determine value for dropdown
	
	// First, check if we have this question in our ExtraQA
	if userData.ExtraQA != nil {
		// Try exact match first
		if value, exists := userData.ExtraQA[labelText]; exists {
			log.Printf("  Found answer in ExtraQA for '%s': %s", labelText, value)
			return value
		}
		// Also check with lowercase
		if value, exists := userData.ExtraQA[labelLower]; exists {
			log.Printf("  Found answer in ExtraQA for '%s': %s", labelText, value)
			return value
		}
		
		// Try to find a partial match for demographic questions
		for question, answer := range userData.ExtraQA {
			questionLower := strings.ToLower(question)
			// Check for sexual orientation question
			if strings.Contains(labelLower, "sexual orientation") && strings.Contains(questionLower, "sexual orientation") {
				log.Printf("  Found partial match in ExtraQA for sexual orientation: %s", answer)
				return answer
			}
			// Check for transgender question
			if strings.Contains(labelLower, "transgender") && strings.Contains(questionLower, "transgender") {
				log.Printf("  Found partial match in ExtraQA for transgender: %s", answer)
				return answer
			}
			// Check for disability question
			if (strings.Contains(labelLower, "disability") || strings.Contains(labelLower, "chronic")) && 
			   (strings.Contains(questionLower, "disability") || strings.Contains(questionLower, "chronic")) {
				log.Printf("  Found partial match in ExtraQA for disability: %s", answer)
				return answer
			}
		}
	}
	
	// Country selection
	if strings.Contains(labelLower, "country") {
		if strings.Contains(labelLower, "currently reside") {
			// Use user's country or default
			if userData.Country != "" {
				return userData.Country
			}
			return "United States"
		}
		if strings.Contains(labelLower, "anticipate working") {
			if userData.Country != "" {
				return userData.Country
			}
			return "USA"
		}
		return "United States"
	}
	
	// Work authorization
	if strings.Contains(labelLower, "authorized to work") {
		if userData.WorkAuthorization == "yes" {
			return "Yes"
		} else if userData.WorkAuthorization == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// Sponsorship
	if strings.Contains(labelLower, "sponsor") && strings.Contains(labelLower, "work permit") {
		if userData.RequiresSponsorship {
			return "Yes"
		}
		return "No"
	}
	
	// Remote work
	if strings.Contains(labelLower, "remote") && strings.Contains(labelLower, "work") {
		if userData.RemoteWorkPreference == "yes" || userData.RemoteWorkPreference == "remote" {
			return "Yes"
		} else if userData.RemoteWorkPreference == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// Previous employment
	if strings.Contains(labelLower, "employed by") {
		return "No"
	}
	
	// WhatsApp
	if strings.Contains(labelLower, "whatsapp") {
		return "Yes"
	}
	
	// Education/Degree questions - use user's education data
	if strings.Contains(labelLower, "degree") || strings.Contains(labelLower, "education") {
		// First check if we have the specific most recent degree field
		if userData.MostRecentDegree != "" {
			return userData.MostRecentDegree
		}
		// Fall back to education array
		if len(userData.Education) > 0 && userData.Education[0].Degree != "" {
			// Return the user's most recent degree
			return userData.Education[0].Degree
		}
		// Don't auto-select - ask user
		return ""
	}
	
	// University/School questions
	if strings.Contains(labelLower, "university") || strings.Contains(labelLower, "college") || strings.Contains(labelLower, "school") {
		// Check specific university field first
		if userData.University != "" {
			return userData.University
		}
		// Fall back to education array
		if len(userData.Education) > 0 && userData.Education[0].Institution != "" {
			return userData.Education[0].Institution
		}
		// Check RecentSchool field
		if userData.RecentSchool != "" {
			return userData.RecentSchool
		}
		return ""
	}
	
	// Major/Field of study questions
	if strings.Contains(labelLower, "major") || strings.Contains(labelLower, "field of study") {
		if userData.Major != "" {
			return userData.Major
		}
		// Fall back to education array if we have field info there
		if len(userData.Education) > 0 && userData.Education[0].Field != "" {
			return userData.Education[0].Field
		}
		return ""
	}
	
	// Demographic questions - only fill if user explicitly provided data
	// Check more specific questions first to avoid false matches
	
	if strings.Contains(labelLower, "transgender") {
		// Use user's transgender status if provided
		if userData.TransgenderStatus != "" {
			return userData.TransgenderStatus
		}
		// Don't auto-select - ask user for transgender status
		return ""
	}
	
	if strings.Contains(labelLower, "sexual orientation") {
		// Use user's sexual orientation if provided
		if userData.SexualOrientation != "" {
			return userData.SexualOrientation
		}
		// Don't auto-select - ask user
		return ""
	}
	
	// Gender identity - but NOT transgender question
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
		// Don't auto-select - ask user
		return ""
	}
	
	if strings.Contains(labelLower, "racial") || strings.Contains(labelLower, "ethnic") {
		if userData.Ethnicity != "" && userData.Ethnicity != "prefer_not_to_say" {
			return userData.Ethnicity
		}
		// Don't auto-select - ask user
		return ""
	}
	
	if strings.Contains(labelLower, "disability") || strings.Contains(labelLower, "chronic") {
		if userData.DisabilityStatus == "yes" {
			return "Yes"
		} else if userData.DisabilityStatus == "no" {
			return "No"
		}
		// Don't auto-select - ask user
		return ""
	}
	
	if strings.Contains(labelLower, "veteran") {
		if userData.VeteranStatus == "yes" {
			return "Yes"
		} else if userData.VeteranStatus == "no" {
			return "No"
		}
		// Don't auto-select - ask user
		return ""
	}
	
	return ""
}