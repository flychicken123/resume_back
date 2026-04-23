package services

import (
	"regexp"
	"strings"
)

var rateLimitPattern = regexp.MustCompile(`(?i)(^|\W)(429|resource[_ ]?exhausted|rate[_ ]limit|quota exceeded)(\W|$)`)

func IsRateLimitErr(err error) bool {
	if err == nil {
		return false
	}
	return rateLimitPattern.MatchString(strings.ToLower(err.Error()))
}
