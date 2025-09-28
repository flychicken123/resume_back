package parsers

import (
	"path/filepath"
	"strings"
	"testing"

	"baliance.com/gooxml/document"
	"baliance.com/gooxml/schema/soo/wml"
)

func TestExtractFromDocxCapturesHeaderContact(t *testing.T) {
	doc := document.New()

	header := doc.AddHeader()
	headerPara := header.AddParagraph()
	headerRun := headerPara.AddRun()
	headerRun.AddText("Yu Han | yuhan@example.com | (555) 123-4567")

	doc.BodySection().SetHeader(header, wml.ST_HdrFtrDefault)

	bodyPara := doc.AddParagraph()
	bodyPara.AddRun().AddText("Professional summary goes here.")

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "contact.docx")
	if err := doc.SaveToFile(tmpFile); err != nil {
		t.Fatalf("failed to save docx: %v", err)
	}

	extractor := NewPDFExtractor()
	text, err := extractor.ExtractFromDocx(tmpFile)
	if err != nil {
		t.Fatalf("expected extraction to succeed, got error: %v", err)
	}

	if !strings.Contains(text, "yuhan@example.com") {
		t.Fatalf("expected email to be present in extracted text; got %q", text)
	}
	if !strings.Contains(text, "(555) 123-4567") {
		t.Fatalf("expected phone to be present in extracted text; got %q", text)
	}
}
