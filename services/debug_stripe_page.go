package services

import (
	"fmt"
	"log"
	"github.com/playwright-community/playwright-go"
)

// DebugStripePage helps understand the page structure
func DebugStripePage(url string) {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), // Run with UI for debugging
	})
	if err != nil {
		log.Fatal(err)
	}
	defer browser.Close()

	context, err := browser.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		log.Fatal(err)
	}

	// Navigate to the page
	if _, err := page.Goto(url); err != nil {
		log.Fatal(err)
	}

	// Wait for page to load
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})

	log.Println("=== PAGE STRUCTURE ANALYSIS ===")
	
	// Check for iframes
	iframeCount, _ := page.Locator("iframe").Count()
	log.Printf("Total iframes on page: %d", iframeCount)
	
	if iframeCount > 0 {
		// Get iframe details
		iframes, _ := page.Locator("iframe").All()
		for i, iframe := range iframes {
			id, _ := iframe.GetAttribute("id")
			src, _ := iframe.GetAttribute("src")
			title, _ := iframe.GetAttribute("title")
			classes, _ := iframe.GetAttribute("class")
			log.Printf("Iframe %d: id='%s', class='%s', title='%s', src='%s'", i, id, classes, title, src)
		}
		
		// Check Greenhouse iframe specifically
		greenhouseIframe := page.FrameLocator("#grnhse_iframe")
		
		// Check buttons inside iframe
		iframeButtons, _ := greenhouseIframe.Locator("button").All()
		log.Printf("\nButtons INSIDE iframe: %d", len(iframeButtons))
		
		for i, btn := range iframeButtons {
			text, _ := btn.TextContent()
			btnType, _ := btn.GetAttribute("type")
			classes, _ := btn.GetAttribute("class")
			visible, _ := btn.IsVisible()
			log.Printf("  Iframe Button %d: text='%s', type='%s', class='%s', visible=%v", 
				i, text, btnType, classes, visible)
		}
		
		// Look specifically for submit button in iframe
		submitInIframe, _ := greenhouseIframe.Locator("button[type='submit'], button:has-text('Submit'), div.application--submit button").Count()
		log.Printf("\nSubmit buttons found INSIDE iframe: %d", submitInIframe)
	}
	
	// Check buttons outside iframe
	pageButtons, _ := page.Locator("button").All()
	log.Printf("\nButtons on MAIN page (outside iframe): %d", len(pageButtons))
	
	for i, btn := range pageButtons {
		text, _ := btn.TextContent()
		btnType, _ := btn.GetAttribute("type")
		classes, _ := btn.GetAttribute("class")
		visible, _ := btn.IsVisible()
		log.Printf("  Page Button %d: text='%s', type='%s', class='%s', visible=%v", 
			i, text, btnType, classes, visible)
	}
	
	// Look for submit button on main page
	submitOnPage, _ := page.Locator("button[type='submit'], button:has-text('Submit'), div.application--submit button").Count()
	log.Printf("\nSubmit buttons found on MAIN page: %d", submitOnPage)
	
	// Use JavaScript to check both page and iframe
	result, _ := page.Evaluate(`
		() => {
			let info = {
				pageButtons: [],
				iframeButtons: []
			};
			
			// Check main page buttons
			document.querySelectorAll('button').forEach(btn => {
				info.pageButtons.push({
					text: btn.textContent.trim(),
					type: btn.type,
					class: btn.className,
					visible: btn.offsetParent !== null
				});
			});
			
			// Check iframe buttons
			const iframe = document.querySelector('#grnhse_iframe');
			if (iframe && iframe.contentDocument) {
				iframe.contentDocument.querySelectorAll('button').forEach(btn => {
					info.iframeButtons.push({
						text: btn.textContent.trim(),
						type: btn.type,
						class: btn.className,
						visible: btn.offsetParent !== null
					});
				});
			}
			
			return info;
		}
	`)
	
	log.Printf("\n=== JAVASCRIPT ANALYSIS ===")
	log.Printf("Result: %+v", result)
	
	fmt.Println("\nPress Enter to close...")
	fmt.Scanln()
}