package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// CopilotAgent wraps a LangChain Go LLM for orchestrating multi-step resume improvements.
type CopilotAgent struct {
	llm llms.Model
}

// NewCopilotAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewCopilotAgent() (*CopilotAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client: %w", err)
	}

	return &CopilotAgent{llm: llm}, nil
}

// RunPrompt executes a prompt through the LangChain model.
func (a *CopilotAgent) RunPrompt(ctx context.Context, prompt string) (string, error) {
	if a == nil || a.llm == nil {
		return "", fmt.Errorf("copilot agent is not initialized")
	}
	return llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
}
