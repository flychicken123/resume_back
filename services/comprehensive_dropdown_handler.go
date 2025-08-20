package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// detectCheckboxGroups detects checkbox groups and extracts their options
func detectCheckboxGroups(iframe playwright.FrameLocator) map[string][]string {
	groups := make(map[string][]string)
	
	// Set a very short timeout to prevent hanging
	startTime := time.Now()
	maxDuration := 500 * time.Millisecond // Reduced from 2 seconds
	
	// Only look for specific checkbox group patterns to avoid processing too many elements
	// Focus on fieldsets and known group patterns
	containers, _ := iframe.Locator("fieldset:has(input[type='checkbox'])").All()
	
	// Limit to first 5 containers to prevent hanging
	maxContainers := 5
	if len(containers) > maxContainers {
		log.Printf("Found %d containers, limiting to first %d", len(containers), maxContainers)
		containers = containers[:maxContainers]
	}
	
	for _, container := range containers {
		// Check timeout
		if time.Since(startTime) > maxDuration {
			log.Printf("Timeout reached while detecting checkbox groups")
			return groups
		}
		
		// Check if this container has multiple checkboxes
		checkboxes, _ := container.Locator("input[type='checkbox']").All()
		if len(checkboxes) < 2 {
			continue // Not a checkbox group
		}
		
		// Try to find the question/label for this group
		var groupQuestion string
		
		// Try legend tag (for fieldset)
		if legend, err := container.Locator("legend").First().TextContent(); err == nil && legend != "" {
			groupQuestion = strings.TrimSpace(legend)
		}
		
		// Try preceding label or heading
		if groupQuestion == "" {
			if label, err := container.Locator("label, h3, h4, h5, .question-text").First().TextContent(); err == nil {
				groupQuestion = strings.TrimSpace(label)
			}
		}
		
		// Extract options from the checkboxes (simplified for speed)
		var options []string
		for _, checkbox := range checkboxes {
			// Timeout check for each checkbox
			if time.Since(startTime) > maxDuration {
				break
			}
			
			// Try to get the label text for this checkbox
			var optionText string
			
			// First try to find associated label
			if id, _ := checkbox.GetAttribute("id"); id != "" {
				if label, err := container.Locator(fmt.Sprintf("label[for='%s']", id)).First().TextContent(); err == nil && label != "" {
					optionText = strings.TrimSpace(label)
				}
			}
			
			// If no label found, try to get text from parent or sibling
			if optionText == "" {
				// Try parent label
				if parent, err := checkbox.Locator("xpath=..").First().TextContent(); err == nil {
					optionText = strings.TrimSpace(parent)
				}
			}
			
			// Fallback to value attribute if no text found
			if optionText == "" {
				if value, _ := checkbox.GetAttribute("value"); value != "" {
					optionText = value
				}
			}
			
			if optionText != "" {
				options = append(options, optionText)
			}
		}
		
		// Store the group if we found a question and options
		if groupQuestion != "" && len(options) > 0 {
			groups[groupQuestion] = options
			log.Printf("Detected checkbox group: %s with options: %v", groupQuestion, options)
		}
	}
	
	// Skip the name-based grouping as it's too slow and not critical
	// The fieldset-based detection should be sufficient for most cases
	
	return groups
}

