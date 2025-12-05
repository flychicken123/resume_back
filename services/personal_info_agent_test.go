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
  "summary": " Senior backend engineer "
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
	assert.Equal(t, " Senior backend engineer ", res.Summary)
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
