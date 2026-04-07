package services

import (
	"os"
	"testing"
)

func TestClassifyWriteIntent(t *testing.T) {
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
