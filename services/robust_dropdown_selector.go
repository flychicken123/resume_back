package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// RobustDropdownSelector ensures dropdown values are actually selected
func RobustDropdownSelector(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== ROBUST DROPDOWN SELECTOR - ENSURING ALL DROPDOWNS ARE FILLED ===")
	
	// Wait for page to be fully loaded
	page.WaitForTimeout(2000)
	
	// Strategy 1: Handle React Select components (like Stripe/Greenhouse uses)
	if err := HandleCustomReactSelects(page, userData); err != nil {
		log.Printf("Error in HandleCustomReactSelects: %v", err)
	}
	
	// Strategy 2: Find all standard select elements and fill them
	if err := fillAllSelectElements(page, userData); err != nil {
		log.Printf("Error in fillAllSelectElements: %v", err)
	}
	
	// Strategy 3: Find dropdowns by clicking on divs with "Select..." text
	if err := fillCustomDropdowns(page, userData); err != nil {
		log.Printf("Error in fillCustomDropdowns: %v", err)
	}
	
	// Strategy 4: Verify and retry unfilled dropdowns
	if err := verifyAndRetryDropdowns(page, userData); err != nil {
		log.Printf("Error in verifyAndRetryDropdowns: %v", err)
	}
	
	return nil
}

func fillAllSelectElements(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Filling all SELECT elements ===")
	
	// Get all select elements
	selects, err := page.Locator("select").All()
	if err != nil {
		return fmt.Errorf("failed to find select elements: %v", err)
	}
	
	log.Printf("Found %d select elements on page", len(selects))
	
	for i, selectElem := range selects {
		// Check if visible
		isVisible, _ := selectElem.IsVisible()
		if !isVisible {
			log.Printf("Select %d is not visible, skipping", i)
			continue
		}
		
		// Get current value to check if already filled
		currentValue, _ := selectElem.InputValue()
		if currentValue != "" && !strings.Contains(strings.ToLower(currentValue), "select") {
			log.Printf("Select %d already has value: %s", i, currentValue)
			continue
		}
		
		// Get context about this dropdown
		context := getDropdownContext(page, selectElem, i)
		log.Printf("Select %d context: %s", i, context)
		
		// Determine value to select
		valueToSelect := determineValueForDropdown(context, userData)
		
		if valueToSelect == "" {
			log.Printf("Could not determine value for select %d", i)
			// Try to select any valid option
			valueToSelect = selectAnyValidOption(selectElem)
		}
		
		if valueToSelect != "" {
			// Method 1: Try SelectOption
			_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{valueToSelect}})
			if err != nil {
				log.Printf("Failed to select value '%s' with SelectOption: %v", valueToSelect, err)
				
				// Method 2: Try clicking and selecting
				if err := clickAndSelect(selectElem, valueToSelect); err != nil {
					log.Printf("Failed with clickAndSelect: %v", err)
					
					// Method 3: Try JavaScript
					if err := selectWithJavaScript(page, selectElem, valueToSelect); err != nil {
						log.Printf("Failed with JavaScript: %v", err)
					}
				}
			} else {
				log.Printf("✓ Successfully selected '%s' for dropdown %d", valueToSelect, i)
			}
		}
		
		// Verify the selection
		newValue, _ := selectElem.InputValue()
		if newValue != currentValue {
			log.Printf("✓ Dropdown %d value changed from '%s' to '%s'", i, currentValue, newValue)
		} else {
			log.Printf("⚠ Dropdown %d value unchanged: '%s'", i, currentValue)
		}
	}
	
	return nil
}

func getDropdownContext(page playwright.Page, selectElem playwright.Locator, index int) string {
	var context []string
	
	// Get select attributes
	if name, _ := selectElem.GetAttribute("name"); name != "" {
		context = append(context, "name:"+name)
	}
	if id, _ := selectElem.GetAttribute("id"); id != "" {
		context = append(context, "id:"+id)
	}
	if ariaLabel, _ := selectElem.GetAttribute("aria-label"); ariaLabel != "" {
		context = append(context, "aria:"+ariaLabel)
	}
	
	// Try to get label text
	if labelText := getLabelForSelect(page, selectElem); labelText != "" {
		context = append(context, "label:"+labelText)
	}
	
	// Get preceding text
	if precedingText := getPrecedingText(selectElem); precedingText != "" {
		context = append(context, "preceding:"+precedingText)
	}
	
	return strings.Join(context, " | ")
}

