package utils

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"baliance.com/gooxml/document"
)

func GenerateWordFile(content string, filepath string) error {
	doc := document.New()
	doc.AddParagraph().AddRun().AddText(content)
	return doc.SaveToFile(filepath)
}

func GenerateWordFileFromTemplate(templateName string, userData map[string]string, outputPath string) error {
	// Map template names to file names
	templateMap := map[string]string{
		"color-block":            "Color block resume.docx",
		"industry-manager":       "Industry manager resume.docx",
		"social-media-marketing": "Social media marketing resume.docx",
	}

	templateFile, exists := templateMap[templateName]
	if !exists {
		return fmt.Errorf("template '%s' not found", templateName)
	}

	// Load the template file
	templatePath := filepath.Join("./templates", templateFile)
	log.Printf("Loading template from: %s", templatePath)

	doc, err := document.Open(templatePath)
	if err != nil {
		log.Printf("Failed to open template, creating basic template: %v", err)
		doc = createBasicTemplate(templateName)
	}

	log.Printf("Template loaded successfully. Paragraphs: %d, Tables: %d", len(doc.Paragraphs()), len(doc.Tables()))

	// If template has no content, create a proper template
	if len(doc.Paragraphs()) == 0 && len(doc.Tables()) == 0 {
		log.Printf("Template appears empty, creating proper template structure")
		doc = createProperTemplate(templateName)
	}

	// Replace placeholders in the document
	replacePlaceholders(doc, userData)

	// Save the modified document
	log.Printf("Saving resume to: %s", outputPath)
	return doc.SaveToFile(outputPath)
}

func createBasicTemplate(templateName string) *document.Document {
	doc := document.New()

	// Add basic structure with placeholders
	headerPara := doc.AddParagraph()
	headerPara.AddRun().AddText("{{NAME}}")

	contactPara := doc.AddParagraph()
	contactPara.AddRun().AddText("{{EMAIL}} • {{PHONE}}")

	summaryHeader := doc.AddParagraph()
	summaryHeader.AddRun().AddText("SUMMARY")
	summaryContent := doc.AddParagraph()
	summaryContent.AddRun().AddText("{{SUMMARY}}")

	expHeader := doc.AddParagraph()
	expHeader.AddRun().AddText("EXPERIENCE")
	expContent := doc.AddParagraph()
	expContent.AddRun().AddText("{{EXPERIENCE}}")

	eduHeader := doc.AddParagraph()
	eduHeader.AddRun().AddText("EDUCATION")
	eduContent := doc.AddParagraph()
	eduContent.AddRun().AddText("{{EDUCATION}}")

	skillsHeader := doc.AddParagraph()
	skillsHeader.AddRun().AddText("SKILLS")
	skillsContent := doc.AddParagraph()
	skillsContent.AddRun().AddText("{{SKILLS}}")

	return doc
}

