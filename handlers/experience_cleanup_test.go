package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanupAIResponseConvertsJSONArrayToDescriptionText(t *testing.T) {
	raw := `[
		"Architected and launched a live AI-driven resume optimization platform",
		"Developed robust evaluation workflows for AI recommendations",
		"Engineered a stateful LangChain-based orchestration layer"
	]`

	cleaned := cleanupAIResponse(raw)

	require.NotContains(t, cleaned, "[")
	require.NotContains(t, cleaned, "]")
	require.NotContains(t, cleaned, `"`)
	require.Contains(t, cleaned, "Architected and launched")
	require.Contains(t, cleaned, "Developed robust evaluation")
	require.Contains(t, cleaned, "Engineered a stateful")
	require.Len(t, strings.Split(cleaned, "\n\n"), 3)
}

func TestCleanupAIResponseConvertsFencedJSONArray(t *testing.T) {
	raw := "```json\n[\n  \"Built backend services\",\n  \"Improved job matching quality\"\n]\n```"

	cleaned := cleanupAIResponse(raw)

	require.Equal(t, "Built backend services\n\nImproved job matching quality", cleaned)
}
