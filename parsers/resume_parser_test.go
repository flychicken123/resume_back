package parsers

import (
	"strings"
	"testing"
)

func TestResumeParser_Basic(t *testing.T) {
	parser := NewResumeParser()
	
	sampleResume := `
John Doe
john.doe@email.com
(555) 123-4567

SUMMARY
Experienced software engineer with 5+ years developing web applications.

EXPERIENCE
Software Engineer at Google
June 2020 - Present
• Developed scalable web applications using Go and React
• Led team of 4 developers on critical projects
• Improved system performance by 40%

Junior Developer at Startup Inc
Jan 2018 - May 2020
• Built RESTful APIs using Python and Django
• Collaborated with cross-functional teams

EDUCATION
Bachelor of Science in Computer Science
Stanford University
2014 - 2018

SKILLS
Go, Python, JavaScript, React, Docker, Kubernetes
`

	result, err := parser.Parse(sampleResume)
	if err != nil {
		t.Fatalf("Parser failed: %v", err)
	}

	// Test basic contact info
	if result.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", result.Name)
	}

	if result.Email != "john.doe@email.com" {
		t.Errorf("Expected email 'john.doe@email.com', got '%s'", result.Email)
	}

	if result.Phone != "(555) 123-4567" {
		t.Errorf("Expected phone '(555) 123-4567', got '%s'", result.Phone)
	}

	// Test sections
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}

	// Test experience
	if len(result.Experience) == 0 {
		t.Error("Should have extracted experience entries")
	}

	if len(result.Experience) > 0 {
		exp := result.Experience[0]
		if !strings.Contains(exp.Role, "Software Engineer") {
			t.Errorf("Expected role to contain 'Software Engineer', got '%s'", exp.Role)
		}
		if !strings.Contains(exp.Company, "Google") {
			t.Errorf("Expected company to contain 'Google', got '%s'", exp.Company)
		}
	}

	// Test education
	if len(result.Education) == 0 {
		t.Error("Should have extracted education entries")
	}

	// Test skills
	if len(result.Skills) == 0 {
		t.Error("Should have extracted skills")
	}
}

func TestResumeParser_EmptyInput(t *testing.T) {
	parser := NewResumeParser()
	
	_, err := parser.Parse("")
	if err == nil {
		t.Error("Expected error for empty input")
	}
}

func TestResumeParser_ContactExtraction(t *testing.T) {
	parser := NewResumeParser()
	
	tests := []struct {
		input    string
		email    string
		phone    string
	}{
		{
			input: "Contact: test@example.com, phone: +1-555-123-4567",
			email: "test@example.com",
			phone: "+1 (555) 123-4567",
		},
		{
			input: "Email: user@domain.org\nPhone: (987) 654-3210",
			email: "user@domain.org", 
			phone: "(987) 654-3210",
		},
	}

	for _, test := range tests {
		result, err := parser.Parse(test.input)
		if err != nil {
			t.Fatalf("Parser failed for input '%s': %v", test.input, err)
		}

		if result.Email != test.email {
			t.Errorf("Expected email '%s', got '%s' for input '%s'", test.email, result.Email, test.input)
		}

		if result.Phone != test.phone {
			t.Errorf("Expected phone '%s', got '%s' for input '%s'", test.phone, result.Phone, test.input)
		}
	}
}