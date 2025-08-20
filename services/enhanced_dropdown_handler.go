package services

import (
	"fmt"
	"log"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// EnhancedDropdownHandler handles all types of dropdowns including regular form fields
func EnhancedDropdownHandler(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Enhanced Dropdown Handler ===")
	
	// Find all select elements including those that might not be visible immediately
	selects, err := page.Locator("select").All()
	if err != nil {
		return fmt.Errorf("failed to find select elements: %v", err)
	}
	
	log.Printf("Found %d total select elements", len(selects))
	
	for i, selectElem := range selects {
		// Skip if not visible
		isVisible, _ := selectElem.IsVisible()
		if !isVisible {
			continue
		}
		
		// Get dropdown information
		selectId, _ := selectElem.GetAttribute("id")
		selectName, _ := selectElem.GetAttribute("name")
		ariaLabel, _ := selectElem.GetAttribute("aria-label")
		
		// Try to find label text multiple ways
		var labelText string
		
		// Method 1: Direct label for this select
		if selectId != "" {
			label := page.Locator(fmt.Sprintf("label[for='%s']", selectId)).First()
			if count, _ := label.Count(); count > 0 {
				labelText, _ = label.TextContent()
			}
		}
		
		// Method 2: Parent label
		if labelText == "" {
			parentLabel := selectElem.Locator("xpath=ancestor::label").First()
			if count, _ := parentLabel.Count(); count > 0 {
				labelText, _ = parentLabel.TextContent()
			}
		}
		
		// Method 3: Preceding label or text
		if labelText == "" {
			precedingText := selectElem.Locator("xpath=preceding::*[contains(text(), '?') or contains(text(), ':')][1]").First()
			if count, _ := precedingText.Count(); count > 0 {
				labelText, _ = precedingText.TextContent()
			}
		}
		
		// Method 4: Parent container text
		if labelText == "" {
			parent := selectElem.Locator("xpath=ancestor::div[1]").First()
			if count, _ := parent.Count(); count > 0 {
				labelText, _ = parent.TextContent()
			}
		}
		
		// Clean up label text
		labelText = strings.TrimSpace(labelText)
		if len(labelText) > 200 {
			labelText = labelText[:200] // Truncate very long text
		}
		
		// Build field info for matching
		fieldInfo := strings.ToLower(labelText + " " + selectName + " " + selectId + " " + ariaLabel)
		
		// Check current value
		currentValue, _ := selectElem.InputValue()
		
		// Skip if already has a valid value
		if currentValue != "" && currentValue != "0" && currentValue != "-1" && 
		   !strings.Contains(strings.ToLower(currentValue), "select") &&
		   !strings.Contains(strings.ToLower(currentValue), "please") &&
		   !strings.Contains(strings.ToLower(currentValue), "choose") {
			log.Printf("Dropdown %d already has value: %s", i, currentValue)
			continue
		}
		
		log.Printf("Processing dropdown %d: label='%s', name='%s', id='%s'", i, labelText, selectName, selectId)
		
		// Get all options
		options, _ := selectElem.Locator("option").All()
		if len(options) == 0 {
			log.Printf("  No options found for dropdown %d", i)
			continue
		}
		
		log.Printf("  Found %d options", len(options))
		
		// Determine what value to select based on field context
		valueToSelect := determineDropdownValue(fieldInfo, labelText, userData, options)
		
		if valueToSelect != "" {
			// Try to select the determined value
			_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{valueToSelect}})
			if err == nil {
				log.Printf("  ✓ Selected value '%s' for dropdown: %s", valueToSelect, selectName)
			} else {
				log.Printf("  ✗ Failed to select value '%s': %v", valueToSelect, err)
				// Try fallback to any non-empty option
				selectFallbackOption(selectElem, options)
			}
		} else {
			// No specific value determined, try fallback
			selectFallbackOption(selectElem, options)
		}
	}
	
	return nil
}

