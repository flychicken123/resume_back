package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

// HandleIframeDropdownsV2 handles dropdowns more intelligently - collect all unknowns first
func HandleIframeDropdownsV2(iframe playwright.FrameLocator, userData *UserProfileData) error {
	log.Println("=== HANDLING DROPDOWNS INSIDE IFRAME V2 ===")
	
	// Give iframe time to load
	time.Sleep(2 * time.Second)
	
	// Step 1: Collect ALL unique questions first
	allQuestions := collectAllDropdownQuestions(iframe)
	log.Printf("Found %d unique questions total", len(allQuestions))
	
	// Step 2: Determine which ones we can answer
	unknownQuestions := []string{}
	knownQuestions := make(map[string]string)
	
	for question := range allQuestions {
		// Skip invalid or placeholder questions
		if question == "US" || question == "Enter manually" || question == "" || len(question) < 3 {
			log.Printf("  ⏭️ Skipping invalid question: %s", question)
			continue
		}
		
		answer := determineIframeDropdownValue(question, userData)
		if answer == "" {
			log.Printf("  ❌ Unknown: %s", question)
			unknownQuestions = append(unknownQuestions, question)
		} else {
			log.Printf("  ✓ Known: %s -> %s", question, answer)
			knownQuestions[question] = answer
		}
	}
	
	// Step 3: If we have unknown questions, stop immediately
	if len(unknownQuestions) > 0 {
		log.Printf("⚠️ STOPPING: Found %d unknown fields that require user input", len(unknownQuestions))
		for _, q := range unknownQuestions {
			log.Printf("  - %s", q)
		}
		// Format the questions with pipe separator for easier parsing
		questionsStr := strings.Join(unknownQuestions, " | ")
		return fmt.Errorf("Unable to fill %d fields. Please provide answers for: [%s]", 
			len(unknownQuestions), questionsStr)
	}
	
	// Step 4: Only if we know all answers, try to fill them
	log.Println("All questions have answers, proceeding to fill...")
	for question, answer := range knownQuestions {
		fillDropdownForQuestion(iframe, question, answer)
	}
	
	return nil
}

// collectAllDropdownQuestions finds all unique dropdown questions in the iframe
func collectAllDropdownQuestions(iframe playwright.FrameLocator) map[string]bool {
	questions := make(map[string]bool)
	processedElements := make(map[string]bool) // Track processed elements to avoid duplicates
	
	// Check React Select placeholders - be more specific to avoid duplicates
	placeholders, err := iframe.Locator("div:has-text('Select...'):visible, span:has-text('Select...'):visible").All()
	if err == nil {
		for _, placeholder := range placeholders {
			// Get a unique identifier for this element to avoid processing duplicates
			text, _ := placeholder.TextContent()
			if processedElements[text] {
				continue
			}
			processedElements[text] = true
			
			question := getQuestionForElement(iframe, placeholder)
			if question != "" && !questions[question] {
				questions[question] = true
			}
		}
	}
	
	// Check combobox inputs
	comboboxes, err := iframe.Locator("input[role='combobox']").All()
	if err == nil {
		for _, combobox := range comboboxes {
			// Check if already has value
			value, _ := combobox.InputValue()
			if value != "" {
				continue
			}
			
			question := getQuestionForCombobox(iframe, combobox)
			if question != "" && !questions[question] {
				questions[question] = true
			}
		}
	}
	
	// Check standard selects
	selects, err := iframe.Locator("select").All()
	if err == nil {
		for _, sel := range selects {
			// Check if already has value
			value, _ := sel.InputValue()
			if value != "" && value != "0" && value != "-1" {
				continue
			}
			
			question := getQuestionForSelect(iframe, sel)
			if question != "" && !questions[question] {
				questions[question] = true
			}
		}
	}
	
	return questions
}

