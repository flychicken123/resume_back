package utils

import (
	"baliance.com/gooxml/document"
)

func GenerateWordFile(content string, filepath string) error {
	doc := document.New()
	doc.AddParagraph().AddRun().AddText(content)
	return doc.SaveToFile(filepath)
}
