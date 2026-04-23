package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"resumeai/services"
)

func withNoOpSleep(t *testing.T) {
	t.Helper()
	orig := chatCallSleep
	chatCallSleep = func(time.Duration) {}
	t.Cleanup(func() { chatCallSleep = orig })
}

func TestChatRetry_RateLimitErr_NoToolsFired_Retries(t *testing.T) {
	withNoOpSleep(t)
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		if calls < 2 {
			return "", &services.ToolCallMetadata{ToolsCalled: nil}, errors.New("429")
		}
		return "final reply", &services.ToolCallMetadata{}, nil
	}
	reply, _, err := chatCallWithRetry(fn)
	assert.NoError(t, err)
	assert.Equal(t, "final reply", reply)
	assert.Equal(t, 2, calls)
}

func TestChatRetry_RateLimitErr_ToolsFired_NoRetry(t *testing.T) {
	withNoOpSleep(t)
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		return "", &services.ToolCallMetadata{ToolsCalled: []string{"updateApp"}},
			errors.New("429 after tool")
	}
	_, _, err := chatCallWithRetry(fn)
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestChatRetry_NonRateLimitErr_NoRetry(t *testing.T) {
	withNoOpSleep(t)
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		return "", nil, errors.New("boom unrelated")
	}
	_, _, err := chatCallWithRetry(fn)
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestChatRetry_AllRetriesExhausted_ReturnsRateLimitErr(t *testing.T) {
	withNoOpSleep(t)
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		return "", &services.ToolCallMetadata{}, errors.New("429")
	}
	_, _, err := chatCallWithRetry(fn)
	assert.Error(t, err)
	assert.True(t, services.IsRateLimitErr(err))
	assert.Equal(t, 3, calls)
}

func TestChatRetry_SuccessFirstAttempt_ReturnsReply(t *testing.T) {
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		return "hello", &services.ToolCallMetadata{}, nil
	}
	reply, _, err := chatCallWithRetry(fn)
	assert.NoError(t, err)
	assert.Equal(t, "hello", reply)
	assert.Equal(t, 1, calls)
}

func TestChatRetry_FeatureFlagDisabled_RunsOnce(t *testing.T) {
	t.Setenv("GEMINI_RETRY_ENABLED", "false")
	withNoOpSleep(t)
	calls := 0
	fn := func() (string, *services.ToolCallMetadata, error) {
		calls++
		return "", &services.ToolCallMetadata{}, errors.New("429")
	}
	_, _, err := chatCallWithRetry(fn)
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}
