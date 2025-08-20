package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// WorkingDropdownHandler - A proven approach to handle React Select dropdowns
func WorkingDropdownHandler(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== WORKING DROPDOWN HANDLER - FILLING ALL DROPDOWNS ===")
	
	// Give page time to fully load
	time.Sleep(3 * time.Second)
	
	// Method 1: Fill by clicking on placeholders and selecting options
	if err := fillByPlaceholderClick(page, userData); err != nil {
		log.Printf("fillByPlaceholderClick error: %v", err)
	}
	
	// Method 2: Use JavaScript to directly set values
	if err := fillByJavaScript(page, userData); err != nil {
		log.Printf("fillByJavaScript error: %v", err)
	}
	
	// Method 3: Fill using keyboard navigation
	if err := fillByKeyboardNavigation(page, userData); err != nil {
		log.Printf("fillByKeyboardNavigation error: %v", err)
	}
	
	return nil
}

func fillByPlaceholderClick(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Method 1: Clicking placeholders and selecting options ===")
	
	// Find all elements with "Select..." text
	placeholders, err := page.Locator("div:has-text('Select...'):visible").All()
	if err != nil {
		return err
	}
	
	log.Printf("Found %d placeholders with 'Select...' text", len(placeholders))
	
	for i, placeholder := range placeholders {
		// Get the question text
		questionText := getQuestionText(page, placeholder)
		log.Printf("Dropdown %d: %s", i, questionText)
		
		// Determine the value to select
		valueToSelect := getValueForQuestion(questionText, userData)
		if valueToSelect == "" {
			log.Printf("  Could not determine value for: %s", questionText)
			continue
		}
		
		log.Printf("  Will select: %s", valueToSelect)
		
		// Click the placeholder to open dropdown
		if err := placeholder.Click(); err != nil {
			log.Printf("  Failed to click placeholder: %v", err)
			continue
		}
		
		// Wait for dropdown to open
		time.Sleep(500 * time.Millisecond)
		
		// Try to find and click the option
		success := false
		
		// Try exact match first
		exactOption := page.Locator(fmt.Sprintf("div[role='option']:text-is('%s'):visible", valueToSelect)).First()
		if count, _ := exactOption.Count(); count > 0 {
			if err := exactOption.Click(); err == nil {
				log.Printf("  ✓ Selected: %s", valueToSelect)
				success = true
			}
		}
		
		// Try contains match if exact fails
		if !success {
			containsOption := page.Locator(fmt.Sprintf("div:has-text('%s'):visible", valueToSelect)).First()
			if count, _ := containsOption.Count(); count > 0 {
				if err := containsOption.Click(); err == nil {
					log.Printf("  ✓ Selected (partial match): %s", valueToSelect)
					success = true
				}
			}
		}
		
		// If still no success, try clicking first available option
		if !success {
			firstOption := page.Locator("div[role='option']:visible").First()
			if count, _ := firstOption.Count(); count > 0 {
				optText, _ := firstOption.TextContent()
				if err := firstOption.Click(); err == nil {
					log.Printf("  ✓ Selected first option: %s", optText)
					success = true
				}
			}
		}
		
		if !success {
			log.Printf("  ✗ Failed to select any option")
			// Click elsewhere to close dropdown
			page.Locator("body").Click()
		}
		
		// Wait before next dropdown
		time.Sleep(500 * time.Millisecond)
	}
	
	return nil
}

func fillByJavaScript(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Method 2: Using JavaScript to set values ===")
	
	// JavaScript to find and fill all React Select components
	script := `
		() => {
			const selects = document.querySelectorAll('input[role="combobox"]');
			const results = [];
			
			selects.forEach((input, index) => {
				// Get the label
				const labelId = input.getAttribute('aria-labelledby');
				const label = labelId ? document.getElementById(labelId) : null;
				const labelText = label ? label.textContent : '';
				
				// Get current value
				const currentValue = input.value;
				
				results.push({
					index: index,
					id: input.id,
					labelText: labelText,
					currentValue: currentValue,
					hasValue: currentValue && currentValue !== ''
				});
			});
			
			return results;
		}
	`
	
	result, err := page.Evaluate(script)
	if err != nil {
		return fmt.Errorf("failed to evaluate script: %v", err)
	}
	
	// Process each select found
	if selectsData, ok := result.([]interface{}); ok {
		for _, selectData := range selectsData {
			if data, ok := selectData.(map[string]interface{}); ok {
				labelText, _ := data["labelText"].(string)
				hasValue, _ := data["hasValue"].(bool)
				id, _ := data["id"].(string)
				
				if hasValue {
					log.Printf("Select %s already has value", id)
					continue
				}
				
				// Determine value
				valueToSelect := getValueForQuestion(labelText, userData)
				if valueToSelect == "" {
					continue
				}
				
				// Try to set the value using JavaScript
				setScript := fmt.Sprintf(`
					() => {
						const input = document.getElementById('%s');
						if (!input) return false;
						
						// Focus the input
						input.focus();
						input.click();
						
						// Wait and then try to select option
						setTimeout(() => {
							const options = document.querySelectorAll('div[role="option"]');
							for (let opt of options) {
								if (opt.textContent.includes('%s')) {
									opt.click();
									return true;
								}
							}
						}, 500);
						
						return false;
					}
				`, id, valueToSelect)
				
				page.Evaluate(setScript)
				time.Sleep(700 * time.Millisecond)
				
				log.Printf("Attempted to set %s to %s via JavaScript", id, valueToSelect)
			}
		}
	}
	
	return nil
}

