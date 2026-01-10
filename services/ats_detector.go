package services

import (
	"net/url"
	"strings"
)

// DetectATSProvider returns a known ATS provider slug based on the careers URL host/path.
// It uses simple substring heuristics and returns an empty string if no match is found.
func DetectATSProvider(careersURL string) string {
	trimmed := strings.TrimSpace(careersURL)
	if trimmed == "" {
		return ""
	}

	rawLower := strings.ToLower(trimmed)

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		parsed, _ = url.Parse("https://" + trimmed)
	}

	host := strings.ToLower(parsed.Host)
	if host == "" {
		host = rawLower
	}
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}

	type matcher struct {
		provider   string
		substrings []string
	}

	matchers := []matcher{
		{provider: "greenhouse", substrings: []string{"boards.greenhouse.io", "job-boards.greenhouse.io", "greenhouse.io", "grnh.se"}},
		{provider: "lever", substrings: []string{"jobs.lever.co", "lever.co"}},
		{provider: "workday", substrings: []string{"myworkdayjobs.com", ".workday.", "workday.com"}},
		{provider: "ashby", substrings: []string{"ashbyhq.com"}},
		{provider: "smartrecruiters", substrings: []string{"smartrecruiters.com"}},
		{provider: "workable", substrings: []string{"workable.com"}},
		{provider: "bamboohr", substrings: []string{"bamboohr.com"}},
		{provider: "recruitee", substrings: []string{"recruitee.com"}},
		{provider: "teamtailor", substrings: []string{"teamtailor.com"}},
	}

	for _, m := range matchers {
		for _, needle := range m.substrings {
			if strings.Contains(host, needle) || strings.Contains(rawLower, needle) {
				return m.provider
			}
		}
	}

	return ""
}
