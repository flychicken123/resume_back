package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanupProjectResponseConvertsJSONArrayToDescriptionText(t *testing.T) {
	raw := `[
		"Built a React and Go job tracking dashboard",
		"Implemented PostgreSQL search and filtering",
		"Improved deployment reliability with AWS automation"
	]`

	cleaned := cleanupProjectResponse(raw)

	require.NotContains(t, cleaned, "[")
	require.NotContains(t, cleaned, "]")
	require.NotContains(t, cleaned, `"`)
	require.Contains(t, cleaned, "Built a React and Go job tracking dashboard")
	require.Len(t, strings.Split(cleaned, "\n\n"), 3)
}

func TestCleanupProjectResponseConvertsLooseQuotedMultilineString(t *testing.T) {
	raw := `"Built a React and Go job tracking dashboard.
Implemented PostgreSQL search and filtering.
Improved deployment reliability with AWS automation."`

	cleaned := cleanupProjectResponse(raw)

	require.NotContains(t, cleaned, `"`)
	require.Equal(t, "Built a React and Go job tracking dashboard.\n\nImplemented PostgreSQL search and filtering.\n\nImproved deployment reliability with AWS automation.", cleaned)
}

func TestCleanupProjectResponseRemovesBulletPrefixes(t *testing.T) {
	raw := "- Built backend APIs\n\u2022 Improved search quality\n* Deployed AWS automation"

	cleaned := cleanupProjectResponse(raw)

	require.Equal(t, "Built backend APIs\n\nImproved search quality\n\nDeployed AWS automation", cleaned)
}

func TestCleanupProjectResponseHandlesCommonAIWrappers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "fenced json array",
			raw:  "```json\n[\n  \"Built project APIs\",\n  \"Improved search quality\"\n]\n```",
			want: "Built project APIs\n\nImproved search quality",
		},
		{
			name: "quoted json string with escaped newlines",
			raw:  `"Built project APIs.\nImproved search quality.\nDeployed AWS automation."`,
			want: "Built project APIs.\n\nImproved search quality.\n\nDeployed AWS automation.",
		},
		{
			name: "json array drops empty items",
			raw:  `["Built project APIs", "", "  ", "Improved search quality"]`,
			want: "Built project APIs\n\nImproved search quality",
		},
		{
			name: "plain text trims repeated blank lines",
			raw:  "  Built project APIs\n\n\nImproved search quality  ",
			want: "Built project APIs\n\nImproved search quality",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, cleanupProjectResponse(tt.raw))
		})
	}
}