// HandleAllDropdownsComprehensive handles ALL dropdowns in iframe forms comprehensively
func HandleAllDropdownsComprehensive(iframe playwright.FrameLocator, userData *UserProfileData) error {
	log.Println("=== COMPREHENSIVE DROPDOWN HANDLING ===")
	
	// Set a deadline to prevent hanging forever
	startTime := time.Now()
	maxDuration := 3 * time.Second // Maximum 3 seconds for dropdown filling
	
	processedQuestions := make(map[string]bool)
	unknownQuestions := []string{}
	missingFieldsWithOptions := make(map[string][]string) // Store checkbox group options
	filledCount := 0
	
	// Strategy 1: Find all visible select elements
	log.Println("Looking for HTML select elements...")
	selects, _ := iframe.Locator("select:visible").All()
	for i, sel := range selects {
		// Check if already has value
		currentVal, _ := sel.InputValue()
		if currentVal != "" && currentVal != "0" && !strings.Contains(strings.ToLower(currentVal), "select") {
			continue
		}
		
		// Get question text
		question := getQuestionForElementComprehensive(iframe, sel, "select")
		if question == "" {
			question = fmt.Sprintf("Select field %d", i+1)
		}
		
		// Skip if already processed
		if processedQuestions[question] {
			continue
		}
		processedQuestions[question] = true
		
		log.Printf("Select dropdown %d: %s", i+1, question)
		
		// Determine answer
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			unknownQuestions = append(unknownQuestions, question)
			continue
		}
		
		// Fill the select
		log.Printf("  Filling with: %s", answer)
		if fillHTMLSelect(sel, answer) {
			filledCount++
			log.Printf("  ✓ Filled successfully")
		}
	}
	
	// Strategy 2: Find all div-based dropdowns (React Select, custom dropdowns)
	log.Println("Looking for div-based dropdowns...")
	
	// Look for elements containing "Select..." text OR elements with dropdown indicators
	// Try multiple strategies to find dropdowns
	dropdownDivs1, _ := iframe.Locator("div:has-text('Select...'):visible").All()
	dropdownDivs2, _ := iframe.Locator("div[class*='select']:visible").All()
	dropdownDivs3, _ := iframe.Locator("div[class*='dropdown']:visible").All()
	dropdownDivs4, _ := iframe.Locator("div[role='combobox']:visible").All()
	
	// Combine all found dropdowns
	var dropdownDivs []playwright.Locator
	dropdownDivs = append(dropdownDivs, dropdownDivs1...)
	dropdownDivs = append(dropdownDivs, dropdownDivs2...)
	dropdownDivs = append(dropdownDivs, dropdownDivs3...)
	dropdownDivs = append(dropdownDivs, dropdownDivs4...)
	
	log.Printf("Found %d potential dropdown divs (Select: %d, class*select: %d, class*dropdown: %d, role=combobox: %d)",
		len(dropdownDivs), len(dropdownDivs1), len(dropdownDivs2), len(dropdownDivs3), len(dropdownDivs4))
	
	// Limit to prevent hanging - BE AGGRESSIVE
	maxDropdowns := 15 // Reduced from 20
	processedCount := 0
	maxChecks := 20 // Much more aggressive limit - stop after checking 20 divs
	consecutiveSkips := 0 // Track consecutive skips to break early
	
	for i, div := range dropdownDivs {
		// Log progress every 5 divs
		if i > 0 && i%5 == 0 {
			log.Printf("Progress: Checking div %d/%d (processed %d dropdowns so far)", i, len(dropdownDivs), processedCount)
		}
		
		// Check timeout
		if time.Since(startTime) > maxDuration {
			log.Printf("TIMEOUT: Dropdown filling exceeded %v, stopping", maxDuration)
			break
		}
		
		// Safety check to prevent infinite loops - CHECK FIRST
		if i >= maxChecks {
			log.Printf("STOPPING: Reached max check limit (%d divs checked)", maxChecks)
			break
		}
		
		if processedCount >= maxDropdowns {
			log.Printf("STOPPING: Reached max dropdown limit (%d dropdowns processed)", maxDropdowns)
			break
		}
		
		// Check if it's actually a dropdown (not too large)
		box, err := div.BoundingBox()
		if err != nil {
			continue
		}
		if box != nil && box.Height > 100 {
			continue // Skip large containers
		}
		
		// Also skip if width is too large (likely a container)
		if box != nil && box.Width > 600 {
			continue
		}
		
		// Get question text
		question := getQuestionForElementComprehensive(iframe, div, "div")
		if question == "" {
			consecutiveSkips++
			// Break if we've had too many consecutive skips (no more real dropdowns)
			if consecutiveSkips > 10 {
				log.Printf("Breaking early - 10 consecutive elements with no question text")
				break
			}
			continue
		}
		
		// Reset consecutive skips counter since we found a valid question
		consecutiveSkips = 0
		
		// Skip if already processed
		if processedQuestions[question] {
			continue
		}
		processedQuestions[question] = true
		processedCount++
		
		log.Printf("Div dropdown %d: %s", processedCount+1, question)
		
		// Determine answer
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			unknownQuestions = append(unknownQuestions, question)
			continue
		}
		
		// Fill the div dropdown directly (no goroutine needed)
		log.Printf("  Filling with: %s", answer)
		if fillDivDropdown(iframe, div, answer) {
			filledCount++
			log.Printf("  ✓ Filled successfully")
		} else {
			log.Printf("  ⚠ Failed to fill dropdown")
		}
	}
	
	// Strategy 3: Find combobox inputs (React Select with role="combobox")
	log.Println("Looking for combobox elements...")
	comboboxes, _ := iframe.Locator("input[role='combobox']:visible").All()
	for i, combo := range comboboxes {
		// Check if already has value
		currentVal, _ := combo.InputValue()
		if currentVal != "" {
			continue
		}
		
		// Get question text
		question := getQuestionForElementComprehensive(iframe, combo, "combobox")
		if question == "" {
			continue
		}
		
		// Skip if already processed
		// Use normalized key for all checks
		normalizedKey := strings.TrimSpace(strings.ToLower(question))
		if processedQuestions[normalizedKey] {
			continue
		}
		
		log.Printf("Combobox %d: %s", i+1, question)
		
		// Check if this is a "mark all that apply" multi-select combobox
		questionLower := strings.ToLower(question)
		if strings.Contains(questionLower, "mark all that apply") {
			// This is a multi-select combobox - we need to extract the available options
			log.Printf("  Multi-select combobox detected (mark all that apply)")
			
			// Try to extract options from the dropdown
			// Click to open the dropdown
			combo.Click()
			time.Sleep(300 * time.Millisecond)
			
			// Look for dropdown options
			options := []string{}
			dropdownOptions, _ := iframe.Locator("div[role='option']:visible, li[role='option']:visible").All()
			for _, opt := range dropdownOptions {
				if text, err := opt.TextContent(); err == nil {
					text = strings.TrimSpace(text)
					if text != "" {
						options = append(options, text)
					}
				}
			}
			
			if len(options) > 0 {
				log.Printf("  Extracted %d options: %v", len(options), options)
				// Store for later use in missing fields
				missingFieldsWithOptions[question] = options
				
				// Try to fill if we have a saved answer
				answer := determineComprehensiveAnswer(question, userData)
				if answer != "" {
					log.Printf("  Found saved answer: %s", answer)
					// Try to select the matching option
					optionSelected := false
					for _, opt := range dropdownOptions {
						if text, err := opt.TextContent(); err == nil {
							text = strings.TrimSpace(text)
							if matchesAnswer(text, answer) {
								log.Printf("  Clicking matching option: %s", text)
								opt.Click()
								time.Sleep(100 * time.Millisecond)
								filledCount++
								processedQuestions[question] = true
								optionSelected = true
								break
							}
						}
					}
					// Press Escape to close after selection
					combo.Press("Escape")
					
					if optionSelected {
						log.Printf("  ✓ Multi-select filled successfully for: %s", question)
						// Mark with multiple variations to prevent duplicate detection
						normalizedKey := strings.TrimSpace(strings.ToLower(question))
						processedQuestions[question] = true
						processedQuestions[normalizedKey] = true
						// Also mark variations that might appear in checkbox groups
						if strings.Contains(normalizedKey, "racial") || strings.Contains(normalizedKey, "ethnic") {
							processedQuestions["racial/ethnic background"] = true
							processedQuestions["how would you describe your racial/ethnic background?"] = true
							processedQuestions["how would you describe your racial/ethnic background? (mark all that apply)"] = true
						}
						if strings.Contains(normalizedKey, "gender identity") {
							processedQuestions["gender identity"] = true
							processedQuestions["what is your gender identity?"] = true
							processedQuestions["what is your gender identity? (mark all that apply)"] = true
						}
						if strings.Contains(normalizedKey, "sexual orientation") {
							processedQuestions["sexual orientation"] = true
							processedQuestions["what is your sexual orientation?"] = true
							processedQuestions["what is your sexual orientation? (mark all that apply)"] = true
						}
						log.Printf("  Marked as processed: '%s' and normalized: '%s'", question, normalizedKey)
						continue
					} else {
						// Couldn't find matching option
						unknownQuestions = append(unknownQuestions, question)
					}
				} else {
					// Press Escape to close the dropdown
					combo.Press("Escape")
					// Add to unknown questions since we don't have an answer
					unknownQuestions = append(unknownQuestions, question)
				}
			} else {
				// Press Escape to close the dropdown
				combo.Press("Escape")
				unknownQuestions = append(unknownQuestions, question)
			}
			
			continue
		}
		
		// Determine answer for regular combobox
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			unknownQuestions = append(unknownQuestions, question)
			continue
		}
		
		// Fill the combobox
		log.Printf("  Filling with: %s", answer)
		if fillCombobox(iframe, combo, answer) {
			filledCount++
			log.Printf("  ✓ Filled successfully")
		}
	}
	
	// Strategy 4: Handle checkboxes and radio buttons
	log.Println("Looking for checkboxes and radio buttons...")
	
	// First, detect checkbox groups (like race/ethnicity with "mark all that apply")
	checkboxGroups := detectCheckboxGroups(iframe)
	for groupQuestion, options := range checkboxGroups {
		log.Printf("Found checkbox group: %s with %d options", groupQuestion, len(options))
		
		// Check if this is a field we need user input for
		questionLower := strings.ToLower(groupQuestion)
		
		// Handle country selection checkboxes
		if strings.Contains(questionLower, "country") || strings.Contains(questionLower, "countries") {
			// This is a country selection checkbox group
			log.Printf("  Country selection checkbox group detected with %d options", len(options))
			
			// Check if we have a saved answer for this question
			answer := determineComprehensiveAnswer(groupQuestion, userData)
			if answer != "" {
				log.Printf("  Found saved answer for country selection: %s", answer)
				// Try to find and check the matching checkbox
				// Find all checkboxes in fieldsets (checkbox groups)
				checkboxes, _ := iframe.Locator("fieldset input[type='checkbox']").All()
				
				foundMatch := false
				for _, checkbox := range checkboxes {
					// Get the label text for this checkbox
					if id, _ := checkbox.GetAttribute("id"); id != "" {
						if label, err := iframe.Locator(fmt.Sprintf("label[for='%s']", id)).First().TextContent(); err == nil {
							labelText := strings.TrimSpace(label)
							// Check for exact match or US/United States variations
							if labelText == answer || 
							   (answer == "US" && (labelText == "US" || labelText == "United States")) ||
							   (answer == "United States" && (labelText == "US" || labelText == "United States")) {
								log.Printf("  Checking checkbox for: %s", labelText)
								checkbox.Check()
								filledCount++
								processedQuestions[groupQuestion] = true
								foundMatch = true
								break
							}
						}
					}
				}
				
				if !foundMatch {
					// Couldn't find matching checkbox, add to missing fields
					missingFieldsWithOptions[groupQuestion] = options
					unknownQuestions = append(unknownQuestions, groupQuestion)
					processedQuestions[groupQuestion] = true
					log.Printf("  Could not find matching checkbox for saved answer, adding to missing fields")
				}
			} else {
				// No saved answer, add to missing fields so user can select
				missingFieldsWithOptions[groupQuestion] = options
				if !processedQuestions[groupQuestion] {
					unknownQuestions = append(unknownQuestions, groupQuestion)
					processedQuestions[groupQuestion] = true
				}
				log.Printf("  No saved answer, adding country selection to missing fields for user input")
			}
			continue
		}
		
		// Handle racial/ethnic background or other "mark all that apply" questions
		if strings.Contains(questionLower, "racial") || strings.Contains(questionLower, "ethnic") || 
		   strings.Contains(questionLower, "mark all that apply") {
			// Check if this was already filled as a multi-select combobox
			// Use normalized key for checking
			normalizedKey := strings.TrimSpace(strings.ToLower(groupQuestion))
			if processedQuestions[normalizedKey] {
				log.Printf("  Checkbox group already processed (normalized key match), skipping")
				continue
			}
			
			// Also check for similar questions that might have been processed with slight variations
			alreadyProcessed := false
			for processedQ := range processedQuestions {
				processedLower := strings.ToLower(processedQ)
				// Check if it's the same question with minor differences
				if (strings.Contains(processedLower, "racial") && strings.Contains(questionLower, "racial")) ||
				   (strings.Contains(processedLower, "ethnic") && strings.Contains(questionLower, "ethnic")) ||
				   (strings.Contains(processedLower, "gender identity") && strings.Contains(questionLower, "gender identity")) ||
				   (strings.Contains(processedLower, "sexual orientation") && strings.Contains(questionLower, "sexual orientation")) {
					log.Printf("  Similar question already processed: '%s' vs '%s'", processedQ, groupQuestion)
					alreadyProcessed = true
					break
				}
			}
			
			if alreadyProcessed {
				log.Printf("  Checkbox group already processed as multi-select combobox, skipping")
				continue
			}
			
			// IMPORTANT: Do NOT add to unknownQuestions if this is a racial/ethnic/gender/orientation field
			// These should have been filled already via multi-select combobox
			if strings.Contains(questionLower, "racial") || strings.Contains(questionLower, "ethnic") ||
			   strings.Contains(questionLower, "gender identity") || strings.Contains(questionLower, "sexual orientation") {
				log.Printf("  WARNING: Racial/ethnic/gender/orientation field not filled via multi-select. This should not happen.")
				log.Printf("  Skipping to avoid duplicate popup entry")
				// Mark as processed to prevent further attempts
				processedQuestions[groupQuestion] = true
				processedQuestions[normalizedKey] = true
				continue
			}
			
			// This is a checkbox group that needs user input
			// Store the options for later use when creating MissingFieldInfo
			missingFieldsWithOptions[groupQuestion] = options
			if !processedQuestions[groupQuestion] {
				unknownQuestions = append(unknownQuestions, groupQuestion)
				processedQuestions[groupQuestion] = true // Mark as processed to avoid duplicates
			}
			log.Printf("  Adding checkbox group to missing fields with options: %v", options)
		}
	}
	
	log.Printf("Finished processing %d checkbox groups", len(checkboxGroups))
	
	// Skip individual checkbox processing if we've already spent too much time
	if time.Since(startTime) > maxDuration {
		log.Printf("Time limit reached, skipping individual checkbox processing")
		// Continue to finish up instead of processing individual checkboxes
	} else {
		// Find all required checkboxes
		checkboxes, _ := iframe.Locator("input[type='checkbox'][required]:visible, input[type='checkbox'][aria-required='true']:visible").All()
		log.Printf("Found %d required checkboxes to process", len(checkboxes))
		
		// Build a map of all checkbox group options to skip individual processing
		groupOptions := make(map[string]bool)
		for _, options := range checkboxGroups {
			for _, option := range options {
				groupOptions[strings.ToLower(option)] = true
			}
		}
		
		// Limit processing time for checkboxes
		checkboxStartTime := time.Now()
		maxCheckboxDuration := 2 * time.Second
		
		for i, checkbox := range checkboxes {
			// Check if we've exceeded time limit
			if time.Since(checkboxStartTime) > maxCheckboxDuration {
				log.Printf("Timeout reached while processing checkboxes (processed %d/%d)", i, len(checkboxes))
				break
			}
		
		// Check if already checked
		isChecked, _ := checkbox.IsChecked()
		if isChecked {
			continue
		}
		
		// Get question text
		question := getQuestionForElementComprehensive(iframe, checkbox, "checkbox")
		if question == "" {
			continue
		}
		
		// Skip if already processed
		if processedQuestions[question] {
			continue
		}
		
		// Skip individual checkboxes if they're part of a group we already processed
		questionLower := strings.ToLower(question)
		if groupOptions[questionLower] || groupOptions["the "+questionLower] {
			// This checkbox is part of a group we already handled
			continue
		}
		processedQuestions[question] = true
		
		log.Printf("Checkbox %d: %s", i+1, question)
		
		// Determine if we should check it
		answer := determineComprehensiveAnswer(question, userData)
		
		// Only check if we have a positive answer or it's a consent/agreement checkbox
		shouldCheck := false
		questionLower = strings.ToLower(question)
		
		if answer != "" {
			// We have an answer - check if it's positive
			answerLower := strings.ToLower(answer)
			if answerLower == "yes" || answerLower == "true" || answerLower == "checked" || answerLower == "selected" {
				shouldCheck = true
			}
		} else if strings.Contains(questionLower, "agree") || strings.Contains(questionLower, "consent") || strings.Contains(questionLower, "acknowledge") {
			// This is likely a terms/consent checkbox - check it
			shouldCheck = true
			log.Printf("  Auto-checking consent/agreement checkbox")
		} else {
			// Skip country/location checkboxes - these are separate from the country dropdown
			// The user needs to explicitly select which countries they're willing to work in
			log.Printf("  Skipping checkbox - no answer provided")
		}
		
		if shouldCheck {
			checkbox.Check()
			filledCount++
			log.Printf("  ✓ Checked checkbox")
		}
	}
	} // End of else block for checkbox processing
	
	// Find radio button groups
	radioGroups, _ := iframe.Locator("fieldset:has(input[type='radio']):visible").All()
	for _, fieldset := range radioGroups {
		// Get question text
		question := getQuestionForElementComprehensive(iframe, fieldset, "fieldset")
		if question == "" {
			// Try to get from legend
			legend := fieldset.Locator("legend").First()
			if legendText, _ := legend.TextContent(); legendText != "" {
				question = strings.TrimSpace(legendText)
			}
		}
		
		if question == "" {
			continue
		}
		
		// Skip if already processed
		if processedQuestions[question] {
			continue
		}
		processedQuestions[question] = true
		
		log.Printf("Radio group: %s", question)
		
		// Determine answer
		answer := determineComprehensiveAnswer(question, userData)
		if answer == "" {
			log.Printf("  No answer for: %s", question)
			unknownQuestions = append(unknownQuestions, question)
			continue
		}
		
		// Find and select the matching radio button
		radios, _ := fieldset.Locator("input[type='radio']").All()
		for _, radio := range radios {
			// Get label for this radio
			radioId, _ := radio.GetAttribute("id")
			var radioLabel string
			if radioId != "" {
				label := iframe.Locator(fmt.Sprintf("label[for='%s']", radioId))
				radioLabel, _ = label.TextContent()
			}
			if radioLabel == "" {
				// Try next sibling
				label := radio.Locator("~ label").First()
				radioLabel, _ = label.TextContent()
			}
			
			radioLabel = strings.TrimSpace(radioLabel)
			if matchesAnswerComprehensive(radioLabel, answer) {
				radio.Check()
				filledCount++
				log.Printf("  ✓ Selected radio: %s", radioLabel)
				break
			}
		}
	}
	
	log.Printf("=== Filled %d fields total ===", filledCount)
	log.Println("=== DROPDOWN HANDLING COMPLETE ===")
	
	if len(unknownQuestions) > 0 {
		log.Printf("WARNING: Unable to fill %d fields: %v", len(unknownQuestions), unknownQuestions)
		
		// Debug: Check what's in unknownQuestions
		log.Printf("=== DEBUG: Checking unknownQuestions for demographic fields ===")
		for _, q := range unknownQuestions {
			lowerQ := strings.ToLower(q)
			if strings.Contains(lowerQ, "racial") || strings.Contains(lowerQ, "ethnic") ||
			   strings.Contains(lowerQ, "gender") || strings.Contains(lowerQ, "orientation") {
				log.Printf("  FOUND DEMOGRAPHIC FIELD IN unknownQuestions: '%s'", q)
			}
		}
		
		// Create MissingFieldsError with options
		var missingFields []MissingFieldInfo
		for _, question := range unknownQuestions {
			question = strings.TrimSpace(question)
			if question != "" {
				// CRITICAL: Skip racial/ethnic/gender/orientation fields that were already filled
				lowerQuestion := strings.ToLower(question)
				if strings.Contains(lowerQuestion, "racial") || strings.Contains(lowerQuestion, "ethnic") ||
				   strings.Contains(lowerQuestion, "gender identity") || strings.Contains(lowerQuestion, "sexual orientation") ||
				   strings.Contains(lowerQuestion, "mark all that apply") {
					log.Printf("WARNING: Skipping demographic field that should have been filled: '%s'", question)
					continue
				}
				
				var info MissingFieldInfo
				
				// Debug logging to understand what's happening
				log.Printf("Processing unknown question: '%s'", question)
				log.Printf("  Available options in missingFieldsWithOptions:")
				for k, v := range missingFieldsWithOptions {
					log.Printf("    Key: '%s' => %d options", k, len(v))
				}
				
				// If we have extracted options for this field, use them instead of createMissingFieldInfo
				if options, ok := missingFieldsWithOptions[question]; ok && len(options) > 0 {
					info = MissingFieldInfo{
						FieldName: question,
						Question:  question,
						Required:  strings.Contains(question, "*"),
						FieldType: "checkbox_group",
						Options:   options,
					}
					log.Printf("Using extracted options for %s: %v", question, options)
				} else {
					log.Printf("No extracted options found for '%s', using createMissingFieldInfo", question)
					// Use the default function for other fields
					info = createMissingFieldInfo(question)
					log.Printf("createMissingFieldInfo returned %d options: %v", len(info.Options), info.Options)
				}
				
				missingFields = append(missingFields, info)
			}
		}
		
		// Return structured error with options
		return &MissingFieldsError{
			Fields: missingFields,
			Message: "Additional information required to complete the application",
		}
	}
	
	log.Println("Returning from HandleAllDropdownsComprehensive successfully")
	return nil
}

