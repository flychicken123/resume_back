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