func fillByKeyboardNavigation(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Method 3: Using keyboard navigation ===")
	
	// Find all combobox inputs
	comboboxes, err := page.Locator("input[role='combobox']:visible").All()
	if err != nil {
		return err
	}
	
	log.Printf("Found %d combobox inputs", len(comboboxes))
	
	for i, combobox := range comboboxes {
		// Check if already has value
		value, _ := combobox.InputValue()
		if value != "" {
			log.Printf("Combobox %d already has value: %s", i, value)
			continue
		}
		
		// Get label
		labelId, _ := combobox.GetAttribute("aria-labelledby")
		labelText := ""
		if labelId != "" {
			label := page.Locator(fmt.Sprintf("#%s", labelId))
			if count, _ := label.Count(); count > 0 {
				labelText, _ = label.TextContent()
			}
		}
		
		log.Printf("Combobox %d: %s", i, labelText)
		
		// Determine value
		valueToSelect := getValueForQuestion(labelText, userData)
		if valueToSelect == "" {
			continue
		}
		
		// Focus and open dropdown
		if err := combobox.Focus(); err != nil {
			continue
		}
		
		// Press space or enter to open
		combobox.Press("Space")
		time.Sleep(500 * time.Millisecond)
		
		// Type the first few letters
		if len(valueToSelect) > 3 {
			combobox.Type(valueToSelect[:3])
			time.Sleep(300 * time.Millisecond)
		}
		
		// Press Enter to select
		combobox.Press("Enter")
		
		log.Printf("Attempted keyboard selection for: %s", valueToSelect)
	}
	
	return nil
}

func getQuestionText(page playwright.Page, element playwright.Locator) string {
	// Try to find the label or question text
	
	// Method 1: Look for preceding label
	label := element.Locator("xpath=preceding::label[1]")
	if count, _ := label.Count(); count > 0 {
		if text, _ := label.TextContent(); text != "" {
			return strings.TrimSpace(text)
		}
	}
	
	// Method 2: Look for parent with label
	parent := element.Locator("xpath=ancestor::div[contains(@class, 'select')]")
	if count, _ := parent.Count(); count > 0 {
		label := parent.Locator("label").First()
		if text, _ := label.TextContent(); text != "" {
			return strings.TrimSpace(text)
		}
	}
	
	// Method 3: Look for aria-labelledby
	parent2 := element.Locator("xpath=parent::div/parent::div")
	if count, _ := parent2.Count(); count > 0 {
		input := parent2.Locator("input[aria-labelledby]").First()
		if labelId, _ := input.GetAttribute("aria-labelledby"); labelId != "" {
			label := page.Locator(fmt.Sprintf("#%s", labelId))
			if text, _ := label.TextContent(); text != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	
	return ""
}

func getValueForQuestion(question string, userData *UserProfileData) string {
	questionLower := strings.ToLower(question)
	
	// Map questions to values based on actual form
	
	// "Please select the country where you currently reside."
	if strings.Contains(questionLower, "where you currently reside") {
		if userData.Country != "" {
			return userData.Country
		}
		return "United States"
	}
	
	// "Please select the country or countries you anticipate working in"
	if strings.Contains(questionLower, "anticipate working") {
		if userData.Country != "" {
			return userData.Country
		}
		return "USA"
	}
	
	// "Are you authorized to work in the location(s)"
	if strings.Contains(questionLower, "authorized to work") {
		if userData.WorkAuthorization == "yes" {
			return "Yes"
		} else if userData.WorkAuthorization == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// "Will you require Stripe to sponsor you for a work permit"
	if strings.Contains(questionLower, "sponsor") && strings.Contains(questionLower, "work permit") {
		if userData.RequiresSponsorship {
			return "Yes"
		}
		return "No"
	}
	
	// "If this role offers the option to work from a remote location, do you plan to work remotely?"
	if strings.Contains(questionLower, "work remotely") || strings.Contains(questionLower, "remote location") {
		if userData.RemoteWorkPreference == "yes" || userData.RemoteWorkPreference == "remote" {
			return "Yes"
		} else if userData.RemoteWorkPreference == "no" {
			return "No"
		}
		return "Yes"
	}
	
	// "Have you ever been employed by Stripe or a Stripe affiliate?"
	if strings.Contains(questionLower, "employed by stripe") {
		return "No"
	}
	
	// "Do you opt-in to receive WhatsApp messages"
	if strings.Contains(questionLower, "whatsapp") {
		return "Yes"
	}
	
	// Demographic questions
	if strings.Contains(questionLower, "gender identity") {
		if userData.Gender != "" && userData.Gender != "prefer_not_to_say" {
			return userData.Gender
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(questionLower, "racial/ethnic") {
		if userData.Ethnicity != "" && userData.Ethnicity != "prefer_not_to_say" {
			return userData.Ethnicity
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(questionLower, "sexual orientation") {
		return "Prefer not to answer"
	}
	
	if strings.Contains(questionLower, "transgender") {
		return "Prefer not to answer"
	}
	
	if strings.Contains(questionLower, "disability") || strings.Contains(questionLower, "chronic condition") {
		if userData.DisabilityStatus == "yes" {
			return "Yes"
		} else if userData.DisabilityStatus == "no" {
			return "No"
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(questionLower, "veteran") || strings.Contains(questionLower, "armed forces") {
		if userData.VeteranStatus == "yes" {
			return "Yes"
		} else if userData.VeteranStatus == "no" {
			return "No"
		}
		return "I am not a protected veteran"
	}
	
	return ""
}