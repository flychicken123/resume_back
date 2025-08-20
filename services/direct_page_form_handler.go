package services

import (
	"fmt"
	"log"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// handleDirectPageForm handles forms that are directly on the page (not in iframe)
func (s *BrowserAutomationServiceV2) handleDirectPageForm(page playwright.Page, userData *UserProfileData, resumeFilePath string, result *AutomationResult) (*AutomationResult, error) {
	log.Println("=== Handling form directly on page ===")
	
	// Debug what's on the page
	log.Println("Debugging page content...")
	
	// Check ALL inputs (not just visible)
	allInputs, _ := page.Locator("input").All()
	log.Printf("Total inputs on page (all types): %d", len(allInputs))
	
	// Check visible inputs
	visibleInputs, _ := page.Locator("input:visible").All()
	log.Printf("Visible inputs: %d", len(visibleInputs))
	
	// Log first few inputs for debugging
	for i, input := range allInputs {
		if i >= 5 {
			break
		}
		inputType, _ := input.GetAttribute("type")
		name, _ := input.GetAttribute("name")
		id, _ := input.GetAttribute("id")
		className, _ := input.GetAttribute("class")
		isVisible, _ := input.IsVisible()
		log.Printf("  Input %d: type=%s, name=%s, id=%s, class=%s, visible=%v", 
			i, inputType, name, id, className, isVisible)
	}
	
	// Check for any divs
	allDivs, _ := page.Locator("div").All()
	log.Printf("Total divs on page: %d", len(allDivs))
	
	// Check page URL and title
	currentURL := page.URL()
	title, _ := page.Title()
	log.Printf("Current URL: %s", currentURL)
	log.Printf("Page title: %s", title)
	
	// Check if page has loaded
	pageState, _ := page.Evaluate(`() => document.readyState`)
	log.Printf("Page ready state: %v", pageState)
	
	// Check for any form elements
	forms, _ := page.Locator("form").All()
	log.Printf("Forms on page: %d", len(forms))
	
	// Check page body text (first 500 chars)
	bodyText, _ := page.Locator("body").TextContent()
	if len(bodyText) > 500 {
		bodyText = bodyText[:500] + "..."
	}
	log.Printf("Page body text (first 500 chars): %s", bodyText)
	
	filledCount := 0
	
	// Fill text inputs
	log.Println("Looking for text inputs...")
	inputs, _ := page.Locator("input[type='text']:visible, input[type='email']:visible, input[type='tel']:visible").All()
	log.Printf("Found %d text/email/tel inputs", len(inputs))
	
	for i, input := range inputs {
		name, _ := input.GetAttribute("name")
		placeholder, _ := input.GetAttribute("placeholder")
		ariaLabel, _ := input.GetAttribute("aria-label")
		id, _ := input.GetAttribute("id")
		
		fieldInfo := strings.ToLower(name + " " + placeholder + " " + ariaLabel + " " + id)
		
		var value string
		if strings.Contains(fieldInfo, "first") && strings.Contains(fieldInfo, "name") {
			value = userData.FirstName
		} else if strings.Contains(fieldInfo, "last") && strings.Contains(fieldInfo, "name") {
			value = userData.LastName
		} else if strings.Contains(fieldInfo, "email") {
			value = userData.Email
		} else if strings.Contains(fieldInfo, "phone") {
			value = userData.Phone
		} else if strings.Contains(fieldInfo, "linkedin") {
			value = userData.LinkedIn
		}
		
		if value != "" {
			log.Printf("  Input %d: Filling '%s' with: %s", i, fieldInfo, value)
			if err := input.Fill(value); err == nil {
				filledCount++
			} else {
				log.Printf("  Failed to fill: %v", err)
			}
		}
	}
	
	// Handle resume upload
	if resumeFilePath != "" {
		log.Println("Looking for file upload...")
		fileInputs, _ := page.Locator("input[type='file']").All()
		log.Printf("Found %d file inputs", len(fileInputs))
		
		for _, fileInput := range fileInputs {
			accept, _ := fileInput.GetAttribute("accept")
			name, _ := fileInput.GetAttribute("name")
			
			if strings.Contains(accept, "pdf") || strings.Contains(accept, "doc") || 
			   strings.Contains(strings.ToLower(name), "resume") {
				log.Printf("  Uploading resume to file input")
				if err := fileInput.SetInputFiles(resumeFilePath); err == nil {
					filledCount++
					log.Printf("  ✓ Resume uploaded")
				} else {
					log.Printf("  Failed to upload: %v", err)
				}
				break
			}
		}
	}
	
	// Handle dropdowns - using comprehensive handler adapted for page
	log.Println("Looking for dropdowns on page...")
	if err := HandlePageDropdownsComprehensive(page, userData); err != nil {
		log.Printf("Some dropdowns could not be filled: %v", err)
	}
	
	// Take screenshot before submit
	screenshotURL, _ := s.screenshotService.SaveScreenshotToResult(page, "before_submit", result)
	if screenshotURL != "" {
		log.Printf("Screenshot taken: %s", screenshotURL)
		result.ApplicationScreenshotKey = screenshotURL
	}
	
	// Find and click submit button
	log.Println("Looking for submit button...")
	if s.findAndClickPageSubmitButton(page) {
		log.Println("Submit button clicked!")
		result.Success = true
		result.Status = "submitted"
		result.Message = "Application submitted"
		
		// Take confirmation screenshot
		confirmURL, _ := s.screenshotService.SaveScreenshotToResult(page, "confirmation", result)
		if confirmURL != "" {
			result.ConfirmationScreenshotKey = confirmURL
		}
	} else {
		log.Println("Warning: Could not find submit button")
		result.Status = "submit_button_not_found"
		result.Message = "Application filled but submit button not found"
	}
	
	result.FilledFields["fields"] = fmt.Sprintf("%d", filledCount)
	return result, nil
}

// findAndClickPageSubmitButton finds and clicks submit button on page
func (s *BrowserAutomationServiceV2) findAndClickPageSubmitButton(page playwright.Page) bool {
	submitSelectors := []string{
		"button[type='submit']:visible",
		"input[type='submit']:visible",
		"button:has-text('Submit application'):visible",
		"button:has-text('Submit Application'):visible",
		"button:has-text('Submit'):visible",
		"button:has-text('Apply'):visible",
		"button:has-text('Send'):visible",
		"button:has-text('Continue'):visible",
		"button[class*='submit']:visible",
		"button[aria-label*='Submit']:visible",
	}
	
	for _, selector := range submitSelectors {
		btn := page.Locator(selector).First()
		if count, _ := btn.Count(); count > 0 {
			// Check if enabled
			disabled, _ := btn.IsDisabled()
			if !disabled {
				// Scroll into view
				btn.ScrollIntoViewIfNeeded()
				
				if err := btn.Click(playwright.LocatorClickOptions{
					Force: playwright.Bool(true),
				}); err == nil {
					return true
				}
			}
		}
	}
	
	// Try JavaScript click as backup
	result, _ := page.Evaluate(`
		() => {
			const buttons = document.querySelectorAll('button, input[type="submit"]');
			for (const btn of buttons) {
				const text = (btn.innerText || btn.value || '').toLowerCase();
				if (text.includes('submit') || text.includes('apply')) {
					if (!btn.disabled) {
						btn.click();
						return true;
					}
				}
			}
			return false;
		}
	`)
	
	return result == true
}

// HandlePageDropdownsComprehensive handles dropdowns directly on page
func HandlePageDropdownsComprehensive(page playwright.Page, userData *UserProfileData) error {
	log.Println("=== Handling dropdowns on page ===")
	
	processedQuestions := make(map[string]bool)
	filledCount := 0
	
	// Find HTML select elements
	selects, _ := page.Locator("select:visible").All()
	log.Printf("Found %d select elements", len(selects))
	
	for i, sel := range selects {
		// Get current value
		currentVal, _ := sel.InputValue()
		if currentVal != "" && currentVal != "0" && !strings.Contains(strings.ToLower(currentVal), "select") {
			continue
		}
		
		// Get question text
		question := getQuestionForPageElement(page, sel)
		if processedQuestions[question] {
			continue
		}
		processedQuestions[question] = true
		
		log.Printf("Select %d: %s", i+1, question)
		
		// Determine answer
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			continue
		}
		
		log.Printf("  Filling with: %s", answer)
		
		// Fill the select
		options, _ := sel.Locator("option").All()
		for _, opt := range options {
			text, _ := opt.TextContent()
			if matchesAnswerComprehensive(strings.TrimSpace(text), answer) {
				value, _ := opt.GetAttribute("value")
				sel.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{value},
				})
				filledCount++
				log.Printf("  ✓ Filled successfully")
				break
			}
		}
	}
	
	// Find div-based dropdowns
	log.Println("Looking for div dropdowns...")
	
	// Multiple strategies to find dropdowns
	dropdownDivs := []playwright.Locator{}
	
	// Strategy 1: Divs with "Select..." text
	divs1, _ := page.Locator("div:has-text('Select...'):visible").All()
	dropdownDivs = append(dropdownDivs, divs1...)
	
	// Strategy 2: Divs with select-related classes
	divs2, _ := page.Locator("div[class*='select']:visible").All()
	for _, div := range divs2 {
		// Check if it's a dropdown (not too large)
		box, _ := div.BoundingBox()
		if box != nil && box.Height < 100 && box.Width < 600 {
			dropdownDivs = append(dropdownDivs, div)
		}
	}
	
	log.Printf("Found %d potential div dropdowns", len(dropdownDivs))
	
	for i, div := range dropdownDivs {
		if i >= 50 { // Limit to prevent too many
			break
		}
		
		question := getQuestionForPageElement(page, div)
		if question == "" || processedQuestions[question] {
			continue
		}
		processedQuestions[question] = true
		
		log.Printf("Div dropdown %d: %s", i+1, question)
		
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			continue
		}
		
		log.Printf("  Filling with: %s", answer)
		
		// Click dropdown
		if err := div.Click(playwright.LocatorClickOptions{
			Force: playwright.Bool(true),
		}); err == nil {
			// Look for options
			fillPageDropdownOption(page, answer)
			filledCount++
		}
	}
	
	log.Printf("Filled %d dropdowns total", filledCount)
	return nil
}

