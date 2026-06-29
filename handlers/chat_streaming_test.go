package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"resumeai/services"
)

func TestChatAssistantStreamSendsTokenBeforeDone(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousStreaming := callGeminiToolsStreaming
	previousClassify := classifyWriteIntent
	callGeminiToolsStreaming = func(ctx context.Context, systemPrompt, userPrompt string, tools []services.ChatTool, userID int, callbacks services.ToolStreamCallbacks, opts ...services.ToolCallOption) (string, *services.ToolCallMetadata, error) {
		require.NoError(t, callbacks.OnToken("Hi"))
		return "Hi", &services.ToolCallMetadata{}, nil
	}
	classifyWriteIntent = func(ctx context.Context, userMessage string) bool {
		return false
	}
	t.Cleanup(func() {
		callGeminiToolsStreaming = previousStreaming
		classifyWriteIntent = previousClassify
	})

	router := gin.New()
	router.POST("/api/assistant/chat", ChatAssistant)
	body, err := json.Marshal(chatRequest{Message: "hello", SessionID: "test-session"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat?stream=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	raw := w.Body.String()
	readyIndex := strings.Index(raw, `"ready":true`)
	tokenIndex := strings.Index(raw, `"token":"Hi"`)
	doneIndex := strings.Index(raw, `"done":true`)
	require.NotEqual(t, -1, readyIndex, raw)
	require.NotEqual(t, -1, tokenIndex, raw)
	require.NotEqual(t, -1, doneIndex, raw)
	require.Less(t, readyIndex, tokenIndex)
	require.Less(t, tokenIndex, doneIndex)
}

func TestChatAssistantStreamForcedRetrySendsReset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousStreaming := callGeminiToolsStreaming
	previousClassify := classifyWriteIntent
	calls := 0
	callGeminiToolsStreaming = func(ctx context.Context, systemPrompt, userPrompt string, tools []services.ChatTool, userID int, callbacks services.ToolStreamCallbacks, opts ...services.ToolCallOption) (string, *services.ToolCallMetadata, error) {
		calls++
		if calls == 1 {
			require.NoError(t, callbacks.OnToken("not written"))
			return "not written", &services.ToolCallMetadata{}, nil
		}
		require.Len(t, opts, 1)
		require.True(t, opts[0].ForceToolCall)
		require.NoError(t, callbacks.OnToken("updated"))
		return "updated", &services.ToolCallMetadata{ToolsCalled: []string{"application_manager"}}, nil
	}
	classifyWriteIntent = func(ctx context.Context, userMessage string) bool {
		return true
	}
	t.Cleanup(func() {
		callGeminiToolsStreaming = previousStreaming
		classifyWriteIntent = previousClassify
	})

	router := gin.New()
	router.POST("/api/assistant/chat", ChatAssistant)
	body, err := json.Marshal(chatRequest{Message: "move application to rejected", SessionID: "test-session"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat?stream=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	raw := w.Body.String()
	firstTokenIndex := strings.Index(raw, `"token":"not written"`)
	resetIndex := strings.Index(raw, `"retry_reset":true`)
	secondTokenIndex := strings.Index(raw, `"token":"updated"`)
	doneIndex := strings.Index(raw, `"reply":"updated"`)
	require.NotEqual(t, -1, firstTokenIndex, raw)
	require.NotEqual(t, -1, resetIndex, raw)
	require.NotEqual(t, -1, secondTokenIndex, raw)
	require.NotEqual(t, -1, doneIndex, raw)
	require.Less(t, firstTokenIndex, resetIndex)
	require.Less(t, resetIndex, secondTokenIndex)
	require.Less(t, secondTokenIndex, doneIndex)
}

func TestChatAssistantStreamQualityGateStreamsOnlyFinalAnswer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	previousStreaming := callGeminiToolsStreaming
	previousGate := runChatQualityGate
	previousClassify := classifyWriteIntent
	callGeminiToolsStreaming = func(ctx context.Context, systemPrompt, userPrompt string, tools []services.ChatTool, userID int, callbacks services.ToolStreamCallbacks, opts ...services.ToolCallOption) (string, *services.ToolCallMetadata, error) {
		t.Fatal("direct streaming path should not run in enforce mode for high-value request")
		return "", nil, nil
	}
	runChatQualityGate = func(ctx context.Context, input services.QualityGateInput, generator services.ChatDraftGenerator, callbacks services.QualityGateCallbacks) (*services.QualityGateResult, error) {
		require.Equal(t, "resume_rewrite", input.Intent)
		require.NoError(t, callbacks.OnStatus("Checking answer..."))
		return &services.QualityGateResult{
			FinalAnswer: "Final quality answer",
			ToolMeta:    &services.ToolCallMetadata{},
			Mode:        string(services.QualityGateModeEnforce),
			Intent:      input.Intent,
			Score:       0.91,
		}, nil
	}
	classifyWriteIntent = func(ctx context.Context, userMessage string) bool {
		return false
	}
	t.Cleanup(func() {
		callGeminiToolsStreaming = previousStreaming
		runChatQualityGate = previousGate
		classifyWriteIntent = previousClassify
	})

	router := gin.New()
	router.POST("/api/assistant/chat", ChatAssistant)
	body, err := json.Marshal(chatRequest{Message: "rewrite my resume summary", SessionID: "test-session"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat?stream=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	raw := w.Body.String()
	statusIndex := strings.Index(raw, `"status":"Checking answer..."`)
	tokenIndex := strings.Index(raw, `"token":"Final "`)
	doneIndex := strings.Index(raw, `"reply":"Final quality answer"`)
	require.NotEqual(t, -1, statusIndex, raw)
	require.NotEqual(t, -1, tokenIndex, raw)
	require.NotEqual(t, -1, doneIndex, raw)
	require.Less(t, statusIndex, tokenIndex)
	require.Less(t, tokenIndex, doneIndex)
}

func TestChatAssistantStreamReturnsFullUpdatedResumeDataForToolPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousStreaming := callGeminiToolsStreaming
	previousClassify := classifyWriteIntent
	callGeminiToolsStreaming = func(ctx context.Context, systemPrompt, userPrompt string, tools []services.ChatTool, userID int, callbacks services.ToolStreamCallbacks, opts ...services.ToolCallOption) (string, *services.ToolCallMetadata, error) {
		return "I added the generated summary.", &services.ToolCallMetadata{
			ResumeUpdates: []map[string]any{
				{
					"resume_update": true,
					"field":         "summary",
					"action":        "set",
					"value":         "Generated professional summary",
				},
			},
		}, nil
	}
	classifyWriteIntent = func(ctx context.Context, userMessage string) bool {
		return false
	}
	t.Cleanup(func() {
		callGeminiToolsStreaming = previousStreaming
		classifyWriteIntent = previousClassify
	})

	router := gin.New()
	router.POST("/api/assistant/chat", ChatAssistant)
	body, err := json.Marshal(chatRequest{
		Message:   "generate one for me",
		SessionID: "test-session",
		ResumeData: map[string]interface{}{
			"name":    "Xuan Wu",
			"email":   "harwtalk@gmail.com",
			"summary": "",
			"skills":  "Go, React",
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat?stream=true", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	raw := w.Body.String()
	var done map[string]any
	for _, event := range strings.Split(raw, "\n\n") {
		event = strings.TrimSpace(event)
		if !strings.HasPrefix(event, "data: ") {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(event, "data: ")), &payload))
		if payload["done"] == true {
			done = payload
			break
		}
	}
	require.NotNil(t, done, raw)
	updated, ok := done["updatedResumeData"].(map[string]any)
	require.True(t, ok, raw)
	require.Equal(t, "Generated professional summary", updated["summary"])
	require.Equal(t, "Xuan Wu", updated["name"])
	require.Equal(t, "harwtalk@gmail.com", updated["email"])
	require.Equal(t, "Go, React", updated["skills"])
}

func TestBuildUpdatedResumeDataFromToolUpdatesIgnoresUnsupportedField(t *testing.T) {
	updated := buildUpdatedResumeDataFromToolUpdates(
		map[string]interface{}{
			"name": "Xuan Wu",
		},
		&services.ToolCallMetadata{
			ResumeUpdates: []map[string]any{
				{
					"resume_update": true,
					"field":         "jobTitle",
					"action":        "set",
					"value":         "Project Leader",
				},
			},
		},
	)

	require.Nil(t, updated)
}
