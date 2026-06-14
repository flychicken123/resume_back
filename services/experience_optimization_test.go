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
