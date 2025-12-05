package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms/fake"
)

func TestTemplatePreferenceAgent_HappyPath(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{"template_id":"modern-clean","font_size_id":"large","reason":"test choice"}`,
	})
	agent := &TemplatePreferenceAgent{llm: llm}

	res, err := agent.InferPreference(context.Background(), "I want a modern clean template with large font size")
	require.NoError(t, err)
	assert.Equal(t, "modern-clean", res.TemplateID)
	assert.Equal(t, "large", res.FontSizeID)
	assert.Equal(t, "test choice", res.Reason)
}

func TestTemplatePreferenceAgent_NormalizesAndDefaults(t *testing.T) {
	t.Parallel()

	// The model returns a shorthand template id and omits font size.
	llm := fake.NewFakeLLM([]string{
		`{"template_id":"modern","font_size_id":""}`,
	})
	agent := &TemplatePreferenceAgent{llm: llm}

	res, err := agent.InferPreference(context.Background(), "something vague")
	require.NoError(t, err)
	// "modern" should normalize to "modern-clean"
	assert.Equal(t, "modern-clean", res.TemplateID)
	// Missing font size should default to "medium"
	assert.Equal(t, "medium", res.FontSizeID)
}

func TestTemplatePreferenceAgent_ErrorWhenLLMUnavailable(t *testing.T) {
	t.Parallel()

	// Zero-value fake LLM returns an error because it has no responses.
	var llm fake.LLM
	agent := &TemplatePreferenceAgent{llm: &llm}

	_, err := agent.InferPreference(context.Background(), "anything")
	require.Error(t, err)
}

func TestTemplatePreferenceAgent_InvalidJSONFromLLM(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`not-json-at-all`,
	})
	agent := &TemplatePreferenceAgent{llm: llm}

	_, err := agent.InferPreference(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template preference JSON")
}

func TestTemplatePreferenceAgent_Uninitialized(t *testing.T) {
	t.Parallel()

	agent := &TemplatePreferenceAgent{} // llm is nil
	_, err := agent.InferPreference(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template preference agent is not initialized")
}