func getQuestionForElementComprehensive(iframe playwright.FrameLocator, elem playwright.Locator, elemType string) string {
	// Try to get ID for label matching - FAST method
	id, _ := elem.GetAttribute("id")
	if id != "" {
		label := iframe.Locator(fmt.Sprintf("label[for='%s']", id))
		if count, _ := label.Count(); count > 0 {
			text, _ := label.TextContent()
			if text != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	
	// Try aria-labelledby - FAST method
	labelledBy, _ := elem.GetAttribute("aria-labelledby")
	if labelledBy != "" {
		label := iframe.Locator(fmt.Sprintf("#%s", labelledBy))
		if count, _ := label.Count(); count > 0 {
			text, _ := label.TextContent()
			if text != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	
	// Skip complex JavaScript evaluation if we're dealing with too many elements
	// This prevents hanging on complex pages
	return ""
	
	// DISABLED: Complex JavaScript can hang on some pages
	// Look for nearby label or text
	/*questionText, _ := elem.Evaluate(`
		el => {
			// Look for label in parent hierarchy
			let parent = el.parentElement;
			let depth = 0;
			while (parent && depth < 5) {
				// Check for label
				const label = parent.querySelector('label');
				if (label && label.textContent) {
					return label.textContent.trim();
				}
				
				// Check for text with * (required field indicator)
				const texts = parent.querySelectorAll('span, div, p');
				for (const t of texts) {
					if (t.textContent && t.textContent.includes('*')) {
						// Make sure it's not the dropdown itself
						if (!t.contains(el) && t.textContent.length < 200) {
							return t.textContent.trim();
						}
					}
				}
				
				parent = parent.parentElement;
				depth++;
			}
			
			// Look for preceding sibling with text
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
	}*/
	
	// return "" // Already returned above
}

func fillHTMLSelect(sel playwright.Locator, answer string) bool {
	// Get all options
	options, _ := sel.Locator("option").All()
	
	// Try exact match first
	for _, opt := range options {
		text, _ := opt.TextContent()
		text = strings.TrimSpace(text)
		if matchesAnswerComprehensive(text, answer) {
			value, _ := opt.GetAttribute("value")
			sel.SelectOption(playwright.SelectOptionValues{
				Values: &[]string{value},
			})
			return true
		}
	}
	
	// Try partial match
	for _, opt := range options {
		text, _ := opt.TextContent()
		text = strings.TrimSpace(text)
		if partialMatch(text, answer) {
			value, _ := opt.GetAttribute("value")
			sel.SelectOption(playwright.SelectOptionValues{
				Values: &[]string{value},
			})
			return true
		}
	}
	
	return false
}

func fillDivDropdown(iframe playwright.FrameLocator, div playwright.Locator, answer string) bool {
	// First check if the element exists and get info about it
	isVisible, _ := div.IsVisible()
	isEnabled, _ := div.IsEnabled()
	box, boxErr := div.BoundingBox()
	
	if !isVisible {
		log.Printf("    ERROR: Dropdown not visible for answer '%s'", answer)
		return false
	}
	if !isEnabled {
		log.Printf("    ERROR: Dropdown not enabled for answer '%s'", answer)
		return false
	}
	if boxErr != nil {
		log.Printf("    ERROR: Cannot get bounding box for dropdown: %v", boxErr)
		return false
	}
	if box == nil {
		log.Printf("    ERROR: Dropdown has no bounding box for answer '%s'", answer)
		return false
	}
	
	log.Printf("    Dropdown info: visible=%v, enabled=%v, size=%dx%d at (%d,%d)", 
		isVisible, isEnabled, int(box.Width), int(box.Height), int(box.X), int(box.Y))
	
	// Click the dropdown
	if err := div.Click(playwright.LocatorClickOptions{
		Force: playwright.Bool(true),
	}); err != nil {
		log.Printf("    ERROR clicking dropdown: %v", err)
		return false
	}
	
	log.Printf("    Dropdown clicked, looking for option: %s", answer)
	
	// Try to find and click the option immediately
	variations := getAnswerVariationsComprehensive(answer)
	
	// First try the most likely selectors
	for _, variant := range variations {
		// Try role='option' first (most common)
		selector := fmt.Sprintf("div[role='option']:text-is('%s'):visible", variant)
		opt := iframe.Locator(selector).First()
		count, _ := opt.Count()
		if count > 0 {
			log.Printf("    Found option with text '%s'", variant)
			if err := opt.Click(playwright.LocatorClickOptions{
				Force: playwright.Bool(true),
			}); err == nil {
				log.Printf("    ✓ Successfully clicked option: %s", variant)
				return true
			} else {
				log.Printf("    ERROR clicking option '%s': %v", variant, err)
			}
		}
	}
	
	// Get all options and click the matching one
	log.Printf("    Looking through all visible options...")
	options, _ := iframe.Locator("div[role='option']:visible").All()
	log.Printf("    Found %d visible options", len(options))
	for i, option := range options {
		text, _ := option.TextContent()
		text = strings.TrimSpace(text)
		if i < 5 { // Log first 5 options for debugging
			log.Printf("      Option %d: '%s'", i, text)
		}
		if matchesAnswerComprehensive(text, answer) {
			log.Printf("    Found matching option: '%s' matches '%s'", text, answer)
			if err := option.Click(playwright.LocatorClickOptions{
				Force: playwright.Bool(true),
			}); err == nil {
				log.Printf("    ✓ Successfully clicked matching option")
				return true
			} else {
				log.Printf("    ERROR clicking matching option: %v", err)
			}
		}
	}
	
	// Last resort: try contains text
	log.Printf("    Trying last resort - searching for text variants...")
	for _, variant := range variations {
		selector := fmt.Sprintf("*:has-text('%s'):visible", variant)
		opt := iframe.Locator(selector).First()
		if count, _ := opt.Count(); count > 0 {
			// Quick size check
			box, _ := opt.BoundingBox()
			if box != nil && box.Height < 60 && box.Height > 10 {
				log.Printf("    Found element with text '%s' (size: %dx%d)", variant, int(box.Width), int(box.Height))
				if err := opt.Click(playwright.LocatorClickOptions{
					Force: playwright.Bool(true),
				}); err == nil {
					log.Printf("    ✓ Successfully clicked element with text: %s", variant)
					return true
				} else {
					log.Printf("    ERROR clicking element: %v", err)
				}
			} else if box != nil {
				log.Printf("    Found element but wrong size: %dx%d (need height 10-60)", int(box.Width), int(box.Height))
			}
		}
	}
	
	log.Printf("    FAILED: Could not find or click option '%s'. Variations tried: %v", answer, variations)
	return false
}

func fillCombobox(iframe playwright.FrameLocator, combo playwright.Locator, answer string) bool {
	// These are typeahead comboboxes - need to type, not click
	
	// First clear any existing value
	if err := combo.Click(); err != nil {
		log.Printf("    ERROR: Could not click combobox: %v", err)
		return false
	}
	
	// Clear existing text with Ctrl+A and Delete
	if err := combo.Press("Control+a"); err == nil {
		combo.Press("Delete")
	}
	
	// Type the answer
	log.Printf("    Typing answer: %s", answer)
	if err := combo.Type(answer, playwright.LocatorTypeOptions{
		Delay: playwright.Float(50), // Small delay between keystrokes
	}); err != nil {
		log.Printf("    ERROR: Could not type in combobox: %v", err)
		return false
	}
	
	// Wait a moment for autocomplete to appear
	time.Sleep(500 * time.Millisecond)
	
	// Try to select the first matching option
	// Look for dropdown options that appeared
	optionSelectors := []string{
		fmt.Sprintf("div[role='option']:has-text('%s'):visible", answer),
		fmt.Sprintf("li:has-text('%s'):visible", answer),
		fmt.Sprintf("*[role='option']:has-text('%s'):visible", answer),
		fmt.Sprintf("div:has-text('%s'):visible", answer),
	}
	
	for _, selector := range optionSelectors {
		opt := iframe.Locator(selector).First()
		if count, _ := opt.Count(); count > 0 {
			// Check if it's a reasonable size (not the whole page)
			if box, err := opt.BoundingBox(); err == nil && box != nil {
				if box.Height < 100 && box.Height > 10 {
					log.Printf("    Found autocomplete option, clicking it")
					if err := opt.Click(); err == nil {
						log.Printf("    ✓ Selected option from autocomplete")
						return true
					}
				}
			}
		}
	}
	
	// If no dropdown appeared, try pressing Enter to accept the typed value
	log.Printf("    No autocomplete found, pressing Enter to accept typed value")
	if err := combo.Press("Enter"); err != nil {
		log.Printf("    ERROR: Could not press Enter: %v", err)
	}
	
	// Also try Tab to move to next field (some forms accept on Tab)
	if err := combo.Press("Tab"); err != nil {
		log.Printf("    ERROR: Could not press Tab: %v", err)
	}
	
	return true // Consider it filled if we typed the value
}

func matchesAnswerComprehensive(text, answer string) bool {
	textLower := strings.ToLower(strings.TrimSpace(text))
	answerLower := strings.ToLower(strings.TrimSpace(answer))
	
	// Exact match
	if text == answer || textLower == answerLower {
		return true
	}
	
	// Common variations
	switch answerLower {
	case "united states":
		return textLower == "united states" || textLower == "us" || textLower == "usa" || textLower == "united states of america"
	case "yes":
		return strings.HasPrefix(textLower, "yes")
	case "no":
		return strings.HasPrefix(textLower, "no") && !strings.Contains(textLower, "not to")
	case "asian":
		// Match various Asian ethnicity options
		return strings.Contains(textLower, "asian") || 
			   textLower == "east asian" || 
			   textLower == "south asian" || 
			   textLower == "southeast asian"
	case "prefer not to answer", "prefer not to say":
		return strings.Contains(textLower, "prefer not") || 
			   strings.Contains(textLower, "decline to")
	}
	
	// Partial matching for other cases
	if strings.Contains(textLower, answerLower) {
		return true
	}
	
	return false
}

func partialMatch(text, answer string) bool {
	textLower := strings.ToLower(text)
	answerLower := strings.ToLower(answer)
	
	// Check if key parts match
	if strings.Contains(textLower, answerLower) || strings.Contains(answerLower, textLower) {
		return true
	}
	
	return false
}

func getAnswerVariationsComprehensive(answer string) []string {
	base := []string{answer}
	
	switch strings.ToLower(answer) {
	case "united states":
		return append(base, "US", "USA", "United States of America")
	case "yes":
		return append(base, "Yes", "YES")
	case "no":
		return append(base, "No", "NO")
	default:
		return base
	}
}

func determineComprehensiveAnswer(question string, userData *UserProfileData) string {
	questionLower := strings.ToLower(question)
	
	// Debug logging for ExtraQA
	if userData.ExtraQA != nil && len(userData.ExtraQA) > 0 {
		log.Printf("DEBUG: Checking question '%s' against %d saved preferences", question, len(userData.ExtraQA))
		for k, v := range userData.ExtraQA {
			log.Printf("  Saved: '%s' => '%s'", k, v)
		}
	}
	
	// Check ExtraQA with flexible matching
	if userData.ExtraQA != nil {
		// Try exact match first
		if answer, exists := userData.ExtraQA[question]; exists {
			return answer
		}
		
		// Try lowercase version
		if answer, exists := userData.ExtraQA[questionLower]; exists {
			return answer
		}
		
		// Clean the question and try again (remove *, ?, trim spaces)
		cleanQuestion := strings.TrimSpace(question)
		cleanQuestion = strings.ReplaceAll(cleanQuestion, "*", "")
		cleanQuestion = strings.ReplaceAll(cleanQuestion, "?", "")
		cleanQuestion = strings.TrimSpace(cleanQuestion)
		
		if answer, exists := userData.ExtraQA[cleanQuestion]; exists {
			return answer
		}
		
		// Try lowercase cleaned version
		cleanQuestionLower := strings.ToLower(cleanQuestion)
		if answer, exists := userData.ExtraQA[cleanQuestionLower]; exists {
			return answer
		}
		
		// Try to find partial matches (for questions that might be saved differently)
		// More intelligent matching based on key terms
		for savedQuestion, savedAnswer := range userData.ExtraQA {
			savedLower := strings.ToLower(savedQuestion)
			
			// Check for key term matches - these are the important parts
			// Remote work question
			if (strings.Contains(questionLower, "remote") && strings.Contains(questionLower, "work")) &&
			   (strings.Contains(savedLower, "remote") && (strings.Contains(savedLower, "work") || strings.Contains(savedLower, "role"))) {
				log.Printf("  MATCH FOUND: Remote work question matched")
				return savedAnswer
			}
			
			// Employment question (Stripe or any company)
			if (strings.Contains(questionLower, "employed") && (strings.Contains(questionLower, "stripe") || strings.Contains(questionLower, "affiliate"))) &&
			   (strings.Contains(savedLower, "employed") && (strings.Contains(savedLower, "company") || strings.Contains(savedLower, "affiliate"))) {
				log.Printf("  MATCH FOUND: Employment history question matched")
				return savedAnswer
			}
			
			// WhatsApp question
			if strings.Contains(questionLower, "whatsapp") && strings.Contains(savedLower, "whatsapp") {
				log.Printf("  MATCH FOUND: WhatsApp question matched")
				return savedAnswer
			}
			
			// Sexual orientation
			if strings.Contains(questionLower, "sexual orientation") && strings.Contains(savedLower, "sexual orientation") {
				log.Printf("  MATCH FOUND: Sexual orientation question matched")
				return savedAnswer
			}
			
			// Transgender question
			if strings.Contains(questionLower, "transgender") && strings.Contains(savedLower, "transgender") {
				log.Printf("  MATCH FOUND: Transgender question matched")
				return savedAnswer
			}
			
			// General fallback - check if core keywords match
			if strings.Contains(questionLower, savedLower) || strings.Contains(savedLower, questionLower) {
				// Additional validation - make sure it's the same type of question
				if len(savedLower) > 10 { // Don't match very short saved answers
					log.Printf("  MATCH FOUND: General match for '%s'", savedQuestion)
					return savedAnswer
				}
			}
		}
	}
	
	// Employment information
	if strings.Contains(questionLower, "current") && (strings.Contains(questionLower, "employer") || strings.Contains(questionLower, "company")) {
		// Get most recent employer from experiences
		if userData.Experience != nil && len(userData.Experience) > 0 {
			return userData.Experience[0].Company
		}
		return ""
	}
	
	if strings.Contains(questionLower, "job title") || strings.Contains(questionLower, "position") || strings.Contains(questionLower, "role") {
		// Get most recent job title
		if userData.Experience != nil && len(userData.Experience) > 0 {
			return userData.Experience[0].Title
		}
		return ""
	}
	
	// Education information
	if strings.Contains(questionLower, "school") || strings.Contains(questionLower, "university") || strings.Contains(questionLower, "college") {
		// Get most recent school
		if userData.Education != nil && len(userData.Education) > 0 {
			return userData.Education[0].Institution
		}
		return ""
	}
	
	if strings.Contains(questionLower, "degree") || strings.Contains(questionLower, "qualification") {
		// Get most recent degree
		if userData.Education != nil && len(userData.Education) > 0 {
			return userData.Education[0].Degree
		}
		return ""
	}
	
	// Country questions
	if strings.Contains(questionLower, "country") {
		if strings.Contains(questionLower, "reside") || strings.Contains(questionLower, "located") {
			if userData.Country != "" {
				return userData.Country
			}
			return "United States"
		}
		if strings.Contains(questionLower, "work") || strings.Contains(questionLower, "anticipate") {
			if userData.Country != "" {
				return userData.Country  
			}
			return "United States"
		}
		return "United States"
	}
	
	// Work authorization
	if strings.Contains(questionLower, "authorized") && strings.Contains(questionLower, "work") {
		if userData.WorkAuthorization == "yes" {
			return "Yes"
		} else if userData.WorkAuthorization == "no" {
			return "No"
		}
		// Don't assume - ask user if not provided
		return ""
	}
	
	// Sponsorship
	if strings.Contains(questionLower, "sponsor") {
		// Only answer if we have explicit data
		if userData.WorkAuthorization == "yes" && !userData.RequiresSponsorship {
			return "No"
		} else if userData.RequiresSponsorship {
			return "Yes"
		}
		// Don't assume - ask user if not provided
		return ""
	}
	
	// Gender
	if strings.Contains(questionLower, "gender") && !strings.Contains(questionLower, "transgender") {
		if userData.Gender != "" {
			switch strings.ToLower(userData.Gender) {
			case "male":
				return "Man"
			case "female":
				return "Woman"
			default:
				return userData.Gender
			}
		}
		return ""
	}
	
	// Race/Ethnicity
	if strings.Contains(questionLower, "race") || strings.Contains(questionLower, "ethnic") {
		if userData.Ethnicity != "" && userData.Ethnicity != "prefer_not_to_say" {
			return userData.Ethnicity
		}
		return ""
	}
	
	// Veteran status
	if strings.Contains(questionLower, "veteran") {
		if userData.VeteranStatus == "yes" {
			return "Yes"
		} else if userData.VeteranStatus == "no" {
			return "No"
		}
		return ""
	}
	
	// Disability
	if strings.Contains(questionLower, "disability") || strings.Contains(questionLower, "chronic condition") {
		if userData.DisabilityStatus == "yes" {
			return "Yes"
		} else if userData.DisabilityStatus == "no" {
			return "No"
		}
		return ""
	}
	
	// Sexual orientation
	if strings.Contains(questionLower, "sexual orientation") {
		if userData.SexualOrientation != "" {
			return userData.SexualOrientation
		}
		return ""
	}
	
	// Transgender
	if strings.Contains(questionLower, "transgender") {
		if userData.TransgenderStatus != "" {
			return userData.TransgenderStatus
		}
		return ""
	}
	
	// Previous employment - handles both "previously employed" and "ever been employed"
	if (strings.Contains(questionLower, "previously") || strings.Contains(questionLower, "ever")) && 
	   strings.Contains(questionLower, "employed") {
		// Check ExtraQA for user's answer
		if userData.ExtraQA != nil {
			if val, ok := userData.ExtraQA["previously_employed"]; ok && val != "" {
				return val
			}
		}
		// Don't assume - this needs user input
		return ""
	}
	
	// Remote work
	if strings.Contains(questionLower, "remote") {
		if userData.RemoteWorkPreference == "yes" || userData.RemoteWorkPreference == "remote" {
			return "Yes"
		} else if userData.RemoteWorkPreference == "no" {
			return "No"
		}
		// Don't assume - ask user if not provided
		return ""
	}
	
	// WhatsApp
	if strings.Contains(questionLower, "whatsapp") {
		// Check ExtraQA for user's preference
		if userData.ExtraQA != nil {
			if val, ok := userData.ExtraQA["whatsapp_opt_in"]; ok && val != "" {
				return val
			}
		}
		// Don't assume - ask user for preference
		return ""
	}
	
	// Default: return empty string for unknown questions
	return ""
}