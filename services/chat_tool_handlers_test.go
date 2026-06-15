package services

import (
	"context"
	"testing"
)

func TestHandleResumeEditorDispatchUpdatesExistingExperienceByCompany(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"experiences": []any{
				map[string]any{
					"jobTitle":    "",
					"company":     "Nanjing Inforich Technology",
					"description": "Built backend systems.",
				},
				map[string]any{
					"jobTitle": "Software Engineer",
					"company":  "Microsoft Corporation",
				},
			},
		},
	})

	result, err := handleResumeEditorDispatch(ctx, 0, map[string]any{
		"operation":        "update_experience",
		"company_name":     "Nanjing Inforich",
		"experience_field": "jobTitle",
		"value":            "Project Leader",
	})
	if err != nil {
		t.Fatalf("handleResumeEditorDispatch returned error: %v", err)
	}

	payload := result.(map[string]any)
	if payload["field"] != "experiences" {
		t.Fatalf("field = %v, want experiences", payload["field"])
	}
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	second := updated[1].(map[string]any)
	if first["jobTitle"] != "Project Leader" {
		t.Fatalf("first jobTitle = %v, want Project Leader", first["jobTitle"])
	}
	if second["jobTitle"] != "Software Engineer" {
		t.Fatalf("second jobTitle changed to %v", second["jobTitle"])
	}
}

func TestHandleUpdateResumeFieldRoutesExperienceFieldsToNestedUpdate(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"experiences": []any{
				map[string]any{
					"jobTitle": "",
					"company":  "Hihired.org",
				},
			},
		},
	})

	result, err := handleUpdateResumeField(ctx, 0, map[string]any{
		"field":        "jobTitle",
		"company_name": "Hihired",
		"value":        "Founding Engineer",
	})
	if err != nil {
		t.Fatalf("handleUpdateResumeField returned error: %v", err)
	}

	payload := result.(map[string]any)
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["jobTitle"] != "Founding Engineer" {
		t.Fatalf("jobTitle = %v, want Founding Engineer", first["jobTitle"])
	}
}

func TestHandleUpdateResumeFieldRejectsUnsupportedTopLevelField(t *testing.T) {
	result, err := handleUpdateResumeField(context.Background(), 0, map[string]any{
		"field": "linkedin",
		"value": "https://linkedin.com/in/example",
	})
	if err != nil {
		t.Fatalf("handleUpdateResumeField returned error: %v", err)
	}

	payload := result.(map[string]any)
	if _, ok := payload["resume_update"]; ok {
		t.Fatalf("resume_update present for unsupported field: %v", payload)
	}
	if payload["error"] == "" {
		t.Fatalf("error missing for unsupported field: %v", payload)
	}
}

func TestHandleUpdateExperienceFieldAcceptsMapSliceResumeData(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"experiences": []map[string]any{
				{
					"jobTitle": "",
					"company":  "Nanjing Inforich Technology",
				},
			},
		},
	})

	result, err := handleUpdateExperienceField(ctx, 0, map[string]any{
		"company_name":     "Inforich",
		"experience_field": "jobTitle",
		"value":            "Project Leader",
	})
	if err != nil {
		t.Fatalf("handleUpdateExperienceField returned error: %v", err)
	}

	payload := result.(map[string]any)
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["jobTitle"] != "Project Leader" {
		t.Fatalf("jobTitle = %v, want Project Leader", first["jobTitle"])
	}
}

func TestHandleUpdateEducationFieldReplacesMalformedTargetEntry(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"education": []any{"malformed"},
		},
	})

	result, err := handleUpdateEducationField(ctx, 0, map[string]any{
		"education_field": "degree",
		"value":           "Master's",
	})
	if err != nil {
		t.Fatalf("handleUpdateEducationField returned error: %v", err)
	}

	payload := result.(map[string]any)
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["degree"] != "Master's" {
		t.Fatalf("degree = %v, want Master's", first["degree"])
	}
}

func TestHandleResumeEditorDispatchUpdatesEducationBySchool(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"education": []any{
				map[string]any{
					"degree": "Bachelor's",
					"school": "University of Washington",
					"field":  "",
				},
			},
		},
	})

	result, err := handleResumeEditorDispatch(ctx, 0, map[string]any{
		"operation":       "update_education",
		"school_name":     "Washington",
		"education_field": "field",
		"value":           "Computer Science",
	})
	if err != nil {
		t.Fatalf("handleResumeEditorDispatch returned error: %v", err)
	}

	payload := result.(map[string]any)
	if payload["field"] != "education" {
		t.Fatalf("field = %v, want education", payload["field"])
	}
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["field"] != "Computer Science" {
		t.Fatalf("education field = %v, want Computer Science", first["field"])
	}
	if first["degree"] != "Bachelor's" {
		t.Fatalf("degree changed to %v", first["degree"])
	}
}

func TestHandleResumeEditorDispatchCreatesBlankEducationEntryWhenMissing(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"education": []any{},
		},
	})

	result, err := handleResumeEditorDispatch(ctx, 0, map[string]any{
		"operation":       "update_education",
		"education_field": "degree",
		"value":           "Master's",
	})
	if err != nil {
		t.Fatalf("handleResumeEditorDispatch returned error: %v", err)
	}

	payload := result.(map[string]any)
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["degree"] != "Master's" {
		t.Fatalf("degree = %v, want Master's", first["degree"])
	}
}

func TestHandleResumeEditorDispatchUpdatesProjectByName(t *testing.T) {
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{
			"projects": []any{
				map[string]any{
					"projectName":  "TaskManager",
					"description":  "",
					"technologies": "React",
				},
			},
		},
	})

	result, err := handleResumeEditorDispatch(ctx, 0, map[string]any{
		"operation":     "update_project",
		"project_name":  "TaskManager",
		"project_field": "description",
		"value":         "A workflow app for tracking team tasks.",
	})
	if err != nil {
		t.Fatalf("handleResumeEditorDispatch returned error: %v", err)
	}

	payload := result.(map[string]any)
	if payload["field"] != "projects" {
		t.Fatalf("field = %v, want projects", payload["field"])
	}
	updated := payload["value"].([]any)
	first := updated[0].(map[string]any)
	if first["description"] != "A workflow app for tracking team tasks." {
		t.Fatalf("description = %v, want updated description", first["description"])
	}
	if first["technologies"] != "React" {
		t.Fatalf("technologies changed to %v", first["technologies"])
	}
}
