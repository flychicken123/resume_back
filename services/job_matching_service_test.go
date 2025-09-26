package services

import (
	"strings"
	"testing"
	"time"
)

func TestBuildResumeProfile(t *testing.T) {
	resume := ResumeData{
		Summary: "Experienced backend engineer with Go and cloud expertise.",
		Skills:  "Go, REST APIs, AWS, Kubernetes",
		Experiences: []ResumeExperience{
			{
				JobTitle:    "Backend Engineer",
				City:        "Austin",
				State:       "TX",
				Remote:      true,
				Description: "Built scalable Go microservices and REST APIs while collaborating with DevOps to maintain Kubernetes clusters.",
			},
		},
	}
	jobDescription := "Position: Senior Backend Engineer\nWe are looking for someone with Go and distributed systems experience."

	profile := buildResumeProfile(resume, jobDescription)

	if profile.TargetTitle != "Senior Backend Engineer" {
		t.Fatalf("expected target title to be derived from job description, got %q", profile.TargetTitle)
	}
	if !profile.RemotePreference {
		t.Fatalf("expected remote preference to be true when resume or job description signals remote work")
	}
	if len(profile.SkillPhrases) != 4 {
		t.Fatalf("expected four skill phrases, got %d", len(profile.SkillPhrases))
	}
	if profile.CountryHint != "us" {
		t.Fatalf("expected country hint to be us, got %s", profile.CountryHint)
	}
	if profile.PrimaryLocation == "" {
		t.Fatalf("expected primary location to be captured from experience")
	}
	if len(profile.QueryTerms) == 0 {
		t.Fatalf("expected query terms to be generated")
	}
}

func TestScorePosting(t *testing.T) {
	profile := resumeProfile{
		TargetTitle:      "Backend Engineer",
		SkillPhrases:     []string{"Go", "Kubernetes"},
		SkillTokens:      []string{"go", "kubernetes"},
		KeywordTokens:    []string{"distributed", "microservices"},
		PrimaryLocation:  "Austin TX",
		CountryHint:      "us",
		RemotePreference: true,
	}

	posting := jobPosting{
		Source:         "remotive",
		SourceKey:      "remotive:1",
		Title:          "Senior Backend Engineer",
		Company:        "Example Corp",
		Location:       "Remote - Austin, TX",
		Remote:         true,
		Description:    "Work on distributed microservices written in Go and deployed on Kubernetes.",
		PublishedAt:    time.Now(),
		NormalizedText: strings.ToLower("Senior Backend Engineer distributed microservices Go Kubernetes"),
	}

	score, matchedSkills, matchedKeywords := scorePosting(profile, posting)
	if score < 8 {
		t.Fatalf("expected strong score, got %f", score)
	}
	if len(matchedSkills) == 0 {
		t.Fatalf("expected skill matches to be detected")
	}
	if len(matchedKeywords) == 0 {
		t.Fatalf("expected keyword matches to be detected")
	}
}

func TestRankJobMatchesFallback(t *testing.T) {
	profile := resumeProfile{}
	postings := []jobPosting{
		{
			Source:      "remotive",
			SourceKey:   "remotive:1",
			Title:       "Generalist",
			Description: "",
			PublishedAt: time.Now(),
		},
	}

	matches := rankJobMatches(profile, postings, 3)
	if len(matches) == 0 {
		t.Fatalf("expected fallback matches when scoring produces zero results")
	}
	if matches[0].Score != 0 {
		t.Fatalf("expected fallback match to have zero score, got %f", matches[0].Score)
	}
}
func TestFilterPostingsByCountryUS(t *testing.T) {
	postings := []jobPosting{
		{Source: "remotive", Location: "Toronto, Canada"},
		{Source: "remotive", Location: "Austin, TX"},
		{Source: "remotive", Location: "Remote - United States"},
		{Source: "remotive", Location: "Remote", Remote: true},
	}

	filtered := filterPostingsByCountry("us", postings)
	if len(filtered) != 2 {
		t.Fatalf("expected US filter to keep 3 jobs, got %d", len(filtered))
	}

	filteredFallback := filterPostingsByCountry("us", []jobPosting{{Location: "Berlin, Germany"}})
	if len(filteredFallback) != 0 {
		t.Fatalf("expected no matches for US when none available, got %d", len(filteredFallback))
	}
}

