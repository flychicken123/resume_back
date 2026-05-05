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
