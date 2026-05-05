package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

type fakeToolModel struct {
	chunks    [][]string
	responses []*llms.ContentResponse
	calls     int
}

func (f *fakeToolModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	callIndex := f.calls
	f.calls++

	var opts llms.CallOptions
	for _, opt := range options {
		opt(&opts)
	}
	if callIndex < len(f.chunks) && opts.StreamingFunc != nil {
		for _, chunk := range f.chunks[callIndex] {
			if err := opts.StreamingFunc(ctx, []byte(chunk)); err != nil {
				return nil, err
			}
		}
	}
	if callIndex >= len(f.responses) {
		return &llms.ContentResponse{}, nil
	}
	return f.responses[callIndex], nil
}

func (f *fakeToolModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := f.GenerateContent(ctx, []llms.MessageContent{{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: prompt}}}}, options...)
	if err != nil || len(resp.Choices) == 0 {
		return "", err
	}
	return resp.Choices[0].Content, nil
}

func withFakeToolModel(t *testing.T, model llms.Model) {
	t.Helper()
	previous := getToolLLM
	getToolLLM = func() (llms.Model, error) {
		return model, nil
	}
	t.Cleanup(func() {
		getToolLLM = previous
	})
}

func TestCallGeminiWithToolsStreamingEmitsTokens(t *testing.T) {
	model := &fakeToolModel{
		chunks: [][]string{{"hel", "lo"}},
		responses: []*llms.ContentResponse{{
			Choices: []*llms.ContentChoice{{Content: "hello"}},
		}},
	}
	withFakeToolModel(t, model)

	var tokens []string
	reply, meta, err := CallGeminiWithToolsStreaming(context.Background(), "system", "user", nil, 0, ToolStreamCallbacks{
		OnToken: func(token string) error {
			tokens = append(tokens, token)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "hello", reply)
	require.Empty(t, meta.ToolsCalled)
	require.Equal(t, []string{"hel", "lo"}, tokens)
}

func TestCallGeminiWithToolsStreamingResetsBeforeToolResultRound(t *testing.T) {
	model := &fakeToolModel{
		chunks: [][]string{{"metadata"}, {"final"}},
		responses: []*llms.ContentResponse{
			{
				Choices: []*llms.ContentChoice{{
					ToolCalls: []llms.ToolCall{{
						ID:   "tool-1",
						Type: "function",
						FunctionCall: &llms.FunctionCall{
							Name:      "demo_tool",
							Arguments: `{"value":"ok"}`,
						},
					}},
				}},
			},
			{
				Choices: []*llms.ContentChoice{{Content: "final"}},
			},
		},
	}
	withFakeToolModel(t, model)

	var resets int
	var statuses []string
	tools := []ChatTool{{
		Name:        "demo_tool",
		Description: "demo",
		Handler: func(ctx context.Context, userID int, args map[string]any) (any, error) {
			require.Equal(t, "Harvey", getResumeString(ctx, "name"))
			return map[string]any{"ok": true}, nil
		},
	}}
	ctx := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{"name": "Harvey"},
	})

	reply, meta, err := CallGeminiWithToolsStreaming(ctx, "system", "user", tools, 123, ToolStreamCallbacks{
		OnReset: func() error {
			resets++
			return nil
		},
		OnStatus: func(status string) error {
			statuses = append(statuses, status)
			return nil
		},
		OnToken: func(token string) error {
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "final", reply)
	require.Equal(t, 1, resets)
	require.Len(t, statuses, 1)
	require.True(t, strings.Contains(statuses[0], "demo_tool"))
	require.Equal(t, []string{"demo_tool"}, meta.ToolsCalled)
}

func TestCallGeminiWithToolsStreamingStopsOnCallbackError(t *testing.T) {
	model := &fakeToolModel{
		chunks: [][]string{{"partial"}},
		responses: []*llms.ContentResponse{{
			Choices: []*llms.ContentChoice{{Content: "partial"}},
		}},
	}
	withFakeToolModel(t, model)

	callbackErr := errors.New("client disconnected")
	reply, _, err := CallGeminiWithToolsStreaming(context.Background(), "system", "user", nil, 0, ToolStreamCallbacks{
		OnToken: func(token string) error {
			return callbackErr
		},
	})

	require.ErrorIs(t, err, callbackErr)
	require.Equal(t, "partial", reply)
}

func TestChatToolRequestContextIsRequestScoped(t *testing.T) {
	ctxA := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{"name": "A"},
	})
	ctxB := WithChatToolRequestContext(context.Background(), ChatToolRequestContext{
		ResumeData: map[string]any{"name": "B"},
	})

	require.Equal(t, "A", getResumeString(ctxA, "name"))
	require.Equal(t, "B", getResumeString(ctxB, "name"))
}
