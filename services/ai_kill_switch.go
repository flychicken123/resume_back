package services

import (
	"os"
	"strings"
)

const DisableJobAutoClassificationEnv = "DISABLE_JOB_AUTO_CLASSIFICATION"

// aiBackgroundJobsDisabled reports whether scheduled/background AI loops should
// stop to avoid consuming Gemini quota. User-facing AI endpoints stay live.
func aiBackgroundJobsDisabled() bool {
	return envFlagEnabled("DISABLE_AI_BACKGROUND_JOBS")
}

// JobAutoClassificationDisabled reports whether newly pulled jobs should skip
// automatic classification. This is intentionally separate from
// DISABLE_AI_BACKGROUND_JOBS so job pulls can still classify fresh postings
// while broader scheduled AI work is paused.
func JobAutoClassificationDisabled() bool {
	return envFlagEnabled(DisableJobAutoClassificationEnv)
}

func envFlagEnabled(name string) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(name)), "true")
}
