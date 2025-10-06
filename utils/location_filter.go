package utils

import (
	"strings"
	"unicode"
)

var supportedLocationPhrases = []string{
	"united states of america",
	"united states",
	"continental united states",
	"contiguous united states",
	"usa",
	"canada",
	"united kingdom",
	"great britain",
	"britain",
	"england",
	"scotland",
	"wales",
	"northern ireland",
	"alabama",
	"alaska",
	"arizona",
	"arkansas",
	"california",
	"colorado",
	"connecticut",
	"delaware",
	"florida",
	"georgia",
	"hawaii",
	"idaho",
	"illinois",
	"indiana",
	"iowa",
	"kansas",
	"kentucky",
	"louisiana",
	"maine",
	"maryland",
	"massachusetts",
	"michigan",
	"minnesota",
	"mississippi",
	"missouri",
	"montana",
	"nebraska",
	"nevada",
	"new hampshire",
	"new jersey",
	"new mexico",
	"new york",
	"north carolina",
	"north dakota",
	"ohio",
	"oklahoma",
	"oregon",
	"pennsylvania",
	"rhode island",
	"south carolina",
	"south dakota",
	"tennessee",
	"texas",
	"utah",
	"vermont",
	"virginia",
	"washington",
	"west virginia",
	"wisconsin",
	"wyoming",
	"district of columbia",
	"alberta",
	"british columbia",
	"manitoba",
	"new brunswick",
	"newfoundland and labrador",
	"nova scotia",
	"ontario",
	"prince edward island",
	"quebec",
	"saskatchewan",
	"yukon",
	"northwest territories",
	"nunavut",
	"toronto",
	"vancouver",
	"montreal",
	"ottawa",
	"calgary",
	"edmonton",
	"winnipeg",
	"hamilton",
	"london",
	"manchester",
	"birmingham",
	"leeds",
	"glasgow",
	"edinburgh",
	"bristol",
	"cardiff",
	"belfast",
	"liverpool",
	"sheffield",
	"newcastle",
	"oxford",
	"cambridge",
}

var supportedLocationTokens = map[string]struct{}{
	"us":    {},
	"usa":   {},
	"u.s":   {},
	"u.s.a": {},
	"uk":    {},
	"gb":    {},
	"eng":   {},
	"scot":  {},
	"wales": {},
	"ni":    {},
	"dc":    {},
	"al":    {},
	"ak":    {},
	"az":    {},
	"ar":    {},
	"ca":    {},
	"co":    {},
	"ct":    {},
	"de":    {},
	"fl":    {},
	"ga":    {},
	"hi":    {},
	"id":    {},
	"il":    {},
	"in":    {},
	"ia":    {},
	"ks":    {},
	"ky":    {},
	"la":    {},
	"me":    {},
	"md":    {},
	"ma":    {},
	"mi":    {},
	"mn":    {},
	"ms":    {},
	"mo":    {},
	"mt":    {},
	"ne":    {},
	"nv":    {},
	"nh":    {},
	"nj":    {},
	"nm":    {},
	"ny":    {},
	"nc":    {},
	"nd":    {},
	"oh":    {},
	"ok":    {},
	"or":    {},
	"pa":    {},
	"ri":    {},
	"sc":    {},
	"sd":    {},
	"tn":    {},
	"tx":    {},
	"ut":    {},
	"vt":    {},
	"va":    {},
	"wa":    {},
	"wi":    {},
	"wv":    {},
	"wy":    {},
	"ab":    {},
	"bc":    {},
	"mb":    {},
	"nb":    {},
	"nl":    {},
	"ns":    {},
	"on":    {},
	"pe":    {},
	"qc":    {},
	"sk":    {},
	"nt":    {},
	"nu":    {},
	"yt":    {},
}
var usLocationKeywords = []string{
	"united states of america",
	"united states",
	"usa",
	"us only",
	"united states only",
	"remote us",
	"us remote",
	"alabama",
	"alaska",
	"arizona",
	"arkansas",
	"california",
	"colorado",
	"connecticut",
	"delaware",
	"florida",
	"georgia",
	"hawaii",
	"idaho",
	"illinois",
	"indiana",
	"iowa",
	"kansas",
	"kentucky",
	"louisiana",
	"maine",
	"maryland",
	"massachusetts",
	"michigan",
	"minnesota",
	"mississippi",
	"missouri",
	"montana",
	"nebraska",
	"nevada",
	"new hampshire",
	"new jersey",
	"new mexico",
	"new york",
	"north carolina",
	"north dakota",
	"ohio",
	"oklahoma",
	"oregon",
	"pennsylvania",
	"rhode island",
	"south carolina",
	"south dakota",
	"tennessee",
	"texas",
	"utah",
	"vermont",
	"virginia",
	"washington",
	"west virginia",
	"wisconsin",
	"wyoming",
	"district of columbia",
	"washington dc",
}

var canadaLocationKeywords = []string{
	"canada",
	"remote canada",
	"canadian",
	"ontario",
	"british columbia",
	"alberta",
	"saskatchewan",
	"manitoba",
	"quebec",
	"new brunswick",
	"nova scotia",
	"newfoundland and labrador",
	"prince edward island",
	"pei",
	"yukon",
	"northwest territories",
	"nunavut",
	"toronto",
	"vancouver",
	"montreal",
	"ottawa",
	"calgary",
	"edmonton",
	"winnipeg",
	"halifax",
	"victoria",
	"waterloo",
}

// IsSupportedJobLocation returns true if the given job location is within the supported geographies (US, Canada, UK).
func IsSupportedJobLocation(location, remoteType string) bool {
	combined := strings.TrimSpace(location + " " + remoteType)
	if combined == "" {
		return false
	}

	sanitized := sanitizeLocationString(combined)
	normalized := normalizeLocationString(sanitized)
	if normalized == "" {
		return false
	}

	for _, phrase := range supportedLocationPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}

	tokens := strings.Fields(normalized)
	for _, token := range tokens {
		if _, ok := supportedLocationTokens[token]; ok {
			return true
		}
	}

	return false
}

func sanitizeLocationString(value string) string {
	lower := strings.ToLower(value)
	replacements := []struct{ old, new string }{
		{"u.s.a", "usa"},
		{"u.s.", "us"},
		{"u.k.", "uk"},
		{"(us)", "us"},
		{"(uk)", "uk"},
		{"(ca)", "ca"},
		{"ã¼", "u"},
	}

	for _, repl := range replacements {
		lower = strings.ReplaceAll(lower, repl.old, repl.new)
	}

	return lower
}

func normalizeLocationString(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		} else {
			builder.WriteByte(' ')
		}
	}

	return strings.Join(strings.Fields(builder.String()), " ")
}

func containsLocationKeyword(normalized string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func LooksUSLocation(value string) bool {
	normalized := normalizeLocationString(sanitizeLocationString(value))
	if normalized == "" {
		return false
	}
	return containsLocationKeyword(normalized, usLocationKeywords)
}

func LooksCanadianLocation(value string) bool {
	normalized := normalizeLocationString(sanitizeLocationString(value))
	if normalized == "" {
		return false
	}
	return containsLocationKeyword(normalized, canadaLocationKeywords)
}