func TestDetectCountryHintUK(t *testing.T) {
	locations := []string{"London, United Kingdom", "Remote, UK"}
	code := detectCountryHint(locations, "Looking for engineers in England")
	if code != "uk" {
		t.Fatalf("expected country hint uk, got %s", code)
	}
}

func TestFilterPostingsByCountryUK(t *testing.T) {
	postings := []jobPosting{
		{Source: "greenhouse", Location: "London, United Kingdom"},
		{Source: "lever", Location: "Berlin, Germany"},
		{Source: "remotive", Location: "Remote (UK/EU)"},
	}
	filtered := filterPostingsByCountry("uk", postings)
	if len(filtered) != 2 {
		t.Fatalf("expected UK filter to keep 2 jobs, got %d", len(filtered))
	}
}

func TestFilterRemoteWorldwide(t *testing.T) {
	postings := []jobPosting{
		{Source: "remotive", Location: "Remote", Remote: true},
		{Source: "lever", Location: "Remote - Global", Remote: true},
	}
	filtered := filterPostingsByCountry("us", postings)
	if len(filtered) != 2 {
		t.Fatalf("expected worldwide remotes to remain, got %d", len(filtered))
	}
}

func TestFilterRemoteRegionalExcluded(t *testing.T) {
	postings := []jobPosting{
		{Source: "remotive", Location: "Remote (Europe)", Remote: true},
		{Source: "remotive", Location: "Remote - Germany", Remote: true},
	}
	filtered := filterPostingsByCountry("us", postings)
	if len(filtered) != 0 {
		t.Fatalf("expected regional remotes to be excluded, got %d", len(filtered))
	}
}

func TestFilterPostingLocationOverridesDescription(t *testing.T) {
	postings := []jobPosting{
		{Source: "lever", Location: "Heidelberg, Baden-Württemberg, Germany", Description: "Collaborate with teams in the United States"},
	}
	filtered := filterPostingsByCountry("us", postings)
	if len(filtered) != 0 {
		t.Fatalf("expected location-mismatched posting to be filtered out, got %d", len(filtered))
	}
}
func TestDetectCountryFromStringCityOnly(t *testing.T) {
	if code := detectCountryFromString("Stuttgart"); code != "de" {
		t.Fatalf("expected Stuttgart to resolve to de, got %s", code)
	}
	if code := detectCountryFromString("New York"); code != "us" {
		t.Fatalf("expected New York to resolve to us, got %s", code)
	}
}

func TestContainsCountryIndicatorCityAlias(t *testing.T) {
	if !containsCountryIndicator("Based in Stuttgart", "de") {
		t.Fatalf("expected Stuttgart to match Germany hint")
	}
	if containsCountryIndicator("Stuttgart", "us") {
		t.Fatalf("expected Stuttgart to be excluded for US hint")
	}
}

func TestFilterPostingsByCountryCityMapping(t *testing.T) {
	postings := []jobPosting{
		{Source: "arbeitnow", Location: "Stuttgart", Remote: true, PublishedAt: time.Now()},
		{Source: "arbeitnow", Location: "Berlin", Remote: true, PublishedAt: time.Now().Add(-time.Hour)},
	}

	if filtered := filterPostingsByCountry("us", postings); len(filtered) != 0 {
		t.Fatalf("expected German remote cities to be filtered for US candidates, got %d", len(filtered))
	}

	filteredDE := filterPostingsByCountry("de", postings)
	if len(filteredDE) != 2 {
		t.Fatalf("expected German cities to remain for DE candidates, got %d", len(filteredDE))
	}
}
