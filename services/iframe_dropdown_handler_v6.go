package services

import (
	"fmt"
	"strings"
	
	"github.com/playwright-community/playwright-go"
)

// HandleIframeDropdownsV6 - Ultra-fast dropdown handling without any timeouts
func HandleIframeDropdownsV6(iframe playwright.FrameLocator, userData *UserProfileData) error {
	// Find all actual dropdown elements at once
	dropdowns := findDropdownsFast(iframe)
	
	unknownQuestions := []string{}
	
	for _, dropdown := range dropdowns {
		// Skip non-questions
		if isNotAQuestion(dropdown.Question) {
			continue
		}
		
		answer := determineIframeDropdownValue(dropdown.Question, userData)
		if answer == "" {
			unknownQuestions = append(unknownQuestions, dropdown.Question)
		} else {
			fillDropdownInstantly(iframe, dropdown, answer)
		}
	}
	
	if len(unknownQuestions) > 0 {
		questionsStr := strings.Join(unknownQuestions, " | ")
		return fmt.Errorf("Unable to fill %d fields. Please provide answers for: [%s]", 
			len(unknownQuestions), questionsStr)
	}
	
	return nil
}

type QuickDropdown struct {
	Element  playwright.Locator
	Question string
	Type     string
}

func findDropdownsFast(iframe playwright.FrameLocator) []QuickDropdown {
	var dropdowns []QuickDropdown
	seen := make(map[string]bool)
	
	// Use JavaScript to find all dropdowns at once - much faster
	allDropdowns, _ := iframe.Locator("div:visible").EvaluateAll(`
		elements => elements.map(el => {
			// Check if this element looks like a dropdown
			const text = el.innerText || '';
			if (!text.includes('Select...')) return null;
			
			// Check size
			if (el.offsetHeight > 100) return null;
			
			// Find associated label
			let question = '';
			
			// Check for label in parent
			let parent = el.parentElement;
			while (parent && !question) {
				const labels = parent.querySelectorAll('label');
				if (labels.length > 0) {
					question = labels[0].innerText;
					break;
				}
				// Check for text with * or ?
				const prevSibling = el.previousElementSibling;
				if (prevSibling) {
					const sibText = prevSibling.innerText || '';
					if (sibText.includes('*') || sibText.includes('?')) {
						question = sibText;
						break;
					}
				}
				parent = parent.parentElement;
			}
			
			if (!question) return null;
			
			return {
				question: question.trim(),
				index: Array.from(elements).indexOf(el)
			};
		}).filter(Boolean)
	`)
	
	// Process the JavaScript results
	if dropdownData, ok := allDropdowns.([]interface{}); ok {
		divs, _ := iframe.Locator("div:visible").All()
		
		for _, item := range dropdownData {
			if data, ok := item.(map[string]interface{}); ok {
				question := data["question"].(string)
				// Handle both int and float64 types for index
				var index int
				switch v := data["index"].(type) {
				case float64:
					index = int(v)
				case int:
					index = v
				default:
					continue
				}
				
				if !seen[question] && index < len(divs) {
					seen[question] = true
					dropdowns = append(dropdowns, QuickDropdown{
						Element:  divs[index],
						Question: question,
						Type:     "div",
					})
				}
			}
		}
	}
	
	// Also get HTML selects
	selects, _ := iframe.Locator("select:visible").All()
	for _, sel := range selects {
		question := getSelectQuestion(iframe, sel)
		if question != "" && !seen[question] {
			seen[question] = true
			dropdowns = append(dropdowns, QuickDropdown{
				Element:  sel,
				Question: question,
				Type:     "select",
			})
		}
	}
	
	return dropdowns
}

