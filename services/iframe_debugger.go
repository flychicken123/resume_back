package services

import (
	"log"
	"github.com/playwright-community/playwright-go"
)

// DebugIframeContent helps understand what's inside the iframe
func DebugIframeContent(iframe playwright.FrameLocator, page playwright.Page) {
	log.Println("=== DEBUGGING IFRAME CONTENT ===")
	
	// Try to get all input elements of any type
	allInputs, _ := iframe.Locator("input").All()
	log.Printf("Total inputs (all types): %d", len(allInputs))
	
	// Check for visible inputs
	visibleInputs, _ := iframe.Locator("input:visible").All()
	log.Printf("Visible inputs: %d", len(visibleInputs))
	
	// Log first few inputs for debugging
	for i, input := range visibleInputs {
		if i >= 3 {
			break
		}
		inputType, _ := input.GetAttribute("type")
		name, _ := input.GetAttribute("name")
		placeholder, _ := input.GetAttribute("placeholder")
		isVisible, _ := input.IsVisible()
		log.Printf("  Input %d: type=%s, name=%s, placeholder=%s, visible=%v", 
			i, inputType, name, placeholder, isVisible)
	}
	
	// Check for select elements
	selects, _ := iframe.Locator("select").All()
	log.Printf("Total select elements: %d", len(selects))
	
	// Check for divs with class containing 'select'
	selectDivs, _ := iframe.Locator("div[class*='select']").All()
	log.Printf("Divs with 'select' in class: %d", len(selectDivs))
	
	// Check for any buttons
	buttons, _ := iframe.Locator("button").All()
	log.Printf("Total buttons: %d", len(buttons))
	
	// Check for labels (to understand form structure)
	labels, _ := iframe.Locator("label").All()
	log.Printf("Total labels: %d", len(labels))
	
	// Log first few labels
	for i, label := range labels {
		if i >= 3 {
			break
		}
		text, _ := label.TextContent()
		log.Printf("  Label %d: %s", i, text)
	}
	
	// Check if content is loaded using JavaScript
	result, _ := page.Evaluate(`
		() => {
			const iframe = document.querySelector('iframe');
			if (!iframe) return 'No iframe found';
			
			try {
				const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
				if (!iframeDoc) return 'Cannot access iframe document';
				
				const inputs = iframeDoc.querySelectorAll('input');
				const selects = iframeDoc.querySelectorAll('select');
				const buttons = iframeDoc.querySelectorAll('button');
				const divs = iframeDoc.querySelectorAll('div');
				
				return {
					inputs: inputs.length,
					selects: selects.length,
					buttons: buttons.length,
					divs: divs.length,
					bodyHTML: iframeDoc.body ? iframeDoc.body.innerHTML.substring(0, 500) : 'No body'
				};
			} catch(e) {
				return 'Cross-origin iframe - cannot access: ' + e.message;
			}
		}
	`)
	
	log.Printf("JavaScript iframe check result: %v", result)
	
	// Try alternative: Check if form might be loading dynamically
	log.Println("Checking for dynamic content indicators...")
	spinners, _ := iframe.Locator("[class*='spinner'], [class*='loading'], [class*='loader']").All()
	log.Printf("Loading indicators found: %d", len(spinners))
	
	// Check for any text content
	bodyText, _ := iframe.Locator("body").First().TextContent()
	if len(bodyText) > 200 {
		bodyText = bodyText[:200] + "..."
	}
	log.Printf("Iframe body text (first 200 chars): %s", bodyText)
	
	log.Println("=== END IFRAME DEBUG ===")
}