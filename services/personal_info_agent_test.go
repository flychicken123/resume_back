package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms/fake"
)

func TestPersonalInfoAgent_HappyPath(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{
  "name": "  jane doe ",
  "email": " jane@example.com ",
  "phone": " +1 (425) 389-1623 ",
  "summary": "Senior backend engineer"
}`,
	})
	agent := &PersonalInfoAgent{llm: llm}

	res, err := agent.ExtractPersonalInfo(context.Background(), "My name is Jane Doe, email jane@example.com, phone +1 (425) 389-1623")
	require.NoError(t, err)

	// Trimming & normalization are applied
	assert.Equal(t, "jane doe", res.Name)
	assert.Equal(t, "jane@example.com", res.Email)
	assert.Equal(t, "+1 (425) 389-1623", res.Phone)
	// Summary is not trimmed inside the agent today; this test just verifies
	// that the model output is propagated.
	assert.Equal(t, "Senior backend engineer", res.Summary)
}

func TestPersonalInfoAgent_ErrorWhenLLMUnavailable(t *testing.T) {
	t.Parallel()

	var llm fake.LLM // no responses configured
	agent := &PersonalInfoAgent{llm: &llm}

	_, err := agent.ExtractPersonalInfo(context.Background(), "anything")
	require.Error(t, err)
}

func TestPersonalInfoAgent_InvalidJSONFromLLM(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`not-json`,
	})
	agent := &PersonalInfoAgent{llm: llm}

	_, err := agent.ExtractPersonalInfo(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse personal info JSON")
}

func TestPersonalInfoAgent_Uninitialized(t *testing.T) {
	t.Parallel()

	agent := &PersonalInfoAgent{} // llm is nil
	_, err := agent.ExtractPersonalInfo(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "personal info agent is not initialized")
}

func TestDetectIntendedField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		// Email updates - should detect "email"
		{"change my email address to test@example.com", "email"},
		{"change my email to test@example.com", "email"},
		{"update my email to test@example.com", "email"},
		{"set my email to test@example.com", "email"},

		// Phone updates - should detect "phone"
		{"change my phone to 555-1234", "phone"},
		{"update my phone number to 555-1234", "phone"},

		// Name updates - should detect "name"
		{"change my name to John Doe", "name"},
		{"my name is John Doe", ""},  // No "change my", so allow all

		// Multiple fields - should return "" to allow all
		{"My name is Jane, email jane@test.com, phone 555-1234", ""},
		{"name is John, email john@test.com", ""},

		// No specific field - should return ""
		{"I am a software engineer with 5 years experience", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := detectIntendedField(tt.input)
			assert.Equal(t, tt.expected, result, "For input: %s", tt.input)
		})
	}
}

func TestPersonalInfoAgent_EmailOnlyUpdate(t *testing.T) {
	t.Parallel()

	// LLM incorrectly returns both name and email
	llm := fake.NewFakeLLM([]string{
		`{
  "name": "address to test@example",
  "email": "test@example.com",
  "phone": null,
  "summary": null
}`,
	})
	agent := &PersonalInfoAgent{llm: llm}

	// Existing data
	existing := PersonalInfoResult{
		Name:  "John Doe",
		Email: "old@example.com",
		Phone: "555-1234",
	}

	// User says "change my email address to test@example.com"
	// The code should detect this is an email-only update and preserve the existing name
	res, err := agent.ExtractPersonalInfoWithContext(
		context.Background(),
		"change my email address to test@example.com",
		existing,
	)
	require.NoError(t, err)

	// Name should be preserved from existing, NOT changed to LLM's incorrect value
	assert.Equal(t, "John Doe", res.Name, "Name should be preserved")
	assert.Equal(t, "test@example.com", res.Email, "Email should be updated")
	assert.Equal(t, "555-1234", res.Phone, "Phone should be preserved")
}
