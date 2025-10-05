package services

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ResumeHashInput represents the fields that uniquely describe a resume snapshot for matching.
type ResumeHashInput struct {
	Position       string
	Name           string
	Email          string
	Summary        string
	Experience     string
	Education      string
	Location       string
	Skills         []string
	JobDescription string
	HtmlContent    string
}

// DeriveResumeHash creates a stable hash for a resume snapshot to support deduplication and retrieval.
func DeriveResumeHash(input ResumeHashInput) string {
	var builder strings.Builder

	appendField := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if builder.Len() > 0 {
			builder.WriteString("|")
		}
		builder.WriteString(trimmed)
	}

	appendField(input.Position)
	appendField(input.Name)
	appendField(input.Email)
	appendField(input.Summary)
	appendField(input.Experience)
	appendField(input.Education)
	appendField(input.JobDescription)
	appendField(input.Location)

	if len(input.Skills) > 0 {
		normalised := make([]string, 0, len(input.Skills))
		for _, skill := range input.Skills {
			if trimmed := strings.TrimSpace(skill); trimmed != "" {
				normalised = append(normalised, trimmed)
			}
		}
		sort.Strings(normalised)
		appendField(strings.Join(normalised, ","))
	}

	appendField(input.HtmlContent)

	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}
