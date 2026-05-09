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

func TestCleanupAIResponseConvertsJSONStringWithEscapedNewlines(t *testing.T) {
	raw := `"Architected and launched a live AI-driven resume platform.\nEngineered retrieval and ranking systems.\nDeveloped a responsive React SPA."`

	cleaned := cleanupAIResponse(raw)

	require.NotContains(t, cleaned, `\n`)
	require.NotContains(t, cleaned, `"`)
	require.Equal(t, "Architected and launched a live AI-driven resume platform.\n\nEngineered retrieval and ranking systems.\n\nDeveloped a responsive React SPA.", cleaned)
}

func TestCleanupAIResponseConvertsLooseQuotedMultilineString(t *testing.T) {
	raw := `"Architected and launched a live AI-driven resume platform.
Engineered retrieval and ranking systems.
Developed a responsive React SPA."`

	cleaned := cleanupAIResponse(raw)

	require.NotContains(t, cleaned, `"`)
	require.Equal(t, "Architected and launched a live AI-driven resume platform.\n\nEngineered retrieval and ranking systems.\n\nDeveloped a responsive React SPA.", cleaned)
}

func TestCleanupAIResponseHandlesCommonWrappers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "plain text trims blank lines",
			raw:  "  Built backend services\n\n\nImproved job matching quality  ",
			want: "Built backend services\n\nImproved job matching quality",
		},
		{
			name: "plain fenced json array",
			raw:  "```\n[\n  \"Built backend services\",\n  \"Improved job matching quality\"\n]\n```",
			want: "Built backend services\n\nImproved job matching quality",
		},
		{
			name: "json array drops blank entries",
			raw:  `["Built backend services", "", "  ", "Improved job matching quality"]`,
			want: "Built backend services\n\nImproved job matching quality",
		},
		{
			name: "bullet prefixes are removed",
			raw:  "- Built backend services\n* Improved job matching quality\n\u2022 Deployed AWS automation",
			want: "Built backend services\n\nImproved job matching quality\n\nDeployed AWS automation",
		},
		{
			name: "quoted json string decodes escaped quote and tab",
			raw:  `"Built \"AI\" services.\nImproved\tjob matching quality."`,
			want: "Built \"AI\" services.\n\nImproved\tjob matching quality.",
		},
		{
			name: "loose quoted text decodes escaped newline",
			raw:  `"Built backend services.\nImproved job matching quality."`,
			want: "Built backend services.\n\nImproved job matching quality.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, cleanupAIResponse(tt.raw))
		})
	}
}
