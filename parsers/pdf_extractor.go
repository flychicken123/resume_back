package parsers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

// PDFExtractor handles extracting text from PDF files
type PDFExtractor struct{}

// NewPDFExtractor creates a new PDF text extractor
func NewPDFExtractor() *PDFExtractor {
	return &PDFExtractor{}
}

// ExtractText extracts text from a PDF file using multiple fallback methods
func (e *PDFExtractor) ExtractText(filePath string) (string, error) {
	if text, err := e.extractWithPdfToText(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	if text, err := e.extractWithGoPDF(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	if text, err := e.extractWithPython(filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

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

	// Run pdftotext with better formatting options
	// -layout: Maintain layout and spacing (better for structured resumes)
	// -nopgbrk: Don't insert page breaks
	cmd := exec.Command("pdftotext", "-layout", "-nopgbrk", filePath, tempFile)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %v", err)
	}

	// Read extracted text
	content, err := os.ReadFile(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted text: %v", err)
	}

	// Clean up text
	text := string(content)
	text = e.cleanupText(text)

	return text, nil
}

func (e *PDFExtractor) extractWithGoPDF(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("go pdf extractor failed to open: %w", err)
	}
	defer f.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("go pdf extractor failed to read: %w", err)
	}

	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return "", fmt.Errorf("go pdf extractor failed to buffer: %w", err)
	}

	text := e.cleanupText(buf.String())
	text = strings.ReplaceAll(text, "\u0000", "")

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("go pdf extractor returned empty text")
	}

	return text, nil
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

// cleanupText fixes common PDF extraction issues
func (e *PDFExtractor) cleanupText(text string) string {
	// Fix words with spaces between letters (e.g., "M a n a g e m e n t" -> "Management")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// Only apply to lines that look like they have this issue
		if strings.Count(line, " ") > len(line)/3 && len(line) > 10 {
			// Check if line has pattern of single chars with spaces
			words := strings.Fields(line)
			hasSpacedWords := false
			singleCharCount := 0

			for _, word := range words {
				if len(word) == 1 && word != "•" && word != "-" && word != "|" && word != "&" {
					singleCharCount++
					if singleCharCount > 3 {
						hasSpacedWords = true
						break
					}
				}
			}

			if hasSpacedWords {
				// Rebuild the line by combining single characters
				var fixedWords []string
				var currentWord []string

				for _, word := range words {
					if len(word) == 1 && word != "•" && word != "-" && word != "|" && word != "," && word != "." && word != "&" {
						currentWord = append(currentWord, word)
					} else {
						if len(currentWord) > 0 {
							fixedWords = append(fixedWords, strings.Join(currentWord, ""))
							currentWord = []string{}
						}
						fixedWords = append(fixedWords, word)
					}
				}
				if len(currentWord) > 0 {
					fixedWords = append(fixedWords, strings.Join(currentWord, ""))
				}

				lines[i] = strings.Join(fixedWords, " ")
			}
		}
	}

	return strings.Join(lines, "\n")
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
