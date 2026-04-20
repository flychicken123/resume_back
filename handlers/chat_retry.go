package handlers

import (
	"os"
	"strings"
	"time"

	"resumeai/services"
)

// chatCallSleep is an indirection so tests can observe/replace the sleep.
var chatCallSleep = time.Sleep

// chatCallWithRetry wraps the tool-calling Gemini invocation with a 3-attempt
// exponential backoff on rate-limit errors only. Skips retry once any tool has
// fired to avoid duplicate tool invocation — see Category B1 of the plan.
//
// Kill switch: if GEMINI_RETRY_ENABLED is set to "false", fn runs exactly once
// with no retries.
func chatCallWithRetry(fn func() (string, *services.ToolCallMetadata, error)) (string, *services.ToolCallMetadata, error) {
	if !chatRetryEnabled() {
		return fn()
	}

	backoffs := []time.Duration{0, 1 * time.Second, 2 * time.Second}
	var lastReply string
	var lastMeta *services.ToolCallMetadata
	var lastErr error

	for attempt, backoff := range backoffs {
		if backoff > 0 {
			chatCallSleep(backoff)
		}

		reply, meta, err := fn()
		lastReply, lastMeta, lastErr = reply, meta, err

		if err == nil {
			return reply, meta, nil
		}
		if !services.IsRateLimitErr(err) {
			return reply, meta, err
		}
		if meta != nil && len(meta.ToolsCalled) > 0 {
			return reply, meta, err
		}
		_ = attempt
	}
	return lastReply, lastMeta, lastErr
}

func chatRetryEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GEMINI_RETRY_ENABLED")))
	return v != "false"
}
