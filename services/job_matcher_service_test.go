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

	score := computeMatchScore(job, "backend engineer", skills, keywords, resumeText, "remote", 5.0)
	if score <= 0 {
		t.Fatalf("expected positive score, got %.2f", score)
	}
}

func TestComputeMatchScoreRequiresSignal(t *testing.T) {
	job := &models.JobPosting{Title: "Office Manager", Description: "assist with office tasks"}
	score := computeMatchScore(job, "backend", nil, nil, "", "", 0)
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

func TestComputeMatchScorePositionTokens(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Staff Product Manager",
		Description: "Lead product strategy with cross-functional teams",
		Location:    "San Francisco, CA",
	}

	keywords := []string{"strategy"}
	score := computeMatchScore(job, "Product Manager", nil, keywords, "", "San Francisco, CA", 5.0)
	if score <= 0 {
		t.Fatalf("expected positive score from resume keywords, got %.2f", score)
	}
}

func TestComputeMatchScoreRejectsTitleOnlyMatch(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Software Engineer",
		Description: "Join our team supporting internal tooling.",
	}

	score := computeMatchScore(job, "Software Engineer", nil, nil, "", "", 0)
	if score != 0 {
		t.Fatalf("expected zero score when only title overlaps, got %.2f", score)
	}
}

func TestInternJobFilteredForSeniorCandidate(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Software Engineer Intern",
		Description: "Join us for a summer internship building web apps",
		Location:    "Remote - US",
	}

	score := computeMatchScore(job, "Senior Software Engineer", nil, nil, "seasoned senior software engineer building systems", "", 8.0)
	if score != 0 {
		t.Fatalf("expected zero score for intern role when candidate is senior, got %.2f", score)
	}
}

func TestInternJobFilteredForNonSeniorCandidate(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Software Engineer Intern",
		Description: "Join us for a summer internship building web apps",
		Location:    "Remote - US",
	}

	score := computeMatchScore(job, "software engineer", nil, nil, "software engineering student", "", 1.0)
	if score != 0 {
		t.Fatalf("expected zero score for intern job when candidate is not senior, got %.2f", score)
	}
}
func TestDetermineJobSeniority(t *testing.T) {
	if level := determineJobSeniority("Lead Data Scientist", ""); level != "lead" {
		t.Fatalf("expected lead seniority, got %s", level)
	}
	if level := determineJobSeniority("Junior Developer", ""); level != "entry" {
		t.Fatalf("expected entry seniority, got %s", level)
	}
	if level := determineJobSeniority("Software Engineer Intern", ""); level != "intern" {
		t.Fatalf("expected intern seniority, got %s", level)
	}
	if level := determineJobSeniority("Staff Software Engineer", ""); level != "staff" {
		t.Fatalf("expected staff seniority, got %s", level)
	}
}

func TestDetermineCandidateSeniority(t *testing.T) {
	level := determineCandidateSeniority("Senior Software Engineer", "Seasoned senior engineer leading teams")
	if level != "senior" && level != "lead" {
		t.Fatalf("expected senior-or-above level, got %s", level)
	}
	entry := determineCandidateSeniority("Junior Developer", "recent graduate software engineer")
	if entry != "entry" {
		t.Fatalf("expected entry level, got %s", entry)
	}
}