func getSelectQuestion(iframe playwright.FrameLocator, sel playwright.Locator) string {
	// Quick ID-based label lookup
	id, _ := sel.GetAttribute("id")
	if id != "" {
		label := iframe.Locator(fmt.Sprintf("label[for='%s']", id)).First()
		if text, _ := label.TextContent(); text != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func isNotAQuestion(question string) bool {
	q := strings.ToLower(question)
	return q == "" || 
		strings.Contains(q, "first name") ||
		strings.Contains(q, "last name") ||
		strings.Contains(q, "apply for this job") ||
		strings.Contains(q, "autofill") ||
		len(question) < 5 ||
		// Skip demographic questions - these will be handled by Stripe handler
		strings.Contains(q, "sexual orientation") ||
		strings.Contains(q, "transgender") ||
		strings.Contains(q, "disability") ||
		strings.Contains(q, "chronic condition") ||
		strings.Contains(q, "veteran") ||
		strings.Contains(q, "armed forces")
}

func fillDropdownInstantly(iframe playwright.FrameLocator, dropdown QuickDropdown, answer string) {
	if dropdown.Type == "select" {
		// HTML select - direct selection
		fillSelectInstantly(dropdown.Element, answer)
	} else {
		// Div dropdown - click and select immediately
		fillDivInstantly(iframe, dropdown.Element, answer)
	}
}

func fillSelectInstantly(sel playwright.Locator, answer string) {
	// Get all options in one go
	optionData, _ := sel.Locator("option").EvaluateAll(`
		options => options.map(opt => ({
			text: opt.innerText.trim(),
			value: opt.value
		}))
	`)
	
	if options, ok := optionData.([]interface{}); ok {
		// Try exact match first
		for _, opt := range options {
			if data, ok := opt.(map[string]interface{}); ok {
				text := data["text"].(string)
				value := data["value"].(string)
				
				if matchesAnswer(text, answer) {
					sel.SelectOption(playwright.SelectOptionValues{Values: &[]string{value}})
					return
				}
			}
		}
	}
}

func fillDivInstantly(iframe playwright.FrameLocator, elem playwright.Locator, answer string) {
	// Click immediately without any timeout
	err := elem.Click()
	if err != nil {
		return
	}
	
	// Try to select with JavaScript for speed
	result, _ := iframe.Locator("*:visible").EvaluateAll(`
		(elements, answer) => {
			const answerLower = answer.toLowerCase();
			// Find and click the matching option
			for (const el of elements) {
				const text = (el.innerText || '').trim();
				const textLower = text.toLowerCase();
				
				// Check various matching patterns
				if (text && (
					text === answer ||
					textLower === answerLower ||
					(answer === 'Yes' && (text === 'Yes' || text.startsWith('Yes'))) ||
					(answer === 'No' && (text === 'No' || text.startsWith('No'))) ||
					(answer === 'United States' && (text === 'US' || text === 'USA' || text === 'United States of America')) ||
					(answer === 'Heterosexual or straight' && (text === 'Straight' || text === 'Heterosexual' || textLower.includes('straight') || textLower.includes('heterosexual')))
				)) {
					// Check if it's an option (small element, not a container)
					if (el.offsetHeight < 60 && el.offsetHeight > 0) {
						el.click();
						return true;
					}
				}
			}
			return false;
		}
	`, answer)
	
	if result != true {
		// Fallback: try direct locator
		variations := getQuickVariations(answer)
		for _, variant := range variations {
			opt := iframe.Locator(fmt.Sprintf("*:text-is('%s'):visible", variant)).First()
			if count, _ := opt.Count(); count > 0 {
				opt.Click()
				return
			}
		}
	}
}

func matchesAnswer(text, answer string) bool {
	textLower := strings.ToLower(text)
	answerLower := strings.ToLower(answer)
	
	// Exact match
	if text == answer || textLower == answerLower {
		return true
	}
	
	// Special cases
	if answer == "United States" && (text == "US" || text == "USA") {
		return true
	}
	if answer == "Yes" && strings.HasPrefix(text, "Yes") {
		return true
	}
	if answer == "No" && strings.HasPrefix(text, "No") {
		return true
	}
	if strings.Contains(answerLower, "straight") && strings.Contains(textLower, "straight") {
		return true
	}
	
	return false
}

func getQuickVariations(answer string) []string {
	switch answer {
	case "United States":
		return []string{"US", "USA", "United States", "United States of America"}
	case "Heterosexual or straight":
		return []string{"Heterosexual or straight", "Straight", "Heterosexual", "Heterosexual/Straight"}
	case "Yes":
		return []string{"Yes", "Yes, I am"}
	case "No":
		return []string{"No", "No, I am not", "I am not"}
	default:
		return []string{answer}
	}
}