package utils

import "strings"

const (
	SeniorityIntern = iota
	SeniorityEntry
	SeniorityMid
	SenioritySenior
	SeniorityLead
)

var (
	seniorityLeadKeywords   = []string{"lead", "principal", "manager", "director", "head", "vp", "svp", "evp", "chief", "cto", "cpo", "cio", "cfo", "coo", "vice president"}
	senioritySeniorKeywords = []string{"senior", "sr", "staff"}
	seniorityEntryKeywords  = []string{"junior", "jr", "associate", "entry", "graduate"}
	seniorityInternKeywords = []string{"intern", "internship", "co-op", "coop", "trainee", "apprentice"}
)

func NormalizeSeniorityString(value string) string {
	lower := strings.ToLower(value)
	replacer := strings.NewReplacer(".", " ", "-", " ", "_", " ", "/", " ", ",", " ")
	cleaned := replacer.Replace(lower)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned
}

func containsAnySeniority(value string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(value, kw) {
			return true
		}
	}
	return false
}

func DetectSeniorityFromString(value string) int {
	normalized := NormalizeSeniorityString(value)
	if normalized == "" {
		return -1
	}

	switch {
	case containsAnySeniority(normalized, seniorityInternKeywords):
		return SeniorityIntern
	case containsAnySeniority(normalized, seniorityLeadKeywords):
		return SeniorityLead
	case containsAnySeniority(normalized, senioritySeniorKeywords):
		return SenioritySenior
	case containsAnySeniority(normalized, seniorityEntryKeywords):
		return SeniorityEntry
	default:
		return -1
	}
}

func DetermineJobSeniority(title, description string) int {
	level := DetectSeniorityFromString(title)
	descriptionLevel := DetectSeniorityFromString(description)
	if descriptionLevel > level {
		level = descriptionLevel
	}
	if level < 0 {
		level = SeniorityMid
	}
	return level
}

func DetermineCandidateSeniority(position, resumeText string) int {
	level := DetectSeniorityFromString(position)
	resumeLevel := DetectSeniorityFromString(resumeText)
	if resumeLevel > level {
		level = resumeLevel
	}
	if level < 0 {
		level = SeniorityMid
	}
	return level
}

func JobLooksLikeInternRole(title, description string) bool {
	return DetermineJobSeniority(title, description) == SeniorityIntern
}
