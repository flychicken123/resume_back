package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

const maxToolCallRounds = 3

// CallGeminiWithTools sends a prompt to Gemini with tool definitions.
// If the model requests a tool call, it executes the handler and feeds the result back.
// Streams text tokens via onChunk. Returns the final text response.
func CallGeminiWithTools(ctx context.Context, systemPrompt, userPrompt string, tools []ChatTool, userID int, onChunk func(string)) (string, error) {
	llm, err := getLangChainModel()
	if err != nil {
		return "", err
	}

	llmTools := ConvertToLLMTools(tools)

	messages := []llms.MessageContent{
		{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextContent{Text: systemPrompt}}},
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: userPrompt}}},
	}

	for round := 0; round < maxToolCallRounds; round++ {
		var buf strings.Builder
		opts := []llms.CallOption{
			llms.WithTools(llmTools),
		}
		_ = buf // used in future streaming rounds

		resp, err := llm.GenerateContent(ctx, messages, opts...)
		if err != nil {
			return buf.String(), fmt.Errorf("gemini tool call failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no response from model")
		}

		choice := resp.Choices[0]

		// If no tool calls, this is the final text response
		if len(choice.ToolCalls) == 0 {
			finalText := choice.Content
			if onChunk != nil && finalText != "" {
				onChunk(finalText)
			}
			return finalText, nil
		}

		// Process tool calls
		// Add assistant message with tool calls to conversation
		assistantParts := []llms.ContentPart{}
		for _, tc := range choice.ToolCalls {
			assistantParts = append(assistantParts, tc)
		}
		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: assistantParts,
		})

		// Execute each tool and add results
		toolResultParts := []llms.ContentPart{}
		for _, tc := range choice.ToolCalls {
			if tc.FunctionCall == nil {
				continue
			}

			// Notify client that a tool is being called
			if onChunk != nil {
				onChunk(fmt.Sprintf("[Searching: %s...]\n", tc.FunctionCall.Name))
			}

			// Parse arguments
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				args = map[string]any{}
			}

			// Find and execute handler
			handler := FindToolHandler(tc.FunctionCall.Name, tools)
			var resultStr string
			if handler != nil {
				result, err := handler(ctx, userID, args)
				if err != nil {
					resultStr = fmt.Sprintf(`{"error": "%s"}`, err.Error())
				} else {
					resultBytes, _ := json.Marshal(result)
					resultStr = string(resultBytes)
				}
			} else {
				resultStr = `{"error": "unknown tool"}`
			}

			toolResultParts = append(toolResultParts, llms.ToolCallResponse{
				ToolCallID: tc.ID,
				Name:       tc.FunctionCall.Name,
				Content:    resultStr,
			})
		}

		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeTool,
			Parts: toolResultParts,
		})
	}

	return "", fmt.Errorf("exceeded max tool call rounds")
}

// CallGeminiWithToolsBlocking is the non-streaming version.
func CallGeminiWithToolsBlocking(ctx context.Context, systemPrompt, userPrompt string, tools []ChatTool, userID int) (string, error) {
	return CallGeminiWithTools(ctx, systemPrompt, userPrompt, tools, userID, nil)
}
