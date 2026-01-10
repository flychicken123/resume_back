package services

import (
	"testing"
)

func TestInferImplicitSkills_Frontend(t *testing.T) {
	skills := []string{"react", "typescript", "css"}
	inferred := InferImplicitSkills(skills)

	found := false
	for _, s := range inferred {
		if s == "frontend" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'frontend' to be inferred from %v, got %v", skills, inferred)
	}
}

func TestInferImplicitSkills_Backend(t *testing.T) {
	skills := []string{"node", "express", "postgresql"}
	inferred := InferImplicitSkills(skills)

	hasBackend := false
	hasDatabase := false
	for _, s := range inferred {
		if s == "backend" {
			hasBackend = true
		}
		if s == "database" {
			hasDatabase = true
		}
	}
	if !hasBackend {
		t.Errorf("expected 'backend' to be inferred from %v, got %v", skills, inferred)
	}
	if !hasDatabase {
		t.Errorf("expected 'database' to be inferred from %v, got %v", skills, inferred)
	}
}

func TestInferImplicitSkills_Fullstack(t *testing.T) {
	skills := []string{"react", "node", "postgresql"}
	inferred := InferImplicitSkills(skills)

	hasFullstack := false
	for _, s := range inferred {
		if s == "fullstack" {
			hasFullstack = true
			break
		}
	}
	if !hasFullstack {
		t.Errorf("expected 'fullstack' to be inferred from %v, got %v", skills, inferred)
	}
}

func TestInferImplicitSkills_DevOps(t *testing.T) {
	skills := []string{"docker", "kubernetes", "terraform"}
	inferred := InferImplicitSkills(skills)

	found := false
	for _, s := range inferred {
		if s == "devops" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'devops' to be inferred from %v, got %v", skills, inferred)
	}
}

func TestInferImplicitSkills_NoInference(t *testing.T) {
	// If user already has the skill, don't infer it
	skills := []string{"frontend", "react"}
	inferred := InferImplicitSkills(skills)

	for _, s := range inferred {
		if s == "frontend" {
			t.Errorf("should not infer 'frontend' when user already has it")
		}
	}
}

func TestExpandSkillsWithInference(t *testing.T) {
	skills := []string{"React", "Node.js", "PostgreSQL"}
	expanded := ExpandSkillsWithInference(skills)

	expectedSkills := map[string]bool{
		"react":      true,
		"node":       true,
		"postgresql": true,
		"frontend":   true,
		"backend":    true,
		"database":   true,
		"fullstack":  true,
	}

	for expected := range expectedSkills {
		found := false
		for _, s := range expanded {
			if s == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in expanded skills, got %v", expected, expanded)
		}
	}
}
