package services

import (
	"fmt"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// HandleStripeSpecificDropdowns handles the remaining unfilled dropdowns on Stripe forms
func HandleStripeSpecificDropdowns(iframe playwright.FrameLocator, userData *UserProfileData) error {
	// Find all dropdowns that still have "Select..." text
	unfilledDropdowns, _ := iframe.Locator("div:has-text('Select...'):visible").All()
	
	// Track which questions we've already processed to avoid duplicates
	processedQuestions := make(map[string]bool)
	processedCount := 0
	maxToProcess := 20 // Limit to avoid processing too many duplicates
	
	for _, dropdown := range unfilledDropdowns {
		// Stop if we've processed enough dropdowns
		if processedCount >= maxToProcess {
			break
		}
		// Get the full text to understand context
		text, _ := dropdown.TextContent()
		if text == "" {
			continue
		}
		
		// Skip if this is just a label or large container
		bbox, _ := dropdown.BoundingBox()
		if bbox != nil && bbox.Height > 100 {
			continue
		}
		
		// Determine what this dropdown is for based on surrounding text
		question := getDropdownQuestion(iframe, dropdown)
		
		// Skip if we've already processed this question
		if processedQuestions[question] {
			continue
		}
		
		// Determine the answer
		answer := determineStripeAnswer(question, userData)
		if answer == "" {
			continue
		}
		
		// Try to fill it
		fillStripeDropdown(iframe, dropdown, answer, question)
		
		// Mark this question as processed
		processedQuestions[question] = true
		processedCount++
	}
	
	return nil
}

func getDropdownQuestion(iframe playwright.FrameLocator, dropdown playwright.Locator) string {
	// Try to get the question from nearby text
	parent, _ := dropdown.Evaluate(`
		el => {
			// Walk up to find the containing div with the question
			let current = el;
			while (current && current.parentElement) {
				current = current.parentElement;
				// Look for text that looks like a question
				const text = current.innerText || '';
				if (text.includes('?') || text.includes('*')) {
					// Extract just the question part
					const lines = text.split('\n');
					for (const line of lines) {
						if (line.includes('?') || (line.includes('*') && line.length > 10)) {
							return line.trim();
						}
					}
				}
			}
			return '';
		}
	`, nil)
	
	if q, ok := parent.(string); ok && q != "" {
		return q
	}
	
	return ""
}

func determineStripeAnswer(question string, userData *UserProfileData) string {
	q := strings.ToLower(question)
	
	// Handle specific Stripe questions
	if strings.Contains(q, "ever been employed by stripe") {
		return "No"
	}
	
	if strings.Contains(q, "remote location") || strings.Contains(q, "plan to work remotely") {
		return "No"
	}
	
	if strings.Contains(q, "gender identity") {
		return "Man"
	}
	
	if strings.Contains(q, "racial") || strings.Contains(q, "ethnic") {
		return "Asian"
	}
	
	if strings.Contains(q, "sexual orientation") {
		// Check ExtraQA first
		if ans, ok := userData.ExtraQA["How would you describe your sexual orientation? (mark all that apply)"]; ok {
			// If we have "Heterosexual or straight", just return "Heterosexual" for Stripe
			if ans == "Heterosexual or straight" {
				return "Heterosexual"
			}
			return ans
		}
		return "Heterosexual"
	}
	
	if strings.Contains(q, "transgender") {
		// Check ExtraQA first
		if ans, ok := userData.ExtraQA["Do you identify as transgender?"]; ok {
			return ans
		}
		return "No"
	}
	
	if strings.Contains(q, "disability") || strings.Contains(q, "chronic condition") {
		// Check ExtraQA first
		for key, val := range userData.ExtraQA {
			if strings.Contains(strings.ToLower(key), "disability") {
				return val
			}
		}
		return "No"
	}
	
	if strings.Contains(q, "veteran") || strings.Contains(q, "armed forces") {
		return "No"
	}
	
	if strings.Contains(q, "whatsapp") {
		return "No"
	}
	
	return ""
}

func fillStripeDropdown(iframe playwright.FrameLocator, dropdown playwright.Locator, answer string, question string) {
	// Click the dropdown with a short timeout
	err := dropdown.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(2000),
		Force: playwright.Bool(true), // Force click even if covered
	})
	
	if err != nil {
		return
	}
	
	// Try to wait for dropdown options to appear, but don't block if they don't
	optionsLocator := iframe.Locator("[role='option']:visible, li:visible").First()
	_ = optionsLocator.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(300), // Very short timeout to avoid blocking
	})
	
	// Now look for the option to select
	// Try multiple strategies
	
	// Strategy 1: Look for exact text match in dropdown options
	options := []string{
		answer,
		strings.ToLower(answer),
		strings.ToUpper(answer),
	}
	
	// Add variations for specific answers
	if answer == "Man" {
		options = append(options, "Male", "M", "Man")
	} else if answer == "Asian" {
		options = append(options, "Asian", "Asia", "Asian or Asian American", "Asian or Pacific Islander")
	} else if answer == "Heterosexual" {
		options = append(options, "Heterosexual", "Straight", "Heterosexual or straight")
	} else if answer == "No" {
		options = append(options, "No", "I am not", "No, I am not", "I don't wish to answer")
	}
	
	for _, opt := range options {
		// Try to find and click the option
		optionLocator := iframe.Locator(fmt.Sprintf("div[role='option']:has-text('%s'), li[role='option']:has-text('%s'), div:has-text('%s'):visible", opt, opt, opt)).First()
		
		if count, _ := optionLocator.Count(); count > 0 {
			// Check if it's actually an option (not too large)
			bbox, _ := optionLocator.BoundingBox()
			if bbox != nil && bbox.Height < 60 && bbox.Height > 0 {
				err := optionLocator.Click(playwright.LocatorClickOptions{
					Timeout: playwright.Float(1000),
				})
				if err == nil {
					return
				}
			}
		}
	}
	
	// Strategy 2: Use JavaScript to find and click
	_, _ = iframe.Locator("*:visible").EvaluateAll(`
		(elements, answer) => {
			const answerLower = answer.toLowerCase();
			const answerVariations = [answer];
			
			// Add variations based on answer
			if (answer === 'Man') {
				answerVariations.push('Male', 'M', 'Man');
			} else if (answer === 'Asian') {
				answerVariations.push('Asian', 'Asian or Asian American', 'Asia', 'Asian or Pacific Islander');
			} else if (answer === 'Heterosexual') {
				answerVariations.push('Heterosexual', 'Straight', 'Heterosexual or straight');
			} else if (answer === 'No') {
				answerVariations.push('No', 'I am not', 'No, I am not', "I don't wish to answer");
			}
			
			// First try to find exact matches in dropdown options
			for (const el of elements) {
				const text = (el.innerText || '').trim();
				const textLower = text.toLowerCase();
				
				// Skip if this is too large (not an option)
				if (el.offsetHeight > 60 || el.offsetHeight === 0) continue;
				
				// Check if this element looks like a dropdown option
				const hasRole = el.getAttribute('role') === 'option';
				const inList = el.tagName === 'LI' || (el.parentElement && el.parentElement.tagName === 'UL');
				const isOption = el.className && el.className.includes && el.className.includes('option');
				const hasAriaSelected = el.hasAttribute('aria-selected');
				
				// Must be some kind of option element
				if (!hasRole && !inList && !isOption && !hasAriaSelected) continue;
				
				// Check if text matches any variation
				for (const variant of answerVariations) {
					const variantLower = variant.toLowerCase();
					if (text === variant || textLower === variantLower || 
					    (textLower.includes(variantLower) && text.length < 50)) {
						try {
							el.click();
							return true;
						} catch (e) {
							console.log('Failed to click:', e);
						}
					}
				}
			}
			
			// If no exact match found, try partial matching for specific cases
			if (answer === 'No') {
				for (const el of elements) {
					const text = (el.innerText || '').trim();
					if (text.includes("don't wish") || text.includes("not wish") || 
					    text === "No" || text === "I am not") {
						if (el.offsetHeight < 60 && el.offsetHeight > 0) {
							try {
								el.click();
								return true;
							} catch (e) {}
						}
					}
				}
			}
			
			return false;
		}
	`, answer)
	
	// Selection complete
}