func determineDropdownValue(fieldInfo, labelText string, userData *UserProfileData, options []playwright.Locator) string {
	fieldLower := strings.ToLower(fieldInfo)
	
	// Country selection
	if strings.Contains(fieldLower, "country") || strings.Contains(fieldLower, "countries") {
		// Default to USA
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			if strings.Contains(strings.ToLower(text), "usa") || 
			   strings.Contains(strings.ToLower(text), "united states") ||
			   strings.Contains(strings.ToLower(text), "us") {
				return value
			}
		}
		return ""
	}
	
	// Work authorization
	if strings.Contains(fieldLower, "authorized to work") || strings.Contains(fieldLower, "work authorization") {
		// Look for "Yes" option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "yes" || textLower == "y" {
				return value
			}
		}
		return ""
	}
	
	// Sponsorship requirement
	if strings.Contains(fieldLower, "sponsor") || strings.Contains(fieldLower, "work permit") {
		// Look for "No" option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "no" || textLower == "n" {
				return value
			}
		}
		return ""
	}
	
	// Remote work preference
	if strings.Contains(fieldLower, "remote") || strings.Contains(fieldLower, "work from") {
		// Look for "Yes" or "Remote" option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "yes" || textLower == "remote" || strings.Contains(textLower, "remote") {
				return value
			}
		}
		// Fallback to "No" if no remote option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "no" || textLower == "office" {
				return value
			}
		}
		return ""
	}
	
	// Previous employment at company
	if strings.Contains(fieldLower, "employed by") || strings.Contains(fieldLower, "previous employ") {
		// Look for "No" option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "no" || textLower == "n" || strings.Contains(textLower, "never") {
				return value
			}
		}
		return ""
	}
	
	// WhatsApp opt-in
	if strings.Contains(fieldLower, "whatsapp") || strings.Contains(fieldLower, "opt-in") {
		// Look for "Yes" option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if textLower == "yes" || textLower == "y" {
				return value
			}
		}
		return ""
	}
	
	// Demographic questions - privacy-preserving options
	if strings.Contains(fieldLower, "gender") || 
	   strings.Contains(fieldLower, "race") || 
	   strings.Contains(fieldLower, "ethnic") ||
	   strings.Contains(fieldLower, "sexual") ||
	   strings.Contains(fieldLower, "orientation") {
		// Look for privacy options
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if strings.Contains(textLower, "prefer not") ||
			   strings.Contains(textLower, "decline") ||
			   strings.Contains(textLower, "not to answer") ||
			   strings.Contains(textLower, "not say") ||
			   strings.Contains(textLower, "not disclose") {
				return value
			}
		}
		return ""
	}
	
	// Transgender question
	if strings.Contains(fieldLower, "transgender") {
		// Look for "No" or privacy option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if strings.Contains(textLower, "prefer not") || textLower == "no" {
				return value
			}
		}
		return ""
	}
	
	// Disability question
	if strings.Contains(fieldLower, "disability") || strings.Contains(fieldLower, "chronic condition") {
		// Look for "No" or privacy option
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if strings.Contains(textLower, "prefer not") || textLower == "no" {
				return value
			}
		}
		return ""
	}
	
	// Veteran status
	if strings.Contains(fieldLower, "veteran") || strings.Contains(fieldLower, "armed forces") {
		// Look for "not a protected veteran" or "No"
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			textLower := strings.ToLower(strings.TrimSpace(text))
			if strings.Contains(textLower, "not a protected") || 
			   strings.Contains(textLower, "prefer not") ||
			   textLower == "no" {
				return value
			}
		}
		return ""
	}
	
	return ""
}

func selectFallbackOption(selectElem playwright.Locator, options []playwright.Locator) {
	// Try to select any valid non-placeholder option
	for i, opt := range options {
		if i == 0 {
			// Skip first option as it's usually placeholder
			text, _ := opt.TextContent()
			textLower := strings.ToLower(strings.TrimSpace(text))
			if strings.Contains(textLower, "select") || 
			   strings.Contains(textLower, "choose") ||
			   strings.Contains(textLower, "please") ||
			   textLower == "" {
				continue
			}
		}
		
		value, _ := opt.GetAttribute("value")
		if value != "" && value != "0" && value != "-1" {
			text, _ := opt.TextContent()
			_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
			if err == nil {
				log.Printf("  ✓ Selected fallback option: '%s'", text)
				return
			}
		}
	}
	
	log.Printf("  ⚠ Could not select any option")
}