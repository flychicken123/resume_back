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

func TestImpactKeywordsAgentUsesReviewerSelection(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{"experiences":{"exp-0":{"description":[{"text":"scalable loan management system","type":"scale","score":0.84},{"text":"Java Spring backend","type":"technology","score":0.58},{"text":"1B+ records","type":"metric","score":0.98}]}}}`,
		`{"experiences":{"exp-0":{"description":[{"text":"1B+ records","type":"metric","score":0.98},{"text":"scalable loan management system","type":"scale","score":0.84}]}}}`,
	})
	agent := &ImpactKeywordsAgent{llm: llm}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID: "exp-0",
			Description: "Built a scalable loan management system with a Java Spring backend and JavaScript frontend, " +
				"streamlining workflows and processing 1B+ records.",
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"1B+ records", "scalable loan management system"}, result.Experiences["exp-0"].Description)
}

func TestImpactKeywordsAgentRetriesTruncatedExtractorJSON(t *testing.T) {
	t.Parallel()

	llm := fake.NewFakeLLM([]string{
		`{"experiences":{"exp-0":{"description":[{"text":"1B+ records"}`,
		`{"experiences":{"exp-0":{"description":[{"text":"1B+ records","type":"metric","score":0.98}]}}}`,
		`{"experiences":{"exp-0":{"description":[{"text":"1B+ records","type":"metric","score":0.98}]}}}`,
	})
	agent := &ImpactKeywordsAgent{llm: llm}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID:          "exp-0",
			Description: "Processed 1B+ records across the loan platform.",
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"1B+ records"}, result.Experiences["exp-0"].Description)
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
	assert.Equal(t, 2, llm.calls)
	assert.Equal(t, impactKeywordsMaxOutputTokens, llm.options.MaxTokens)
	assert.True(t, llm.options.JSONMode)
}

func TestImpactKeywordsAgentReturnsEmptyWhenLLMCallFails(t *testing.T) {
	t.Parallel()

	agent := &ImpactKeywordsAgent{llm: &errorImpactKeywordsLLM{}}

	result, err := agent.ExtractImpactKeywords(context.Background(), ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID:          "exp-0",
			Description: "Reduced costs by 40% using AWS automation.",
		}},
	})

	require.NoError(t, err)
	assert.Empty(t, result.Experiences["exp-0"].Description)
}

func TestValidateImpactKeywordCandidatesRejectsUnsafePhrases(t *testing.T) {
	t.Parallel()

	input := ImpactKeywordsRequest{
		Experiences: []ExperienceImpactInput{{
			ID:          "exp-0",
			Description: "Reduced API latency by 80% using Redis caching.",
		}},
	}
	candidates := impactCandidateResult{
		Experiences: map[string]impactCandidateExperience{
			"exp-0": {
				Description: []impactKeywordCandidate{
					{Text: "Reduced API latency by 80%", Score: 0.95},
					{Text: "API latency", Score: 0.90},
					{Text: "80% using Redis caching and many unrelated extra words", Score: 0.80},
					{Text: "invented revenue growth", Score: 0.99},
					{Text: "Redis caching", Score: 0.70},
				},
			},
		},
	}

	result := validateImpactKeywordCandidates(input, candidates)

	assert.Equal(t, []string{"Reduced API latency by 80%", "Redis caching"}, result.Experiences["exp-0"].Description)
}

func TestImpactKeywordCandidateAcceptsLegacyStringJSON(t *testing.T) {
	t.Parallel()

	parsed, err := parseImpactCandidateResult(`{"experiences":{"exp-0":{"description":["80%","Redis caching"]}}}`)

	require.NoError(t, err)
	require.Len(t, parsed.Experiences["exp-0"].Description, 2)
	assert.Equal(t, "80%", parsed.Experiences["exp-0"].Description[0].Text)
	assert.Equal(t, "Redis caching", parsed.Experiences["exp-0"].Description[1].Text)
}

type capturingImpactKeywordsLLM struct {
	response string
	options  llms.CallOptions
	calls    int
}

func (c *capturingImpactKeywordsLLM) GenerateContent(_ context.Context, _ []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	c.calls++
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
