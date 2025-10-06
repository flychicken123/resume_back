package utils

import "testing"

func TestDetectSeniorityFromString(t *testing.T) {
	if DetectSeniorityFromString("Principal Engineer") != SeniorityLead {
		t.Fatalf("expected lead level")
	}
	if DetectSeniorityFromString("Senior Developer") != SenioritySenior {
		t.Fatalf("expected senior level")
	}
	if DetectSeniorityFromString("Junior Analyst") != SeniorityEntry {
		t.Fatalf("expected entry level")
	}
	if DetectSeniorityFromString("Software Engineer Intern") != SeniorityIntern {
		t.Fatalf("expected intern level")
	}
}

func TestDetermineJobSeniority(t *testing.T) {
	if DetermineJobSeniority("Senior Product Manager", "") != SenioritySenior {
		t.Fatalf("expected senior level")
	}
	if DetermineJobSeniority("Backend Engineer", "We are looking for an apprentice or intern") != SeniorityIntern {
		t.Fatalf("expected intern level from description")
	}
}

func TestDetermineCandidateSeniority(t *testing.T) {
	level := DetermineCandidateSeniority("Lead Data Scientist", "Led senior data teams")
	if level != SeniorityLead {
		t.Fatalf("expected lead level, got %d", level)
	}
	entry := DetermineCandidateSeniority("Software Developer", "recent graduate developer")
	if entry != SeniorityEntry {
		t.Fatalf("expected entry level, got %d", entry)
	}
}
