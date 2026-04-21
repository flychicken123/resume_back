package services

import (
	"os"
	"strings"
)

// aiBackgroundJobsDisabled reports whether DISABLE_AI_BACKGROUND_JOBS is set to
// "true" in the environment. When true, classification and embedding goroutines
// spawned from ingestion return early so background work doesn't consume Gemini
// quota. Only affects background paths — user-facing chat, AI optimize, and
// tailoring endpoints stay live.
func aiBackgroundJobsDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_AI_BACKGROUND_JOBS")), "true")
}
