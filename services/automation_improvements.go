package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// ImprovedDropdownSelection handles dropdown selection with better option matching
func ImprovedDropdownSelection(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Starting Improved Dropdown Selection ===")
	
	// Find all select elements
	selects, err := page.Locator("select:visible").All()
	if err != nil {
		return fmt.Errorf("failed to find select elements: %v", err)
	}
	
	log.Printf("Found %d visible select elements", len(selects))
	
	for i, selectElem := range selects {
		// Get dropdown information
		selectId, _ := selectElem.GetAttribute("id")
		selectName, _ := selectElem.GetAttribute("name")
		
		// Find label
		var labelText string
		if selectId != "" {
			label := page.Locator(fmt.Sprintf("label[for='%s']", selectId)).First()
			if label != nil {
				labelText, _ = label.TextContent()
			}
		}
		
		// Clean label text
		labelText = strings.TrimSpace(labelText)
		fieldInfo := strings.ToLower(labelText + " " + selectName + " " + selectId)
		
		// Check current value
		currentValue, _ := selectElem.InputValue()
		
		// Skip if already has a non-empty, non-placeholder value
		if currentValue != "" && currentValue != "0" && currentValue != "-1" &&
			!strings.Contains(strings.ToLower(currentValue), "select") {
			log.Printf("Dropdown %d already has value: %s", i, currentValue)
			continue
		}
		
		log.Printf("Processing dropdown %d: label='%s', name='%s'", i, labelText, selectName)
		
		// Get all options
		options, _ := selectElem.Locator("option").All()
		log.Printf("  Available options: %d", len(options))
		
		// List all options for debugging
		var optionsList []string
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			optionsList = append(optionsList, fmt.Sprintf("'%s' (value=%s)", strings.TrimSpace(text), value))
		}
		log.Printf("  Options: %v", optionsList)
		
		// Try to select appropriate option
		selected := false
		
		// For demographic questions, look for privacy-preserving options
		if strings.Contains(fieldInfo, "gender") || 
		   strings.Contains(fieldInfo, "race") || 
		   strings.Contains(fieldInfo, "ethnic") ||
		   strings.Contains(fieldInfo, "sexual") ||
		   strings.Contains(fieldInfo, "transgender") ||
		   strings.Contains(fieldInfo, "veteran") ||
		   strings.Contains(fieldInfo, "disability") {
			
			// Try to find and select privacy option
			for _, opt := range options {
				text, _ := opt.TextContent()
				value, _ := opt.GetAttribute("value")
				textLower := strings.ToLower(strings.TrimSpace(text))
				
				// Check for privacy-preserving options
				if strings.Contains(textLower, "prefer not") ||
				   strings.Contains(textLower, "decline") ||
				   strings.Contains(textLower, "not to answer") ||
				   strings.Contains(textLower, "not say") ||
				   strings.Contains(textLower, "not disclose") ||
				   strings.Contains(textLower, "choose not") ||
				   (strings.Contains(fieldInfo, "veteran") && strings.Contains(textLower, "not a protected")) ||
				   (value != "" && value != "0" && len(options) > 2 && strings.TrimSpace(text) == "") { // Empty option that's not first
					
					log.Printf("  Attempting to select: '%s'", text)
					// Use SelectOption like in browser_automation_service.go
					_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
					if err == nil {
						log.Printf("  ✓ Selected privacy option: '%s'", text)
						selected = true
						break
					}
				}
			}
			
			// If no privacy option found, try "No" for yes/no questions
			if !selected && (strings.Contains(fieldInfo, "transgender") || 
			                 strings.Contains(fieldInfo, "disability") ||
			                 strings.Contains(fieldInfo, "veteran")) {
				for _, opt := range options {
					text, _ := opt.TextContent()
					value, _ := opt.GetAttribute("value")
					textLower := strings.ToLower(strings.TrimSpace(text))
					
					if textLower == "no" || textLower == "n" {
						_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
						if err == nil {
							log.Printf("  ✓ Selected 'No' option")
							selected = true
							break
						}
					}
				}
			}
		}
		
		if !selected && len(options) > 1 {
			// Select the second option (first is usually placeholder)
			if len(options) > 1 {
				value, _ := options[1].GetAttribute("value")
				text, _ := options[1].TextContent()
				if value != "" {
					_, err := selectElem.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
					if err == nil {
						log.Printf("  ✓ Selected second option as fallback: '%s'", text)
						selected = true
					}
				}
			}
		}
		
		if !selected {
			log.Printf("  ⚠ Could not select any option for dropdown: %s", labelText)
		}
	}
	
	return nil
}

// EnsureResumeUpload makes sure resume is uploaded with multiple fallback strategies
func EnsureResumeUpload(page playwright.Page, resumeFilePath string) error {
	if resumeFilePath == "" {
		return fmt.Errorf("no resume file path provided")
	}
	
	log.Printf("=== Ensuring Resume Upload: %s ===", resumeFilePath)
	
	// Strategy 1: Find all file inputs and try each one
	fileInputs, _ := page.Locator("input[type='file']").All()
	log.Printf("Found %d file input elements", len(fileInputs))
	
	for i, input := range fileInputs {
		// Check if this looks like a resume upload
		name, _ := input.GetAttribute("name")
		id, _ := input.GetAttribute("id")
		accept, _ := input.GetAttribute("accept")
		
		log.Printf("File input %d: name='%s', id='%s', accept='%s'", i, name, id, accept)
		
		// Check if it's for resume/CV
		fieldInfo := strings.ToLower(name + " " + id + " " + accept)
		isResumeField := strings.Contains(fieldInfo, "resume") ||
		                 strings.Contains(fieldInfo, "cv") ||
		                 strings.Contains(fieldInfo, "file") ||
		                 strings.Contains(fieldInfo, "document") ||
		                 strings.Contains(accept, "pdf")
		
		if !isResumeField && len(fileInputs) > 1 {
			log.Printf("  Skipping non-resume field")
			continue
		}
		
		// Try to upload
		log.Printf("  Attempting to upload resume to input %d", i)
		err := input.SetInputFiles(resumeFilePath)
		if err != nil {
			log.Printf("  Failed to upload: %v", err)
			continue
		}
		
		// Verify upload - simplified check
		time.Sleep(500 * time.Millisecond)
		log.Printf("  ✓ Successfully uploaded resume to input %d", i)
		return nil
	}
	
	// Strategy 2: Click on upload button/area and then set file
	uploadButtons, _ := page.Locator("button:has-text('upload'), div:has-text('upload'), label:has-text('upload'), button:has-text('choose'), div:has-text('choose file')").All()
	for _, button := range uploadButtons {
		text, _ := button.TextContent()
		log.Printf("Found upload button: %s", text)
		
		// Click it
		button.Click()
		time.Sleep(500 * time.Millisecond)
		
		// Try to find file input again
		fileInputs, _ = page.Locator("input[type='file']").All()
		for _, input := range fileInputs {
			err := input.SetInputFiles(resumeFilePath)
			if err == nil {
				log.Printf("✓ Successfully uploaded resume after clicking upload button")
				return nil
			}
		}
	}
	
	return fmt.Errorf("could not upload resume after trying all strategies")
}