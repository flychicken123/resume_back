package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

const maxToolCallRounds = 3

// ToolCallMetadata holds side effects from tool execution (e.g., resume field updates).
type ToolCallMetadata struct {
	ResumeUpdates []map[string]any
	ToolsCalled   []string
	ToolArgs      []string // raw JSON args for each tool call
	ToolResults   []string // raw JSON results for each tool call
	ToolErrors    []string // errors returned by tool handlers (as data, not Go errors)
}

// CallGeminiWithTools sends a prompt to Gemini with tool definitions.
// If the model requests a tool call, it executes the handler and feeds the result back.
// Streams text tokens via onChunk. Returns the final text response and any metadata.
func CallGeminiWithTools(ctx context.Context, systemPrompt, userPrompt string, tools []ChatTool, userID int, onChunk func(string)) (string, *ToolCallMetadata, error) {
	llm, err := getLangChainModel()
	if err != nil {
		return "", nil, err
	}

	llmTools := ConvertToLLMTools(tools)
	meta := &ToolCallMetadata{}

	messages := []llms.MessageContent{
		{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextContent{Text: systemPrompt}}},
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextContent{Text: userPrompt}}},
	}

	for round := 0; round < maxToolCallRounds; round++ {
		var streamedText strings.Builder
		opts := []llms.CallOption{
			llms.WithTools(llmTools),
		}

		// Add streaming callback — tokens flow to client as they arrive
		if onChunk != nil {
			opts = append(opts, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
				s := string(chunk)
				streamedText.WriteString(s)
				onChunk(s)
				return nil
			}))
		}

		resp, err := llm.GenerateContent(ctx, messages, opts...)
		if err != nil {
			return streamedText.String(), meta, fmt.Errorf("gemini tool call failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return streamedText.String(), meta, nil
		}

		choice := resp.Choices[0]

		// If no tool calls, this is the final text response (already streamed via callback)
		if len(choice.ToolCalls) == 0 {
			log.Printf("[TOOL-DISPATCH] round=%d: no tool calls, returning text response (len=%d)", round, len(choice.Content))
			if streamedText.Len() > 0 {
				return streamedText.String(), meta, nil
			}
			return choice.Content, meta, nil
		}

		// Tool calls detected — any streamed text was tool call metadata, not user text
		// Reset for next round (tool results will be fed back, then LLM generates final text)
		streamedText.Reset()

		// Process tool calls
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
			log.Printf("[TOOL-CALL] tool=%q args=%s", tc.FunctionCall.Name, tc.FunctionCall.Arguments)
			meta.ToolsCalled = append(meta.ToolsCalled, tc.FunctionCall.Name)
			meta.ToolArgs = append(meta.ToolArgs, tc.FunctionCall.Arguments)
			handler := FindToolHandler(tc.FunctionCall.Name, tools)
			var resultStr string
			if handler != nil {
				result, err := handler(ctx, userID, args)
				if err != nil {
					resultStr = fmt.Sprintf(`{"error": "%s"}`, err.Error())
					meta.ToolErrors = append(meta.ToolErrors, err.Error())
				} else {
					resultBytes, _ := json.Marshal(result)
					resultStr = string(resultBytes)

					// Collect resume update metadata and track data-level errors
					if resultMap, ok := result.(map[string]any); ok {
						if _, isResumeUpdate := resultMap["resume_update"]; isResumeUpdate {
							meta.ResumeUpdates = append(meta.ResumeUpdates, resultMap)
						}
						if errMsg, hasErr := resultMap["error"]; hasErr {
							meta.ToolErrors = append(meta.ToolErrors, fmt.Sprintf("%v", errMsg))
						}
					}
				}
			} else {
				resultStr = `{"error": "unknown tool"}`
				meta.ToolErrors = append(meta.ToolErrors, "unknown tool: "+tc.FunctionCall.Name)
			}

			log.Printf("[TOOL-RESULT] tool=%q result=%s", tc.FunctionCall.Name, resultStr)
			meta.ToolResults = append(meta.ToolResults, resultStr)
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

	return "", meta, fmt.Errorf("exceeded max tool call rounds")
}

// CallGeminiWithToolsBlocking is the non-streaming version.
func CallGeminiWithToolsBlocking(ctx context.Context, systemPrompt, userPrompt string, tools []ChatTool, userID int) (string, *ToolCallMetadata, error) {
	return CallGeminiWithTools(ctx, systemPrompt, userPrompt, tools, userID, nil)
}
