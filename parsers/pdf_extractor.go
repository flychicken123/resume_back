package parsers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PDFExtractor handles extracting text from PDF files
type PDFExtractor struct{}

// NewPDFExtractor creates a new PDF text extractor
func NewPDFExtractor() *PDFExtractor {
	return &PDFExtractor{}
}

// ExtractText extracts text from a PDF file using multiple fallback methods
func (e *PDFExtractor) ExtractText(filePath string) (string, error) {
	// Method 1: Try pdftotext (poppler-utils)
	if text, err := e.extractWithPdfToText(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	// Method 2: Try Python script as fallback
	if text, err := e.extractWithPython(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	// Method 3: Try ps2ascii if available
	if text, err := e.extractWithPs2Ascii(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	return "", fmt.Errorf("failed to extract text from PDF using all available methods")
}

// extractWithPdfToText uses pdftotext command (most reliable)
func (e *PDFExtractor) extractWithPdfToText(filePath string) (string, error) {
	// Check if pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", fmt.Errorf("pdftotext not available: %v", err)
	}

	// Create temp file for output
	tempFile := filePath + ".txt"
	defer os.Remove(tempFile)

	// Run pdftotext
	cmd := exec.Command("pdftotext", "-layout", filePath, tempFile)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %v", err)
	}

	// Read extracted text
	content, err := os.ReadFile(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted text: %v", err)
	}

	return string(content), nil
}

// extractWithPython uses the existing Python script
func (e *PDFExtractor) extractWithPython(filePath string) (string, error) {
	// Check if Python script exists
	scriptPath := "parse_resume.py"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", fmt.Errorf("Python script not found: %s", scriptPath)
	}

	// Determine Python executable
	pythonCmd := "python3"
	if _, err := exec.LookPath(pythonCmd); err != nil {
		pythonCmd = "python"
		if _, err := exec.LookPath(pythonCmd); err != nil {
			return "", fmt.Errorf("Python not available")
		}
	}

	// Run Python script
	cmd := exec.Command(pythonCmd, scriptPath, filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Python extraction failed: %v", err)
	}

	// Parse JSON output to get raw_text
	result := struct {
		RawText string `json:"raw_text"`
	}{}

	if err := json.Unmarshal(output, &result); err != nil {
		// If JSON parsing fails, return raw output
		return string(output), nil
	}

	return result.RawText, nil
}

// extractWithPs2Ascii uses ps2ascii as another fallback
func (e *PDFExtractor) extractWithPs2Ascii(filePath string) (string, error) {
	// Check if ps2ascii is available
	if _, err := exec.LookPath("ps2ascii"); err != nil {
		return "", fmt.Errorf("ps2ascii not available: %v", err)
	}

	// Run ps2ascii
	cmd := exec.Command("ps2ascii", filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ps2ascii failed: %v", err)
	}

	return string(output), nil
}

// ExtractFromDocx extracts text from DOCX files (basic implementation)
func (e *PDFExtractor) ExtractFromDocx(filePath string) (string, error) {
	// Try using docx2txt if available
	if _, err := exec.LookPath("docx2txt"); err == nil {
		cmd := exec.Command("docx2txt", filePath, "-")
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("docx2txt failed: %v", err)
		}
		return string(output), nil
	}

	// Try using pandoc if available
	if _, err := exec.LookPath("pandoc"); err == nil {
		cmd := exec.Command("pandoc", "-t", "plain", filePath)
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("pandoc failed: %v", err)
		}
		return string(output), nil
	}

	return "", fmt.Errorf("no DOCX extraction tools available (tried docx2txt, pandoc)")
}

// ExtractFromFile determines file type and extracts text accordingly
func (e *PDFExtractor) ExtractFromFile(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".pdf":
		return e.ExtractText(filePath)
	case ".docx":
		return e.ExtractFromDocx(filePath)
	case ".txt":
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read text file: %v", err)
		}
		return string(content), nil
	case ".doc":
		// Try to convert .doc to text using antiword or catdoc
		if text, err := e.extractFromDoc(filePath); err == nil {
			return text, nil
		}
		return "", fmt.Errorf("unsupported file format: %s (old .doc format)", ext)
	default:
		return "", fmt.Errorf("unsupported file format: %s", ext)
	}
}

// extractFromDoc extracts text from old .doc files
func (e *PDFExtractor) extractFromDoc(filePath string) (string, error) {
	// Try antiword first
	if _, err := exec.LookPath("antiword"); err == nil {
		cmd := exec.Command("antiword", filePath)
		output, err := cmd.Output()
		if err == nil {
			return string(output), nil
		}
	}

	// Try catdoc
	if _, err := exec.LookPath("catdoc"); err == nil {
		cmd := exec.Command("catdoc", filePath)
		output, err := cmd.Output()
		if err == nil {
			return string(output), nil
		}
	}

	return "", fmt.Errorf("no .doc extraction tools available (tried antiword, catdoc)")
}