func getQuestionForElement(iframe playwright.FrameLocator, element playwright.Locator) string {
	// Strategy 1: Look for parent container with label
	parent := element.Locator("xpath=ancestor::div[contains(@class, 'form') or contains(@class, 'field') or contains(@class, 'question') or contains(@class, 'select')]").First()
	if count, _ := parent.Count(); count > 0 {
		// Look for label within the parent container
		label := parent.Locator("label").First()
		if labelCount, _ := label.Count(); labelCount > 0 {
			text, _ := label.TextContent()
			cleanText := strings.TrimSpace(text)
			if cleanText != "" && cleanText != "Select..." {
				return cleanText
			}
		}
	}
	
	// Strategy 2: Look for closest preceding label (most recent one)
	precedingLabel := element.Locator("xpath=preceding::label[1]").First()
	if count, _ := precedingLabel.Count(); count > 0 {
		text, _ := precedingLabel.TextContent()
		cleanText := strings.TrimSpace(text)
		// Make sure it's a real question, not just "Select..."
		if cleanText != "" && cleanText != "Select..." && len(cleanText) > 5 {
			return cleanText
		}
	}
	
	// Strategy 3: Look for text in parent that ends with ? or *
	grandParent := element.Locator("xpath=ancestor::div[1]").First()
	if count, _ := grandParent.Count(); count > 0 {
		text, _ := grandParent.TextContent()
		// Extract question-like text
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if (strings.HasSuffix(line, "?") || strings.HasSuffix(line, "*")) && len(line) > 10 {
				return line
			}
		}
	}
	
	return ""
}

func getQuestionForCombobox(iframe playwright.FrameLocator, combobox playwright.Locator) string {
	// Check aria-labelledby
	labelId, _ := combobox.GetAttribute("aria-labelledby")
	if labelId != "" {
		label := iframe.Locator(fmt.Sprintf("#%s", labelId))
		if count, _ := label.Count(); count > 0 {
			text, _ := label.TextContent()
			return strings.TrimSpace(text)
		}
	}
	
	return ""
}

func getQuestionForSelect(iframe playwright.FrameLocator, sel playwright.Locator) string {
	selectId, _ := sel.GetAttribute("id")
	if selectId != "" {
		label := iframe.Locator(fmt.Sprintf("label[for='%s']", selectId))
		if count, _ := label.Count(); count > 0 {
			text, _ := label.TextContent()
			return strings.TrimSpace(text)
		}
	}
	
	selectName, _ := sel.GetAttribute("name")
	return selectName
}

