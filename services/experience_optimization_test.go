package services

import (
	"strings"
	"testing"
)

func TestBuildExperienceOptimizationPromptIncludesContext(t *testing.T) {
	prompt := BuildContextualExperienceOptimizationPrompt(ExperienceOptimizationInput{
		JobDescription: "Backend engineer role focused on Go services and reliability.",
		UserExperience: "Built API services and improved deployment process.",
		MatchedSkills:  []string{"Go", "API design"},
		MissingSkills:  []string{"Kubernetes"},
		Context: ExperienceOptimizationContext{
			JobTitle:         "Software Engineer",
			Company:          "Acme",
			City:             "Seattle",
			State:            "WA",
			CurrentlyWorking: true,
			ResumeSkills:     []string{"Go", "PostgreSQL"},
			TargetRole:       "Backend Engineer",
			TargetCompany:    "Stripe",
		},
	})

	for _, want := range []string{
		"Current role title: Software Engineer",
		"Current employer: Acme",
		"Location: Seattle, WA",
		"End date: Present",
		"Candidate's existing resume skills: Go, PostgreSQL",
		"Skills that match the target job: Go, API design",
		"Skills the target job wants but are not confirmed in the resume: Kubernetes",
		"Target role: Backend Engineer",
		"Target company: Stripe",
		"NEVER add missing skills as if the candidate has them",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildContextualExperienceGrammarPromptIncludesContext(t *testing.T) {
	prompt := BuildContextualExperienceGrammarPrompt(ExperienceOptimizationInput{
		UserExperience: "built APIs and fixed deploys",
		Context: ExperienceOptimizationContext{
			JobTitle:     "Software Engineer",
			Company:      "Acme",
			ResumeSkills: []string{"Go", "React"},
		},
	})

	for _, want := range []string{
		"Current role title: Software Engineer",
		"Current employer: Acme",
		"Candidate's existing resume skills: Go, React",
		"Do NOT invent new duties, technologies, metrics",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildExperienceBatchOptimizationPromptIncludesPositionsAndIntegrityRules(t *testing.T) {
	prompt := BuildExperienceBatchOptimizationPrompt([]ExperienceOptimizationBatchItem{
		{
			Position: 0,
			Index:    10,
			Input: ExperienceOptimizationInput{
				JobDescription: "Backend role focused on Go APIs.",
				UserExperience: "Built API services.",
				MatchedSkills:  []string{"Go"},
				MissingSkills:  []string{"Kubernetes"},
				Context: ExperienceOptimizationContext{
					JobTitle:     "Software Engineer",
					Company:      "Acme",
					ResumeSkills: []string{"Go", "PostgreSQL"},
				},
			},
		},
		{
			Position: 1,
			Index:    11,
			Input: ExperienceOptimizationInput{
				JobDescription: "Backend role focused on Go APIs.",
				UserExperience: "Improved deployments.",
			},
		},
	})

	for _, want := range []string{
		"EXPERIENCE POSITION 0 / ORIGINAL INDEX 10",
		"EXPERIENCE POSITION 1 / ORIGINAL INDEX 11",
		"Backend role focused on Go APIs.",
		"Built API services.",
		"Improved deployments.",
		"Candidate's existing resume skills: Go, PostgreSQL",
		"Skills the target job wants but are not confirmed in the resume: Kubernetes",
		"NEVER invent accomplishments",
		"Return one result for every supplied experience position",
		"under 900 characters",
		`"reviewReason": "checked"`,
		`"position": 0`,
		`"optimizedExperience"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildExperienceBatchOptimizationPromptTruncatesLargeInputs(t *testing.T) {
	longJob := strings.Repeat("job ", 700)
	longExperience := strings.Repeat("experience ", 300)

	prompt := BuildExperienceBatchOptimizationPrompt([]ExperienceOptimizationBatchItem{
		{
			Position: 0,
			Index:    10,
			Input: ExperienceOptimizationInput{
				JobDescription: longJob,
				UserExperience: longExperience,
			},
		},
	})

	if strings.Count(prompt, "[truncated]") != 2 {
		t.Fatalf("expected job and experience text to be truncated, prompt:\n%s", prompt)
	}
	if strings.Contains(prompt, strings.TrimSpace(longJob)) {
		t.Fatalf("prompt contains untruncated job description")
	}
	if strings.Contains(prompt, strings.TrimSpace(longExperience)) {
		t.Fatalf("prompt contains untruncated experience")
	}
}

func TestBuildExperienceSingleOptimizationPromptUsesPlainTextRules(t *testing.T) {
	prompt := BuildExperienceSingleOptimizationPrompt(ExperienceOptimizationBatchItem{
		Position: 0,
		Index:    10,
		Input: ExperienceOptimizationInput{
			JobDescription: "Backend role focused on Go APIs.",
			UserExperience: "Built API services.",
			MatchedSkills:  []string{"Go"},
			MissingSkills:  []string{"Kubernetes"},
			Context: ExperienceOptimizationContext{
				JobTitle:     "Software Engineer",
				Company:      "Acme",
				ResumeSkills: []string{"Go", "PostgreSQL"},
			},
		},
	})

	for _, want := range []string{
		"Backend role focused on Go APIs.",
		"Built API services.",
		"Candidate's existing resume skills: Go, PostgreSQL",
		"Skills the target job wants but are not confirmed in the resume: Kubernetes",
		"under 900 characters",
		"Return ONLY the improved experience text",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"optimizedExperience"`) {
		t.Fatalf("single prompt should not ask for JSON:\n%s", prompt)
	}
}

func TestCleanPlainExperienceResponseHandlesFencedAndQuotedText(t *testing.T) {
	got := cleanPlainExperienceResponse("```text\n\"Built Go APIs.\\nImproved reliability.\"\n```")
	want := "Built Go APIs.\nImproved reliability."
	if got != want {
		t.Fatalf("cleanPlainExperienceResponse() = %q, want %q", got, want)
	}
}

func TestParseExperienceBatchOptimizationResultsHandlesWrappedAndFencedJSON(t *testing.T) {
	raw := "```json\n{\"results\":[{\"position\":1,\"index\":11,\"optimizedExperience\":\"Improved deployments.\",\"reviewStatus\":\"fast_batch\",\"reviewReason\":\"ok\"}]}\n```"

	results, err := ParseExperienceBatchOptimizationResults(raw)
	if err != nil {
		t.Fatalf("ParseExperienceBatchOptimizationResults returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].position != 1 {
		t.Fatalf("position = %d, want 1", results[0].position)
	}
	if !results[0].hasIndex || results[0].index != 11 {
		t.Fatalf("index = %d hasIndex=%v, want 11 true", results[0].index, results[0].hasIndex)
	}
	if results[0].optimizedExperience != "Improved deployments." {
		t.Fatalf("optimizedExperience = %q", results[0].optimizedExperience)
	}
	if results[0].reviewStatus != "fast_batch" || results[0].reviewReason != "ok" {
		t.Fatalf("unexpected review fields: %#v", results[0])
	}
}

func TestParseExperienceBatchOptimizationResultsHandlesBareArray(t *testing.T) {
	raw := `[{"position":0,"optimized_experience":"Built Go APIs."}]`

	results, err := ParseExperienceBatchOptimizationResults(raw)
	if err != nil {
		t.Fatalf("ParseExperienceBatchOptimizationResults returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].position != 0 || results[0].optimizedExperienceSnake != "Built Go APIs." {
		t.Fatalf("unexpected result: %#v", results[0])
	}
}

func TestParseExperienceOptimizationReviewHandlesFencedSnakeCaseJSON(t *testing.T) {
	raw := "```json\n{\"approved\":false,\"revised_experience\":\"Built Go APIs without adding Kubernetes claims.\",\"reason\":\"removed unsupported skill\"}\n```"

	review, err := ParseExperienceOptimizationReview(raw)
	if err != nil {
		t.Fatalf("ParseExperienceOptimizationReview returned error: %v", err)
	}
	if review.Approved {
		t.Fatalf("expected review to be unapproved")
	}
	if review.RevisedExperience != "Built Go APIs without adding Kubernetes claims." {
		t.Fatalf("unexpected revised experience: %q", review.RevisedExperience)
	}
	if review.Reason != "removed unsupported skill" {
		t.Fatalf("unexpected reason: %q", review.Reason)
	}
}

func TestApplyExperienceOptimizationReviewRequiresApprovalOrRevision(t *testing.T) {
	_, status, err := applyExperienceOptimizationReview("Invented Kubernetes work.", ExperienceOptimizationReview{
		Approved: false,
		Reason:   "unsupported technology",
	})
	if err == nil {
		t.Fatalf("expected rejected review to return an error")
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected", status)
	}
}

func TestApplyExperienceOptimizationReviewUsesRevision(t *testing.T) {
	final, status, err := applyExperienceOptimizationReview("Invented Kubernetes work.", ExperienceOptimizationReview{
		Approved:          false,
		RevisedExperience: "Built Go APIs and improved deployment reliability.",
		Reason:            "revised unsupported claim",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "revised" {
		t.Fatalf("status = %q, want revised", status)
	}
	if final != "Built Go APIs and improved deployment reliability." {
		t.Fatalf("final = %q", final)
	}
}