func getQuestionForPageElement(page playwright.Page, elem playwright.Locator) string {
	// Try to find associated label or text
	questionText, _ := elem.Evaluate(`
		el => {
			// Look for label
			const id = el.id;
			if (id) {
				const label = document.querySelector('label[for="' + id + '"]');
				if (label) return label.textContent.trim();
			}
			
			// Look in parent for label
			let parent = el.parentElement;
			let depth = 0;
			while (parent && depth < 5) {
				const label = parent.querySelector('label');
				if (label && label.textContent) {
					return label.textContent.trim();
				}
				
				// Check for text with *
				const texts = parent.querySelectorAll('span, div, p');
				for (const t of texts) {
					if (t.textContent && t.textContent.includes('*') && !t.contains(el)) {
						if (t.textContent.length < 200) {
							return t.textContent.trim();
						}
					}
				}
				
				parent = parent.parentElement;
				depth++;
			}
			
			// Check preceding sibling
			let sibling = el.previousElementSibling;
			while (sibling) {
				if (sibling.textContent && sibling.textContent.trim()) {
					const text = sibling.textContent.trim();
					if (text.length > 3 && text.length < 200) {
						return text;
					}
				}
				sibling = sibling.previousElementSibling;
			}
			
			return '';
		}
	`, nil)
	
	if questionText != nil {
		return strings.TrimSpace(questionText.(string))
	}
	return ""
}

func fillPageDropdownOption(page playwright.Page, answer string) {
	// Try to find and click the option
	variations := []string{answer}
	if answer == "United States" {
		variations = append(variations, "US", "USA", "United States of America")
	} else if answer == "Yes" {
		variations = append(variations, "YES")
	} else if answer == "No" {
		variations = append(variations, "NO")
	}
	
	for _, variant := range variations {
		selectors := []string{
			fmt.Sprintf("div[role='option']:text-is('%s'):visible", variant),
			fmt.Sprintf("li:text-is('%s'):visible", variant),
			fmt.Sprintf("*[role='option']:text-is('%s'):visible", variant),
		}
		
		for _, selector := range selectors {
			opt := page.Locator(selector).First()
			if count, _ := opt.Count(); count > 0 {
				opt.Click(playwright.LocatorClickOptions{
					Force: playwright.Bool(true),
				})
				return
			}
		}
	}
	
	// Try partial match
	opt := page.Locator(fmt.Sprintf("div[role='option']:has-text('%s'):visible", answer)).First()
	if count, _ := opt.Count(); count > 0 {
		opt.Click(playwright.LocatorClickOptions{
			Force: playwright.Bool(true),
		})
	}
}