func fillDropdownForQuestion(iframe playwright.FrameLocator, question string, answer string) {
	log.Printf("Filling '%s' with '%s'", question, answer)
	
	// Try React Select components first (most common in modern forms)
	// Be more specific - look for the actual dropdown container, not just any element with "Select..."
	reactSelects, err := iframe.Locator("div[class*='select']:has-text('Select...'):visible, div[class*='Select']:has-text('Select...'):visible, div[class*='dropdown']:has-text('Select...'):visible").All()
	if err == nil {
		foundAndFilled := false
		for _, placeholder := range reactSelects {
			if foundAndFilled {
				break // Stop after successfully filling one
			}
			
			// Check if this is actually a dropdown container (not too large)
			box, _ := placeholder.BoundingBox()
			if box != nil && box.Height > 200 {
				continue // Skip if container is too large (probably not a dropdown)
			}
			
			foundQuestion := getQuestionForElement(iframe, placeholder)
			// Make sure we have the right question match
			if foundQuestion == question || strings.Contains(foundQuestion, strings.TrimSuffix(question, "*")) {
				// Click to open the React Select dropdown
				if err := placeholder.Click(); err == nil {
					log.Printf("  Clicked React Select dropdown for: %s", question)
					time.Sleep(700 * time.Millisecond) // Give more time for dropdown to open
					
					// Try multiple strategies to find and click the option
					// Strategy 1: Look for exact text match
					exactOption := iframe.Locator(fmt.Sprintf("text='%s'", answer)).First()
					if count, _ := exactOption.Count(); count > 0 {
						if visible, _ := exactOption.IsVisible(); visible {
							if err := exactOption.Click(); err == nil {
								log.Printf("  ✓ Selected '%s' in React Select dropdown (exact match)", answer)
								foundAndFilled = true
								time.Sleep(300 * time.Millisecond)
								return
							}
						}
					}
					
					// Strategy 2: Look for option elements with various patterns
					optionPatterns := []string{
						fmt.Sprintf("div[id*='option']:text-is('%s')", answer),
						fmt.Sprintf("div[class*='option']:text-is('%s')", answer),
						fmt.Sprintf("*[role='option']:text-is('%s')", answer),
						fmt.Sprintf("div[id*='react-select']:has-text('%s')", answer),
						fmt.Sprintf("div:has-text('%s'):not(:has(*))", answer), // Leaf nodes only
					}
					
					for _, pattern := range optionPatterns {
						options, err := iframe.Locator(pattern).All()
						if err == nil && len(options) > 0 {
							for _, option := range options {
								if visible, _ := option.IsVisible(); visible {
									if err := option.Click(); err == nil {
										log.Printf("  ✓ Selected '%s' in React Select dropdown (pattern: %s)", answer, pattern)
										foundAndFilled = true
										time.Sleep(300 * time.Millisecond)
										return
									}
								}
							}
						}
					}
					
					// Strategy 3: Find all visible divs with the answer text
					allDivs, _ := iframe.Locator(fmt.Sprintf("div:visible:has-text('%s')", answer)).All()
					for _, div := range allDivs {
						// Check if this is likely an option (not too big)
						box, _ := div.BoundingBox()
						if box != nil && box.Height < 100 { // Options are usually small
							if err := div.Click(); err == nil {
								log.Printf("  ✓ Selected '%s' in React Select dropdown (div click)", answer)
								foundAndFilled = true
								time.Sleep(300 * time.Millisecond)
								return
							}
						}
					}
					
					log.Printf("  ⚠ Could not find option '%s' in opened dropdown", answer)
				}
			}
		}
	}
	
	// Try combobox inputs
	comboboxes, err := iframe.Locator("input[role='combobox']").All()
	if err == nil {
		for _, combobox := range comboboxes {
			foundQuestion := getQuestionForCombobox(iframe, combobox)
			if foundQuestion == question {
				// Click to open dropdown
				if err := combobox.Click(); err == nil {
					log.Printf("  Clicked combobox for: %s", question)
					time.Sleep(500 * time.Millisecond)
					
					// Look for the option
					optionSelectors := []string{
						fmt.Sprintf("li:has-text('%s')", answer),
						fmt.Sprintf("div[role='option']:has-text('%s')", answer),
						fmt.Sprintf("div:has-text('%s'):visible", answer),
					}
					
					for _, selector := range optionSelectors {
						option := iframe.Locator(selector).First()
						if count, _ := option.Count(); count > 0 {
							if err := option.Click(); err == nil {
								log.Printf("  ✓ Selected '%s' in combobox", answer)
								time.Sleep(300 * time.Millisecond)
								return
							}
						}
					}
				}
			}
		}
	}
	
	// Try standard select elements
	selects, err := iframe.Locator("select").All()
	if err == nil {
		for _, sel := range selects {
			foundQuestion := getQuestionForSelect(iframe, sel)
			if foundQuestion == question {
				// Get all options to find the right value
				options, _ := sel.Locator("option").All()
				for _, opt := range options {
					optText, _ := opt.TextContent()
					optValue, _ := opt.GetAttribute("value")
					
					// Check if this option matches our answer
					optTextClean := strings.TrimSpace(optText)
					if strings.EqualFold(optTextClean, answer) || 
					   strings.Contains(strings.ToLower(optTextClean), strings.ToLower(answer)) {
						// Use SelectOption to select this value
						_, err := sel.SelectOption(playwright.SelectOptionValues{Values: &[]string{optValue}})
						if err == nil {
							log.Printf("  ✓ Selected '%s' (value=%s) in standard select", optText, optValue)
							return
						}
					}
				}
			}
		}
	}
	
	log.Printf("  ⚠ Could not find or fill dropdown for question: %s", question)
}