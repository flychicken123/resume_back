package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	
	"github.com/playwright-community/playwright-go"
)

type DropdownInfo struct {
	Element  playwright.Locator
	Question string
	Type     string // "html-select" or "div-select"
}

// HandleIframeDropdownsV5 handles dropdowns with improved detection
func HandleIframeDropdownsV5(iframe playwright.FrameLocator, userData *UserProfileData) error {
	log.Println("=== HANDLING DROPDOWNS INSIDE IFRAME V5 ===")
	
	// Give iframe time to load
	time.Sleep(1 * time.Second)
	
	// Find only actual dropdowns, not text inputs or checkboxes
	dropdowns := findRealDropdowns(iframe)
	log.Printf("Found %d actual dropdowns to process", len(dropdowns))
	
	unknownQuestions := []string{}
	
	for _, dropdown := range dropdowns {
		// Skip questions that shouldn't be dropdowns
		questionLower := strings.ToLower(dropdown.Question)
		if strings.Contains(questionLower, "first name") || 
		   strings.Contains(questionLower, "last name") ||
		   strings.Contains(questionLower, "apply for this job") ||
		   strings.Contains(questionLower, "autofill with greenhouse") {
			log.Printf("  Skipping non-dropdown field: %s", dropdown.Question)
			continue
		}
		
		answer := determineIframeDropdownValue(dropdown.Question, userData)
		if answer == "" {
			log.Printf("  ❌ Unknown: %s", dropdown.Question)
			unknownQuestions = append(unknownQuestions, dropdown.Question)
		} else {
			log.Printf("  Processing: %s -> %s", dropdown.Question, answer)
			fillDropdownQuickly(iframe, dropdown, answer)
		}
	}
	
	if len(unknownQuestions) > 0 {
		questionsStr := strings.Join(unknownQuestions, " | ")
		return fmt.Errorf("Unable to fill %d fields. Please provide answers for: [%s]", 
			len(unknownQuestions), questionsStr)
	}
	
	return nil
}

func findRealDropdowns(iframe playwright.FrameLocator) []DropdownInfo {
	var dropdowns []DropdownInfo
	seenQuestions := make(map[string]bool)
	processedElements := make(map[string]bool)
	
	// Method 1: Find divs with "Select..." that are actual dropdowns
	selectDivs, _ := iframe.Locator("div:has-text('Select...'):visible").All()
	
	for _, elem := range selectDivs {
		// Create element ID to avoid duplicates
		elemText, _ := elem.TextContent()
		elemID := fmt.Sprintf("%p_%s", elem, elemText)
		if processedElements[elemID] {
			continue
		}
		processedElements[elemID] = true
		
		// Check if this is a dropdown control (not a container or label)
		isDropdown, _ := elem.Evaluate(`
			el => {
				// Check if element is small enough to be a dropdown
				if (el.offsetHeight > 100) return false;
				
				// Check if it's clickable
				const style = window.getComputedStyle(el);
				if (style.cursor === 'pointer' || style.cursor === 'hand') return true;
				
				// Check if it has dropdown-like classes
				const className = el.className || '';
				if (className.includes('select') || className.includes('dropdown') || 
				    className.includes('control') || className.includes('value')) return true;
				
				// Check if parent is a dropdown container
				const parent = el.parentElement;
				if (parent) {
					const parentClass = parent.className || '';
					if (parentClass.includes('select') || parentClass.includes('dropdown')) return true;
				}
				
				return false;
			}
		`, nil)
		
		if isDropdown != true {
			continue
		}
		
		// Find the question for this dropdown
		question := findQuestionForElement(iframe, elem)
		
		// Skip if:
		// 1. No question found
		// 2. Question already processed
		// 3. Question is too short (likely not a real question)
		// 4. Question contains checkbox list items
		if question == "" || seenQuestions[question] || len(question) < 3 {
			continue
		}
		
		// Skip if question contains country list (these are checkboxes, not dropdowns)
		if strings.Contains(question, "Australia") && strings.Contains(question, "Belgium") {
			continue
		}
		
		// Skip single country names (these are checkbox labels)
		if question == "Australia" || question == "Belgium" || question == "Brazil" || 
		   question == "Canada" || question == "France" || question == "Germany" {
			continue
		}
		
		seenQuestions[question] = true
		dropdowns = append(dropdowns, DropdownInfo{
			Element:  elem,
			Question: question,
			Type:     "div-select",
		})
		
		log.Printf("  Found dropdown: %s", question)
	}
	
	// Method 2: Find standard HTML select elements
	htmlSelects, _ := iframe.Locator("select:visible").All()
	for _, sel := range htmlSelects {
		// Check if already has a value
		value, _ := sel.InputValue()
		if value != "" && value != "0" && value != "-1" && !strings.Contains(strings.ToLower(value), "select") {
			continue
		}
		
		question := findQuestionForElement(iframe, sel)
		if question == "" || seenQuestions[question] {
			continue
		}
		
		seenQuestions[question] = true
		dropdowns = append(dropdowns, DropdownInfo{
			Element:  sel,
			Question: question,
			Type:     "html-select",
		})
		
		log.Printf("  Found HTML select: %s", question)
	}
	
	return dropdowns
}

