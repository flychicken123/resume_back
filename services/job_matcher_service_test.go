package services

import (
	"testing"

	"resumeai/models"
)

func TestComputeMatchScore(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Senior Backend Engineer",
		Description: "We build scalable systems in Go and AWS, working with microservices.",
		Department:  "Software Engineering",
		RemoteType:  "Remote",
		Location:    "Remote - US",
	}

	skills := []string{"go", "aws"}
	keywords := []string{"scalable", "systems", "microservices"}
	resumeText := "seasoned backend engineer focused on remote distributed systems"

	score := computeMatchScore(job, "backend engineer", skills, keywords, resumeText, "remote")
	if score <= 0 {
		t.Fatalf("expected positive score, got %.2f", score)
	}
}

func TestComputeMatchScoreRequiresSignal(t *testing.T) {
	job := &models.JobPosting{Title: "Office Manager", Description: "assist with office tasks"}
	score := computeMatchScore(job, "backend", nil, nil, "", "")
	if score != 0 {
		t.Fatalf("expected zero score when no matches, got %.2f", score)
	}
}

func TestExtractKeywords(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog while building scalable software systems"
	keywords := extractKeywords(text, 5)
	if len(keywords) == 0 {
		t.Fatal("expected keywords extracted")
	}
	for _, kw := range keywords {
		if kw == "the" || len(kw) < 4 {
			t.Fatalf("unexpected keyword: %s", kw)
		}
	}
}

func TestNormaliseSkills(t *testing.T) {
	raw := []string{"Go", "go ", "AWS", "Amazon Web Services"}
	skills := normaliseSkills(raw)
	if len(skills) != 3 {
		t.Fatalf("expected deduplicated skills, got %v", skills)
	}
}