func getLabelForSelect(page playwright.Page, selectElem playwright.Locator) string {
	// Try to find associated label
	if id, _ := selectElem.GetAttribute("id"); id != "" {
		label := page.Locator(fmt.Sprintf("label[for='%s']", id))
		if count, _ := label.Count(); count > 0 {
			if text, _ := label.TextContent(); text != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	
	// Try parent label
	parentLabel := selectElem.Locator("xpath=ancestor::label")
	if count, _ := parentLabel.Count(); count > 0 {
		if text, _ := parentLabel.First().TextContent(); text != "" {
			return strings.TrimSpace(text)
		}
	}
	
	return ""
}

func getPrecedingText(selectElem playwright.Locator) string {
	// Get text from preceding sibling
	precedingSibling := selectElem.Locator("xpath=preceding-sibling::*[1]")
	if count, _ := precedingSibling.Count(); count > 0 {
		if text, _ := precedingSibling.TextContent(); text != "" {
			text = strings.TrimSpace(text)
			if len(text) < 200 { // Only use if reasonable length
				return text
			}
		}
	}
	return ""
}

func determineValueForDropdown(context string, userData *UserProfileData) string {
	contextLower := strings.ToLower(context)
	
	// Country where you reside
	if strings.Contains(contextLower, "where you") && strings.Contains(contextLower, "reside") {
		return "USA"
	}
	
	// Countries you anticipate working in
	if strings.Contains(contextLower, "anticipate working") || strings.Contains(contextLower, "countries") {
		return "USA"
	}
	
	// Work authorization
	if strings.Contains(contextLower, "authorized to work") {
		return "Yes"
	}
	
	// Sponsorship
	if strings.Contains(contextLower, "sponsor") || strings.Contains(contextLower, "work permit") {
		return "No"
	}
	
	// Remote work
	if strings.Contains(contextLower, "remote") {
		return "Yes"
	}
	
	// Previous employment
	if strings.Contains(contextLower, "employed by") && (strings.Contains(contextLower, "stripe") || strings.Contains(contextLower, "affiliate")) {
		return "No"
	}
	
	// WhatsApp
	if strings.Contains(contextLower, "whatsapp") {
		return "Yes"
	}
	
	// Gender
	if strings.Contains(contextLower, "gender") {
		return "Prefer not to answer"
	}
	
	// Race/ethnicity
	if strings.Contains(contextLower, "racial") || strings.Contains(contextLower, "ethnic") {
		return "Prefer not to answer"
	}
	
	// Sexual orientation
	if strings.Contains(contextLower, "sexual orientation") {
		return "Prefer not to answer"
	}
	
	// Transgender
	if strings.Contains(contextLower, "transgender") {
		return "No"
	}
	
	// Disability
	if strings.Contains(contextLower, "disability") || strings.Contains(contextLower, "chronic condition") {
		return "No"
	}
	
	// Veteran
	if strings.Contains(contextLower, "veteran") || strings.Contains(contextLower, "armed forces") {
		return "I am not a protected veteran"
	}
	
	return ""
}

func selectAnyValidOption(selectElem playwright.Locator) string {
	// Get all options
	options, _ := selectElem.Locator("option").All()
	
	// Try to find a valid option (skip first if it's a placeholder)
	for i, option := range options {
		value, _ := option.GetAttribute("value")
		text, _ := option.TextContent()
		textLower := strings.ToLower(strings.TrimSpace(text))
		
		// Skip placeholders
		if i == 0 && (value == "" || value == "0" || strings.Contains(textLower, "select") || strings.Contains(textLower, "choose")) {
			continue
		}
		
		// Return first valid option
		if value != "" && value != "0" && value != "-1" {
			log.Printf("Selecting fallback option: '%s' (value: %s)", text, value)
			return value
		}
	}
	
	return ""
}

func clickAndSelect(selectElem playwright.Locator, value string) error {
	// Click on the select to open it
	if err := selectElem.Click(); err != nil {
		return fmt.Errorf("failed to click select: %v", err)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	// Try to find and click the option
	option := selectElem.Locator(fmt.Sprintf("option[value='%s']", value))
	if count, _ := option.Count(); count > 0 {
		if err := option.Click(); err != nil {
			return fmt.Errorf("failed to click option: %v", err)
		}
	}
	
	return nil
}

func selectWithJavaScript(page playwright.Page, selectElem playwright.Locator, value string) error {
	// Use JavaScript to set the value directly
	_, err := selectElem.Evaluate(fmt.Sprintf(`
		(element) => {
			element.value = '%s';
			element.dispatchEvent(new Event('change', { bubbles: true }));
		}
	`, value), nil)
	
	return err
}

func fillCustomDropdowns(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Filling custom dropdowns (divs/buttons with 'Select...') ===")
	
	// Find all elements that look like custom dropdowns
	customDropdowns, _ := page.Locator("div:has-text('Select...'):visible, button:has-text('Select...'):visible, span:has-text('Select...'):visible").All()
	
	log.Printf("Found %d custom dropdown elements", len(customDropdowns))
	
	for i, dropdown := range customDropdowns {
		// Get parent context
		parentText := ""
		parent := dropdown.Locator("xpath=ancestor::div[1]")
		if count, _ := parent.Count(); count > 0 {
			parentText, _ = parent.TextContent()
		}
		
		log.Printf("Custom dropdown %d context: %s", i, parentText)
		
		// Click to open
		if err := dropdown.Click(); err != nil {
			log.Printf("Failed to click custom dropdown %d: %v", i, err)
			continue
		}
		
		time.Sleep(500 * time.Millisecond)
		
		// Determine what to select based on context
		valueToSelect := determineValueForDropdown(parentText, userData)
		
		if valueToSelect != "" {
			// Try to find and click the option in the dropdown menu
			if err := selectCustomDropdownOption(page, valueToSelect); err != nil {
				log.Printf("Failed to select option in custom dropdown: %v", err)
			} else {
				log.Printf("✓ Selected '%s' in custom dropdown %d", valueToSelect, i)
			}
		}
		
		// Click elsewhere to close dropdown if still open
		page.Locator("body").Click()
		time.Sleep(200 * time.Millisecond)
	}
	
	return nil
}

func selectCustomDropdownOption(page playwright.Page, value string) error {
	// Try various selectors for dropdown options
	optionSelectors := []string{
		fmt.Sprintf("li:has-text('%s'):visible", value),
		fmt.Sprintf("div[role='option']:has-text('%s'):visible", value),
		fmt.Sprintf("span:has-text('%s'):visible", value),
		fmt.Sprintf("option:has-text('%s'):visible", value),
		fmt.Sprintf("*[role='menuitem']:has-text('%s'):visible", value),
	}
	
	for _, selector := range optionSelectors {
		option := page.Locator(selector)
		if count, _ := option.Count(); count > 0 {
			if err := option.First().Click(); err == nil {
				return nil
			}
		}
	}
	
	// If exact match fails, try partial match
	partialSelectors := []string{
		fmt.Sprintf("li:visible:has-text('%s')", value[:min(len(value), 5)]),
		fmt.Sprintf("div[role='option']:visible:has-text('%s')", value[:min(len(value), 5)]),
	}
	
	for _, selector := range partialSelectors {
		option := page.Locator(selector)
		if count, _ := option.Count(); count > 0 {
			if err := option.First().Click(); err == nil {
				return nil
			}
		}
	}
	
	return fmt.Errorf("could not find option '%s'", value)
}

func verifyAndRetryDropdowns(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Verifying and retrying unfilled dropdowns ===")
	
	// Check all selects again
	selects, _ := page.Locator("select:visible").All()
	
	unfilledCount := 0
	for i, selectElem := range selects {
		value, _ := selectElem.InputValue()
		valueLower := strings.ToLower(value)
		
		// Check if still unfilled
		if value == "" || value == "0" || strings.Contains(valueLower, "select") || strings.Contains(valueLower, "choose") {
			unfilledCount++
			log.Printf("⚠ Dropdown %d is still unfilled: '%s'", i, value)
			
			// Get all options and force select the first valid one
			options, _ := selectElem.Locator("option").All()
			for j, option := range options {
				if j == 0 {
					continue // Skip first (usually placeholder)
				}
				
				optValue, _ := option.GetAttribute("value")
				optText, _ := option.TextContent()
				
				if optValue != "" && optValue != "0" {
					// Force select using JavaScript
					_, err := selectElem.Evaluate(fmt.Sprintf(`
						(element) => {
							element.value = '%s';
							element.dispatchEvent(new Event('change', { bubbles: true }));
							return element.value;
						}
					`, optValue), nil)
					
					if err == nil {
						log.Printf("✓ Force-selected '%s' for dropdown %d", optText, i)
						break
					}
				}
			}
		}
	}
	
	if unfilledCount > 0 {
		log.Printf("⚠ WARNING: %d dropdowns remain unfilled", unfilledCount)
		return fmt.Errorf("%d dropdowns could not be filled", unfilledCount)
	}
	
	log.Println("✓ All dropdowns have been filled")
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}