func createProperTemplate(templateName string) *document.Document {
	doc := document.New()

	switch templateName {
	case "color-block":
		// Color Block template structure
		headerPara := doc.AddParagraph()
		headerPara.AddRun().AddText("{{NAME}}")

		contactPara := doc.AddParagraph()
		contactPara.AddRun().AddText("{{EMAIL}} • {{PHONE}}")

		summaryHeader := doc.AddParagraph()
		summaryHeader.AddRun().AddText("SUMMARY")
		summaryContent := doc.AddParagraph()
		summaryContent.AddRun().AddText("{{SUMMARY}}")

		expHeader := doc.AddParagraph()
		expHeader.AddRun().AddText("EXPERIENCE")
		expContent := doc.AddParagraph()
		expContent.AddRun().AddText("{{EXPERIENCE}}")

		eduHeader := doc.AddParagraph()
		eduHeader.AddRun().AddText("EDUCATION")
		eduContent := doc.AddParagraph()
		eduContent.AddRun().AddText("{{EDUCATION}}")

		skillsHeader := doc.AddParagraph()
		skillsHeader.AddRun().AddText("SKILLS")
		skillsContent := doc.AddParagraph()
		skillsContent.AddRun().AddText("{{SKILLS}}")

	case "industry-manager":
		// Industry Manager template structure
		headerPara := doc.AddParagraph()
		headerPara.AddRun().AddText("{{NAME}}")

		contactPara := doc.AddParagraph()
		contactPara.AddRun().AddText("{{EMAIL}} • {{PHONE}}")

		summaryHeader := doc.AddParagraph()
		summaryHeader.AddRun().AddText("PROFESSIONAL SUMMARY")
		summaryContent := doc.AddParagraph()
		summaryContent.AddRun().AddText("{{SUMMARY}}")

		expHeader := doc.AddParagraph()
		expHeader.AddRun().AddText("EXPERIENCE")
		expContent := doc.AddParagraph()
		expContent.AddRun().AddText("{{EXPERIENCE}}")

		eduHeader := doc.AddParagraph()
		eduHeader.AddRun().AddText("EDUCATION")
		eduContent := doc.AddParagraph()
		eduContent.AddRun().AddText("{{EDUCATION}}")

		skillsHeader := doc.AddParagraph()
		skillsHeader.AddRun().AddText("SKILLS")
		skillsContent := doc.AddParagraph()
		skillsContent.AddRun().AddText("{{SKILLS}}")

	case "social-media-marketing":
		// Social Media Marketing template structure
		headerPara := doc.AddParagraph()
		headerPara.AddRun().AddText("{{NAME}}")

		subtitlePara := doc.AddParagraph()
		subtitlePara.AddRun().AddText("Creative Marketing Professional")

		contactPara := doc.AddParagraph()
		contactPara.AddRun().AddText("{{EMAIL}} • {{PHONE}}")

		summaryHeader := doc.AddParagraph()
		summaryHeader.AddRun().AddText("CREATIVE SUMMARY")
		summaryContent := doc.AddParagraph()
		summaryContent.AddRun().AddText("{{SUMMARY}}")

		expHeader := doc.AddParagraph()
		expHeader.AddRun().AddText("EXPERIENCE")
		expContent := doc.AddParagraph()
		expContent.AddRun().AddText("{{EXPERIENCE}}")

		eduHeader := doc.AddParagraph()
		eduHeader.AddRun().AddText("EDUCATION")
		eduContent := doc.AddParagraph()
		eduContent.AddRun().AddText("{{EDUCATION}}")

		skillsHeader := doc.AddParagraph()
		skillsHeader.AddRun().AddText("SKILLS")
		skillsContent := doc.AddParagraph()
		skillsContent.AddRun().AddText("{{SKILLS}}")

	default:
		// Default template
		return createBasicTemplate(templateName)
	}

	return doc
}

func replacePlaceholders(doc *document.Document, userData map[string]string) {
	// Common placeholders to replace
	placeholders := map[string]string{
		"{{NAME}}":       userData["name"],
		"{{EMAIL}}":      userData["email"],
		"{{PHONE}}":      userData["phone"],
		"{{SUMMARY}}":    userData["summary"],
		"{{EXPERIENCE}}": userData["experience"],
		"{{EDUCATION}}":  userData["education"],
		"{{SKILLS}}":     userData["skills"],
		"{{POSITION}}":   userData["position"],
	}

	replacements := 0

	// Replace text in all paragraphs
	for _, para := range doc.Paragraphs() {
		for _, run := range para.Runs() {
			text := run.Text()
			for placeholder, value := range placeholders {
				if strings.Contains(text, placeholder) {
					newText := strings.ReplaceAll(text, placeholder, value)
					run.Clear()
					run.AddText(newText)
					replacements++
					log.Printf("Replaced %s with %s", placeholder, value)
				}
			}
		}
	}

	// Replace text in all tables
	for _, table := range doc.Tables() {
		for _, row := range table.Rows() {
			for _, cell := range row.Cells() {
				for _, para := range cell.Paragraphs() {
					for _, run := range para.Runs() {
						text := run.Text()
						for placeholder, value := range placeholders {
							if strings.Contains(text, placeholder) {
								newText := strings.ReplaceAll(text, placeholder, value)
								run.Clear()
								run.AddText(newText)
								replacements++
								log.Printf("Replaced %s with %s", placeholder, value)
							}
						}
					}
				}
			}
		}
	}

	log.Printf("Total replacements made: %d", replacements)
}