func findQuestionForElement(iframe playwright.FrameLocator, elem playwright.Locator) string {
	// Use JavaScript to find the associated question more reliably
	question, _ := elem.Evaluate(`
		el => {
			// Method 1: Check for associated label
			if (el.id) {
				const label = document.querySelector('label[for="' + el.id + '"]');
				if (label) return label.innerText.trim();
			}
			
			// Method 2: Look for preceding label or text
			let current = el;
			let attempts = 0;
			while (current && attempts < 5) {
				attempts++;
				
				// Check previous sibling
				let prev = current.previousElementSibling;
				if (prev) {
					if (prev.tagName === 'LABEL') {
						return prev.innerText.trim();
					}
					// Check if it contains a label
					const label = prev.querySelector('label');
					if (label) {
						return label.innerText.trim();
					}
					// Check if it's text with a question mark or asterisk
					const text = prev.innerText;
					if (text && (text.includes('?') || text.includes('*')) && text.length > 10) {
						// Make sure it's not a list of countries
						if (!text.includes('Australia') || !text.includes('Belgium')) {
							return text.trim();
						}
					}
				}
				
				// Move to parent and try again
				current = current.parentElement;
			}
			
			// Method 3: Check aria-label
			if (el.getAttribute('aria-label')) {
				return el.getAttribute('aria-label').trim();
			}
			
			return '';
		}
	`, nil)
	
	if questionStr, ok := question.(string); ok && questionStr != "" {
		// Clean up the question
		cleaned := strings.TrimSpace(questionStr)
		// Remove country lists if they're included
		if idx := strings.Index(cleaned, "\nAustralia"); idx > 0 {
			cleaned = cleaned[:idx]
		}
		return cleaned
	}
	
	return ""
}

func fillDropdownQuickly(iframe playwright.FrameLocator, dropdown DropdownInfo, answer string) {
	if dropdown.Type == "html-select" {
		// For HTML select, use SelectOption directly
		fillHTMLSelectQuickly(dropdown.Element, answer)
	} else {
		// For div-based dropdowns, click and select
		fillDivSelectQuickly(iframe, dropdown.Element, answer)
	}
}

func fillHTMLSelectQuickly(sel playwright.Locator, answer string) {
	// Get all options
	options, _ := sel.Locator("option").All()
	
	// Get answer variations
	variations := getAnswerVariations(answer)
	
	for _, variant := range variations {
		for _, opt := range options {
			text, _ := opt.TextContent()
			value, _ := opt.GetAttribute("value")
			
			textClean := strings.TrimSpace(text)
			if strings.EqualFold(textClean, variant) || 
			   (len(variant) == 2 && strings.HasPrefix(strings.ToUpper(textClean), strings.ToUpper(variant))) {
				sel.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
				log.Printf("  ✓ Selected '%s' in HTML select", textClean)
				return
			}
		}
	}
	
	log.Printf("  ⚠ Could not select '%s' in HTML select", answer)
}

func fillDivSelectQuickly(iframe playwright.FrameLocator, elem playwright.Locator, answer string) {
	// Click to open dropdown
	if err := elem.Click(); err != nil {
		log.Printf("  Failed to click dropdown: %v", err)
		return
	}
	
	log.Printf("  Clicked dropdown")
	time.Sleep(200 * time.Millisecond) // Small wait for animation
	
	// Get variations
	variations := getAnswerVariations(answer)
	
	// Try to find and click the option quickly
	for _, variant := range variations {
		// Use the most efficient selector first
		option := iframe.Locator(fmt.Sprintf("*:text-is('%s'):visible", variant)).First()
		if count, _ := option.Count(); count > 0 {
			// Make sure it's not too large (not a container)
			isOption, _ := option.Evaluate(`el => el.offsetHeight < 60`, nil)
			if isOption == true {
				if err := option.Click(); err == nil {
					log.Printf("  ✓ Selected '%s'", variant)
					return
				}
			}
		}
	}
	
	// Quick fallback - try role=option
	for _, variant := range variations {
		option := iframe.Locator(fmt.Sprintf("*[role='option']:has-text('%s'):visible", variant)).First()
		if count, _ := option.Count(); count > 0 {
			if err := option.Click(); err == nil {
				log.Printf("  ✓ Selected '%s' (role=option)", variant)
				return
			}
		}
	}
	
	log.Printf("  ⚠ Could not find option '%s' in dropdown", answer)
}

func getAnswerVariations(answer string) []string {
	variations := []string{answer}
	
	switch answer {
	case "United States":
		// For country selection, US is most common
		variations = []string{"US", "USA", "United States", "United States of America"}
	case "Heterosexual or straight":
		variations = []string{"Straight", "Heterosexual", "Heterosexual or straight"}
	case "Yes":
		// For yes, also check for longer options that start with Yes
		variations = []string{"Yes", "Yes, I intend to work remotely."}
	case "No":
		// For no, also check for longer options
		variations = []string{"No", "No, I intend to work from an office location."}
	}
	
	return variations
}