package services

import (
	"fmt"
	"log"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// HandleCustomReactSelects handles custom React Select components (like in Stripe/Greenhouse forms)
func HandleCustomReactSelects(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Handling Custom React Select Components ===")
	
	// Wait for page to stabilize
	page.WaitForTimeout(2000)
	
	// Strategy 1: Find all select containers with "Select..." placeholder
	selectContainers, err := page.Locator("div.select__container, div[class*='select__container']").All()
	if err == nil && len(selectContainers) > 0 {
		log.Printf("Found %d React Select containers", len(selectContainers))
		for i, container := range selectContainers {
			handleSingleReactSelect(page, container, i, userData)
		}
	}
	
	// Strategy 2: Find by placeholder text
	placeholders, err := page.Locator("div:has-text('Select...'):visible").All()
	if err == nil && len(placeholders) > 0 {
		log.Printf("Found %d elements with 'Select...' text", len(placeholders))
		for i, placeholder := range placeholders {
			// Check if it's part of a select component
			parent := placeholder.Locator("xpath=ancestor::div[contains(@class, 'select')]")
			if count, _ := parent.Count(); count > 0 {
				handleReactSelectByPlaceholder(page, placeholder, i, userData)
			}
		}
	}
	
	// Strategy 3: Find by specific input role
	comboboxes, err := page.Locator("input[role='combobox']").All()
	if err == nil && len(comboboxes) > 0 {
		log.Printf("Found %d combobox inputs", len(comboboxes))
		for i, combobox := range comboboxes {
			handleReactSelectByCombobox(page, combobox, i, userData)
		}
	}
	
	return nil
}

func handleSingleReactSelect(page playwright.Page, container playwright.Locator, index int, userData *UserProfileData) {
	// Get label text to understand what field this is
	label := container.Locator("label").First()
	labelText := ""
	if count, _ := label.Count(); count > 0 {
		labelText, _ = label.TextContent()
	}
	
	log.Printf("React Select %d: %s", index, labelText)
	
	// Check if already has a value
	valueContainer := container.Locator("div.select__single-value, div[class*='singleValue']").First()
	if count, _ := valueContainer.Count(); count > 0 {
		currentValue, _ := valueContainer.TextContent()
		if currentValue != "" && !strings.Contains(strings.ToLower(currentValue), "select") {
			log.Printf("  Already has value: %s", currentValue)
			return
		}
	}
	
	// Determine what to select
	valueToSelect := determineValueForField(labelText, userData)
	if valueToSelect == "" {
		log.Printf("  Could not determine value for: %s", labelText)
		return
	}
	
	// Click to open the dropdown
	clickTarget := container.Locator("div.select__control, div[class*='control']").First()
	if count, _ := clickTarget.Count(); count > 0 {
		if err := clickTarget.Click(); err != nil {
			log.Printf("  Failed to click control: %v", err)
			return
		}
		
		// Wait for options to appear
		page.WaitForTimeout(500)
		
		// Try to select the option
		selectReactSelectOption(page, valueToSelect, labelText)
	}
}

func handleReactSelectByPlaceholder(page playwright.Page, placeholder playwright.Locator, index int, userData *UserProfileData) {
	// Get the parent select container
	selectContainer := placeholder.Locator("xpath=ancestor::div[contains(@class, 'select__container')]").First()
	if count, _ := selectContainer.Count(); count == 0 {
		return
	}
	
	// Get label
	label := selectContainer.Locator("xpath=preceding::label[1]").First()
	labelText := ""
	if count, _ := label.Count(); count > 0 {
		labelText, _ = label.TextContent()
	}
	
	log.Printf("React Select by placeholder %d: %s", index, labelText)
	
	// Determine value
	valueToSelect := determineValueForField(labelText, userData)
	if valueToSelect == "" {
		return
	}
	
	// Click the placeholder or its parent to open
	if err := placeholder.Click(); err != nil {
		// Try clicking parent
		parent := placeholder.Locator("xpath=parent::div")
		if err := parent.Click(); err != nil {
			log.Printf("  Failed to open dropdown: %v", err)
			return
		}
	}
	
	page.WaitForTimeout(500)
	selectReactSelectOption(page, valueToSelect, labelText)
}

func handleReactSelectByCombobox(page playwright.Page, combobox playwright.Locator, index int, userData *UserProfileData) {
	// Check if visible
	isVisible, _ := combobox.IsVisible()
	if !isVisible {
		return
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
	
	// Check current value
	currentValue, _ := combobox.InputValue()
	if currentValue != "" {
		log.Printf("Combobox %d already has value: %s", index, currentValue)
		return
	}
	
	log.Printf("Combobox %d: %s", index, labelText)
	
	// Determine value
	valueToSelect := determineValueForField(labelText, userData)
	if valueToSelect == "" {
		return
	}
	
	// Click to focus and open
	if err := combobox.Click(); err != nil {
		log.Printf("  Failed to click combobox: %v", err)
		return
	}
	
	page.WaitForTimeout(500)
	
	// Type the value (some React Selects allow typing)
	if err := combobox.Type(valueToSelect); err != nil {
		log.Printf("  Failed to type value: %v", err)
	}
	
	page.WaitForTimeout(300)
	
	// Try to select from options that appear
	selectReactSelectOption(page, valueToSelect, labelText)
}

func selectReactSelectOption(page playwright.Page, valueToSelect string, fieldLabel string) {
	log.Printf("  Trying to select: %s", valueToSelect)
	
	// Try various selectors for React Select options
	optionSelectors := []string{
		// Common React Select option selectors
		fmt.Sprintf("div[class*='option']:has-text('%s')", valueToSelect),
		fmt.Sprintf("div[id*='option']:has-text('%s')", valueToSelect),
		fmt.Sprintf("div[role='option']:has-text('%s')", valueToSelect),
		// Exact text match
		fmt.Sprintf("div:text-is('%s')", valueToSelect),
		// Partial match
		fmt.Sprintf("div[class*='menu'] div:has-text('%s')", valueToSelect),
	}
	
	for _, selector := range optionSelectors {
		options := page.Locator(selector)
		if count, _ := options.Count(); count > 0 {
			// Click the first matching option
			if err := options.First().Click(); err == nil {
				log.Printf("  ✓ Selected '%s' for %s", valueToSelect, fieldLabel)
				return
			}
		}
	}
	
	// If exact match fails, try partial matches for certain fields
	if strings.Contains(strings.ToLower(fieldLabel), "country") {
		// For country, try "United States" variations
		alternatives := []string{"United States", "USA", "US", "United States of America"}
		for _, alt := range alternatives {
			option := page.Locator(fmt.Sprintf("div[role='option']:has-text('%s'), div[class*='option']:has-text('%s')", alt, alt))
			if count, _ := option.Count(); count > 0 {
				if err := option.First().Click(); err == nil {
					log.Printf("  ✓ Selected '%s' for country field", alt)
					return
				}
			}
		}
	}
	
	// Last resort: click the first non-disabled option
	firstOption := page.Locator("div[role='option']:visible, div[class*='option']:visible").First()
	if count, _ := firstOption.Count(); count > 0 {
		text, _ := firstOption.TextContent()
		if err := firstOption.Click(); err == nil {
			log.Printf("  ✓ Selected first available option: '%s'", text)
			return
		}
	}
	
	log.Printf("  ✗ Could not select any option for: %s", fieldLabel)
}

func determineValueForField(labelText string, userData *UserProfileData) string {
	labelLower := strings.ToLower(labelText)
	
	// Country selection - use user's actual country
	if strings.Contains(labelLower, "country") || strings.Contains(labelLower, "location") {
		if userData.Country != "" {
			// User's actual country
			return userData.Country
		}
		// Default fallback
		return "United States"
	}
	
	// Work authorization - use user's actual status
	if strings.Contains(labelLower, "authorized") && strings.Contains(labelLower, "work") {
		if userData.WorkAuthorization == "yes" {
			return "Yes"
		} else if userData.WorkAuthorization == "no" {
			return "No"
		} else if userData.WorkAuthorization == "requires_sponsorship" {
			return "No"
		}
		// Default to Yes if not specified
		return "Yes"
	}
	
	// Sponsorship - based on user's work authorization
	if strings.Contains(labelLower, "sponsor") || strings.Contains(labelLower, "work permit") {
		if userData.RequiresSponsorship {
			return "Yes"
		}
		return "No"
	}
	
	// Remote work - use user's preference
	if strings.Contains(labelLower, "remote") {
		if userData.RemoteWorkPreference == "yes" || userData.RemoteWorkPreference == "remote" {
			return "Yes"
		} else if userData.RemoteWorkPreference == "no" || userData.RemoteWorkPreference == "office" {
			return "No"
		} else if userData.RemoteWorkPreference == "hybrid" {
			return "Hybrid"
		}
		// Default
		return "Yes"
	}
	
	// Previous employment - this should be based on user's history with the specific company
	if strings.Contains(labelLower, "employed by") || strings.Contains(labelLower, "ever been employed") {
		// For now default to No, but this should check user's work history
		// TODO: Check if userData.Experience contains this company
		return "No"
	}
	
	// WhatsApp - could be a user preference
	if strings.Contains(labelLower, "whatsapp") {
		// This could be a user setting, for now default to Yes
		return "Yes"
	}
	
	// Demographic questions - use user's actual data or privacy option
	if strings.Contains(labelLower, "gender") {
		if userData.Gender != "" && userData.Gender != "prefer_not_to_say" {
			// Map common gender values
			switch strings.ToLower(userData.Gender) {
			case "male", "m":
				return "Man"
			case "female", "f":
				return "Woman"
			case "other":
				return "Non-binary"
			default:
				return userData.Gender
			}
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(labelLower, "racial") || strings.Contains(labelLower, "ethnic") {
		if userData.Ethnicity != "" && userData.Ethnicity != "prefer_not_to_say" {
			return userData.Ethnicity
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(labelLower, "sexual orientation") {
		// This field is not in UserProfileData, so use privacy option
		return "Prefer not to answer"
	}
	
	if strings.Contains(labelLower, "transgender") {
		// Use privacy option unless user has specified
		return "Prefer not to answer"
	}
	
	if strings.Contains(labelLower, "disability") || strings.Contains(labelLower, "chronic") {
		if userData.DisabilityStatus == "yes" {
			return "Yes"
		} else if userData.DisabilityStatus == "no" {
			return "No"
		}
		return "Prefer not to answer"
	}
	
	if strings.Contains(labelLower, "veteran") || strings.Contains(labelLower, "armed forces") {
		if userData.VeteranStatus == "yes" {
			return "Yes, I am a protected veteran"
		} else if userData.VeteranStatus == "no" {
			return "I am not a protected veteran"
		}
		return "Prefer not to answer"
	}
	
	return ""
}