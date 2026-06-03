package services

import (
	"testing"

	"resumeai/models"
)

func TestFinalizeJobClassificationFallsBackFromUnusableAIResult(t *testing.T) {
	tests := []struct {
		name          string
		title         string
		wantField     CareerField
		wantSeniority string
	}{
		{
			name:          "technical recruiter",
			title:         "Technical Recruiter",
			wantField:     CareerFieldHRRecruiting,
			wantSeniority: "mid",
		},
		{
			name:          "senior sales manager",
			title:         "Sr. Sales Manager (Global Starlink Enterprise Sales)",
			wantField:     CareerFieldSales,
			wantSeniority: "senior",
		},
		{
			name:          "ai research scientist",
			title:         "AI Research Scientist- GenAI",
			wantField:     CareerFieldDataScience,
			wantSeniority: "mid",
		},
		{
			name:          "site reliability engineer",
			title:         "Software Engineer, Site Reliability Engineering",
			wantField:     CareerFieldSoftwareEngineering,
			wantSeniority: "mid",
		},
		{
			name:          "plant controller",
			title:         "Director - Plant Controlling",
			wantField:     CareerFieldFinance,
			wantSeniority: "lead",
		},
		{
			name:          "supplier quality",
			title:         "Supplier Quality Specialist (Starshield)",
			wantField:     CareerFieldOperations,
			wantSeniority: "mid",
		},
		{
			name:          "uncategorized role",
			title:         "Youth Activities Specialist",
			wantField:     CareerFieldOther,
			wantSeniority: "mid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &models.JobPosting{Title: tt.title}

			field, _, seniority := finalizeJobClassification(job, CareerFieldUnknown, nil, "")

			if field != tt.wantField {
				t.Fatalf("expected field %s, got %s", tt.wantField, field)
			}
			if seniority != tt.wantSeniority {
				t.Fatalf("expected seniority %q, got %q", tt.wantSeniority, seniority)
			}
		})
	}
}

func TestFinalizeJobClassificationOverridesOtherWithSpecificFallback(t *testing.T) {
	job := &models.JobPosting{Title: "Sr. Software Engineer (Starlink)"}

	field, _, seniority := finalizeJobClassification(job, CareerFieldOther, nil, "mid")

	if field != CareerFieldSoftwareEngineering {
		t.Fatalf("expected specific fallback field, got %s", field)
	}
	if seniority != "senior" {
		t.Fatalf("expected title-derived seniority, got %q", seniority)
	}
}

func TestFinalizeJobClassificationPreservesExistingClassification(t *testing.T) {
	job := &models.JobPosting{
		Title:       "Software Engineer",
		CareerField: string(CareerFieldProductManagement),
		Seniority:   "staff",
	}

	field, _, seniority := finalizeJobClassification(job, CareerFieldUnknown, nil, "")

	if field != CareerFieldProductManagement {
		t.Fatalf("expected existing field to be preserved, got %s", field)
	}
	if seniority != "staff" {
		t.Fatalf("expected existing seniority to be preserved, got %q", seniority)
	}
}
