package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms/fake"
)

func TestJobIntentAgent_HappyPath_URLAndText(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{
  "url": " https://job-boards.greenhouse.io/affirm/jobs/7492221003 ",
  "job_description_from_text": " Senior backend role focusing on AI agents. "
}`,
	})
	agent := &JobIntentAgent{llm: llm}

	res, err := agent.ParseJobIntent(context.Background(), "Use this job and description")
	require.NoError(t, err)

	assert.Equal(t, "https://job-boards.greenhouse.io/affirm/jobs/7492221003", res.URL)
	assert.Equal(t, "Senior backend role focusing on AI agents.", res.JobDescriptionFromText)
}

func TestJobIntentAgent_HappyPath_URLOnly(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{
  "url": "https://example.com/job/123",
  "job_description_from_text": ""
}`,
	})
	agent := &JobIntentAgent{llm: llm}

	res, err := agent.ParseJobIntent(context.Background(), "Just use this URL")
	require.NoError(t, err)

	assert.Equal(t, "https://example.com/job/123", res.URL)
	assert.Equal(t, "", res.JobDescriptionFromText)
}

func TestJobIntentAgent_ErrorWhenLLMUnavailable(t *testing.T) {
	t.Parallel()

	var llm fake.LLM // no responses configured
	agent := &JobIntentAgent{llm: &llm}

	_, err := agent.ParseJobIntent(context.Background(), "anything")
	require.Error(t, err)
}

func TestJobIntentAgent_InvalidJSONFromLLM(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`not-json`,
	})
	agent := &JobIntentAgent{llm: llm}

	_, err := agent.ParseJobIntent(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse job intent JSON")
}

func TestJobIntentAgent_Uninitialized(t *testing.T) {
	t.Parallel()

	agent := &JobIntentAgent{} // llm is nil
	_, err := agent.ParseJobIntent(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job intent agent is not initialized")
}

