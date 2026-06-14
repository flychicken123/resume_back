package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/fake"
)

func TestImpactKeywordsAgentParsesValidJSON(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{"experiences":{"exp-0":{"description":["80%","Redis","production systems"]}}}`,
	})
	agent := &ImpactKeywordsAgent{llm: llm}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID:          "exp-0",
			Description: "Reduced API latency by 80% using Redis in production systems.",
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"80%", "Redis", "production systems"}, result.Experiences["exp-0"].Description)
}

func TestImpactKeywordsAgentFallsBackWhenJSONIsTruncated(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{"experiences":{"exp-0":{"description":["scalable loan management system","Java Spring backend","1B+`,
	})
	agent := &ImpactKeywordsAgent{llm: llm}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID: "exp-0",
			Description: "Built a scalable loan management system with a Java Spring backend and JavaScript frontend, " +
				"streamlining workflows, improving operational efficiency, automating batch processing with stored procedures, " +
				"and processing 1B+ records.",
		}},
	})

	require.NoError(t, err)
	keywords := result.Experiences["exp-0"].Description
	assert.Contains(t, keywords, "1B+")
	assert.Contains(t, keywords, "scalable loan management system")
	assert.Contains(t, keywords, "Java Spring backend")
	assert.Contains(t, keywords, "JavaScript frontend")
	assert.LessOrEqual(t, len(keywords), impactKeywordsMaxPerField)
}

func TestImpactKeywordsAgentPassesStrictJSONOptions(t *testing.T) {
	t.Parallel()

	llm := &capturingImpactKeywordsLLM{
		response: `{"experiences":{"exp-0":{"description":[]}}}`,
	}
	agent := &ImpactKeywordsAgent{llm: llm}

	_, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{ID: "exp-0", Description: "No measurable impact."}},
	})

	require.NoError(t, err)
	assert.Equal(t, impactKeywordsMaxOutputTokens, llm.options.MaxTokens)
	assert.True(t, llm.options.JSONMode)
}

func TestImpactKeywordsAgentFallsBackWhenLLMCallFails(t *testing.T) {
	t.Parallel()

	agent := &ImpactKeywordsAgent{llm: &errorImpactKeywordsLLM{}}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID:          "exp-0",
			Description: "Reduced costs by 40% using AWS automation.",
		}},
	})

	require.NoError(t, err)
	keywords := result.Experiences["exp-0"].Description
	assert.Contains(t, keywords, "40%")
	assert.Contains(t, keywords, "AWS")
}

type capturingImpactKeywordsLLM struct {
	response string
	options  llms.CallOptions
}

func (c *capturingImpactKeywordsLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, opt := range options {
		opt(&c.options)
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: c.response}},
	}, nil
}

func (c *capturingImpactKeywordsLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := c.GenerateContent(ctx, []llms.MessageContent{{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: prompt}}}}, options...)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("empty response")
	}
	return resp.Choices[0].Content, nil
}

type errorImpactKeywordsLLM struct{}

func (e *errorImpactKeywordsLLM) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return nil, errors.New("llm unavailable")
}

func (e *errorImpactKeywordsLLM) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", errors.New("llm unavailable")
}
