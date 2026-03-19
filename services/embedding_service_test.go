package services

import (
	"context"
	"strings"
	"testing"

	"resumeai/models"
)

func TestNewEmbeddingServiceRejectsEmptyKey(t *testing.T) {
	svc, err := NewEmbeddingService("")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service for empty API key, got %v", svc)
	}
	if !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("error message should mention GEMINI_API_KEY, got: %s", err.Error())
	}
}

func TestHTMLTagStrippingRegex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>hello</b> world", " hello  world"},
		{`<p class="x">text</p>`, " text "},
		{"no tags here", "no tags here"},
		{"", ""},
		{"<br/><p></p>", "   "},
	}
	for _, tt := range tests {
		got := htmlTagRe.ReplaceAllString(tt.input, " ")
		if got != tt.want {
			t.Errorf("htmlTagRe.ReplaceAllString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEmbedJobPostingRejectsEmptyPosting(t *testing.T) {
	// client is nil — if the text guard doesn't fire first, the call panics.
	svc := &EmbeddingService{client: nil}
	posting := &models.JobPosting{} // all zero-value string fields
	_, err := svc.EmbedJobPosting(context.Background(), posting)
	if err == nil {
		t.Fatal("expected error for empty job posting, got nil")
	}
}

func TestEmbedJobPostingRejectsHTMLOnlyPosting(t *testing.T) {
	svc := &EmbeddingService{client: nil}
	posting := &models.JobPosting{
		Description: "<p></p><br/>  ",
	}
	_, err := svc.EmbedJobPosting(context.Background(), posting)
	if err == nil {
		t.Fatal("expected error when posting text is HTML-only whitespace, got nil")
	}
}

func TestEmbedResumeContentRejectsEmptyInput(t *testing.T) {
	svc := &EmbeddingService{client: nil}
	_, err := svc.EmbedResumeContent(context.Background(), ResumeJobMatchInput{})
	if err == nil {
		t.Fatal("expected error for empty resume input, got nil")
	}
}
