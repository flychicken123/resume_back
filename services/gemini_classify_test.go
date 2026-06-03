package services

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClassifyWriteIntent(t *testing.T) {
	if os.Getenv("GEMINI_LIVE_TESTS") != "1" {
		t.Skip("GEMINI_LIVE_TESTS=1 not set, skipping live API test")
	}
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set, skipping live API test")
	}

	tests := []struct {
		message string
		want    bool
	}{
		{"move Staff Machine Learning Engineer to rejected", true},
		{"track my application at Netflix", true},
		{"update my skills to include Python", true},
		{"mark Google interview as screening", true},
		{"delete that entry", true},

		{"I got rejected from Google", false},
		{"what are my applications?", false},
		{"hello", false},
		{"how many jobs did I apply to?", false},
		{"what is my current status?", false},
		{"tell me about remote jobs", false},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			got := ClassifyWriteIntent(tt.message)
			if got != tt.want {
				t.Errorf("ClassifyWriteIntent(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestBuildJobClassificationPrompt_RemovesInvalidUTF8(t *testing.T) {
	title := "Engineer " + string([]byte{0xff, 0xfe})
	description := "Build systems " + string([]byte{0xc3, 0x28}) + " at scale"

	prompt := BuildJobClassificationPrompt(title, description)

	if !utf8.ValidString(prompt) {
		t.Fatal("expected classification prompt to be valid UTF-8")
	}
	if strings.Contains(prompt, string([]byte{0xff})) || strings.Contains(prompt, string([]byte{0xfe})) {
		t.Fatal("expected invalid UTF-8 bytes to be removed")
	}
}

func TestBuildJobClassificationPrompt_TruncatesAtRuneBoundary(t *testing.T) {
	description := strings.Repeat("a", 1999) + "你" + "tail"

	prompt := BuildJobClassificationPrompt("Engineer", description)

	if !utf8.ValidString(prompt) {
		t.Fatal("expected classification prompt to stay valid UTF-8 after truncation")
	}
	if !strings.Contains(prompt, "你...") {
		t.Fatal("expected long description to be truncated")
	}
	if strings.Contains(prompt, "tail") {
		t.Fatal("expected content after the rune-safe limit to be removed")
	}
}

func TestSanitizeGeminiPrompt_RemovesInvalidUTF8(t *testing.T) {
	prompt := "hello " + string([]byte{0xff}) + " world"

	got := sanitizeGeminiPrompt(prompt)

	if !utf8.ValidString(got) {
		t.Fatal("expected sanitized prompt to be valid UTF-8")
	}
	if got != "hello  world" {
		t.Fatalf("sanitizeGeminiPrompt() = %q", got)
	}
}

func TestValidateJobClassificationResult_RejectsNonPersistableResult(t *testing.T) {
	if err := validateJobClassificationResult(CareerFieldUnknown, nil, "mid"); err == nil {
		t.Fatal("expected UNKNOWN career field to be rejected")
	}
}

func TestValidateJobClassificationResult_AllowsEmptySkills(t *testing.T) {
	if err := validateJobClassificationResult(CareerFieldOther, nil, "mid"); err != nil {
		t.Fatalf("expected empty skills to be persistable, got %v", err)
	}
}

func TestParseJobClassificationResponse_NormalizesCareerFieldAliasesAndSkillKeys(t *testing.T) {
	raw := "Here is the JSON:\n```JSON\n{\"careerField\":\"Manufacturing Engineering\",\"requiredSkills\":[\"CNC Machining\",\"GD&T\"],\"seniorityLevel\":\"Lead\"}\n```"

	field, skills, seniority := ParseJobClassificationResponse(raw)

	if field != CareerFieldOperations {
		t.Fatalf("expected manufacturing engineering to normalize to %s, got %s", CareerFieldOperations, field)
	}
	if seniority != "lead" {
		t.Fatalf("expected lead seniority, got %q", seniority)
	}
	if len(skills) != 2 || skills[0] != "cnc machining" || skills[1] != "gd&t" {
		t.Fatalf("expected normalized skills, got %#v", skills)
	}
}

func TestParseJobClassificationResponse_UsesOtherForOutOfTaxonomyFields(t *testing.T) {
	raw := `{"career_field":"Legal","skills":"contracts, compliance","seniority":"senior"}`

	field, skills, seniority := ParseJobClassificationResponse(raw)

	if field != CareerFieldOther {
		t.Fatalf("expected legal to normalize to %s, got %s", CareerFieldOther, field)
	}
	if seniority != "senior" {
		t.Fatalf("expected senior seniority, got %q", seniority)
	}
	if len(skills) != 2 || skills[0] != "contracts" || skills[1] != "compliance" {
		t.Fatalf("expected comma-separated skills to parse, got %#v", skills)
	}
}
