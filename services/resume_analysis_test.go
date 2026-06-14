package services

import (
	"strings"
	"testing"
)

func TestAnalyzeResumeDeterministicFactsDetectsMissingFieldsAndBulletSignals(t *testing.T) {
	resumeData := map[string]interface{}{
		"summary": "",
		"skills":  "",
		"experiences": []interface{}{
			map[string]interface{}{
				"jobTitle":    "",
				"company":     "Microsoft",
				"description": "Worked on backend services\nReduced API latency by 40%\nBuilt a very long internal platform bullet that includes too many words and keeps going beyond the practical scanning limit for a recruiter reviewing quickly while also mixing architecture, delivery, operations, stakeholder alignment, monitoring, rollout planning, and support details into one line",
			},
		},
		"education": []interface{}{
			map[string]interface{}{},
		},
	}

	facts := AnalyzeResumeDeterministicFacts(resumeData, "")

	if facts.SectionCoverage.HasSummary {
		t.Fatal("summary should be reported missing")
	}
	if facts.SectionCoverage.HasSkills {
		t.Fatal("skills should be reported missing")
	}
	if facts.SectionCoverage.HasEducation {
		t.Fatal("education should be reported missing")
	}
	if facts.Counts.BulletCount != 3 {
		t.Fatalf("BulletCount = %d, want 3", facts.Counts.BulletCount)
	}
	if facts.Counts.BulletsWithMetrics != 1 {
		t.Fatalf("BulletsWithMetrics = %d, want 1", facts.Counts.BulletsWithMetrics)
	}
	if facts.Counts.WeakVerbBulletCount != 1 {
		t.Fatalf("WeakVerbBulletCount = %d, want 1", facts.Counts.WeakVerbBulletCount)
	}
	if facts.Counts.LongBulletCount != 1 {
		t.Fatalf("LongBulletCount = %d, want 1", facts.Counts.LongBulletCount)
	}

	for _, code := range []string{"missing_summary", "missing_skills_section", "missing_education_section", "missing_experience_title", "missing_experience_dates"} {
		if !analysisFactsContainIssue(facts, code) {
			t.Fatalf("expected issue code %q in %#v", code, facts)
		}
	}
	if !analysisFactsContainSignal(facts, "quantified_achievement") {
		t.Fatalf("expected quantified achievement signal in %#v", facts.StrengthSignals)
	}
}

func TestBuildResumeAnalysisPromptIncludesStructuredFactsContract(t *testing.T) {
	prompt := BuildResumeAnalysisPrompt(map[string]interface{}{
		"name":   "Xuan Wu",
		"skills": "",
	}, "")

	for _, expected := range []string{
		"DETERMINISTIC FACTS JSON",
		"missing_skills_section",
		"Do not say you already analyzed this resume before",
		"Priority fixes",
		"Use only evidence from the resume and the deterministic facts",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func analysisFactsContainIssue(facts ResumeAnalysisFacts, code string) bool {
	for _, issue := range facts.MissingFields {
		if issue.Code == code {
			return true
		}
	}
	for _, issue := range facts.QualityIssues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func analysisFactsContainSignal(facts ResumeAnalysisFacts, code string) bool {
	for _, signal := range facts.StrengthSignals {
		if signal.Code == code {
			return true
		}
	}
	return false
}
