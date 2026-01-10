package services

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	yoeTier1Buffer  = 2   // Can stretch up to M+2 for Tier 1
	yoeTier2Buffer  = 5   // Can stretch up to M+5 for Tier 2
	yoeTier1Bonus   = 3.0 // Score bonus for Tier 1 matches
	yoeTier2Penalty = -2.0 // Score penalty for Tier 2 matches
)

// YOETier represents the matching tier for YOE alignment
type YOETier int

const (
	YOETierUnknown YOETier = iota // Cannot determine (missing data)
	YOETier1                      // Best match: Y <= M+2
	YOETier2                      // Stretch: M+2 < Y <= M+5
	YOETier3                      // Mismatch: Y > M+5 (should be filtered)
)

// YOEResult holds the extracted years of experience
type YOEResult struct {
	Years     float64 // Total years of experience
	Confident bool    // Whether we're confident in the extraction
}

// JobYOERequirement holds the extracted YOE requirement from a job posting
type JobYOERequirement struct {
	MinYears  float64 // Minimum years required (0 if not specified)
	MaxYears  float64 // Maximum years if range given (0 if not specified)
	Found     bool    // Whether a YOE requirement was found
}

// Common date patterns for parsing experience entries
var (
	// Matches patterns like "Jan 2020", "January 2020", "2020"
	datePatternMonthYear = regexp.MustCompile(`(?i)(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s*,?\s*(\d{4})`)
	datePatternYearOnly  = regexp.MustCompile(`\b(20\d{2}|19\d{2})\b`)

	// Matches "Present", "Current", "Now", etc.
	presentPattern = regexp.MustCompile(`(?i)\b(present|current|now|ongoing|today)\b`)

	// Matches date ranges like "Jan 2020 - Present", "2018-2022", "2019 to 2021"
	dateRangePattern = regexp.MustCompile(`(?i)(\d{4}|\w+\s*,?\s*\d{4})\s*[-–—to]+\s*(\d{4}|\w+\s*,?\s*\d{4}|present|current|now|ongoing|today)`)

	// YOE requirement patterns in job descriptions
	yoePatterns = []*regexp.Regexp{
		// "5+ years", "5+ yrs"
		regexp.MustCompile(`(?i)(\d+)\+?\s*(?:years?|yrs?)(?:\s+of)?(?:\s+(?:professional|relevant|related|hands-on|work))?\s*(?:experience)?`),
		// "minimum 5 years", "at least 5 years"
		regexp.MustCompile(`(?i)(?:minimum|at\s+least|min\.?)\s*(\d+)\s*(?:years?|yrs?)`),
		// "5-7 years", "5 to 7 years"
		regexp.MustCompile(`(?i)(\d+)\s*[-–—to]+\s*(\d+)\s*(?:years?|yrs?)`),
		// "5 years required", "5 years of experience required"
		regexp.MustCompile(`(?i)(\d+)\s*(?:years?|yrs?)(?:\s+of\s+(?:\w+\s+)*experience)?\s+(?:required|needed|necessary)`),
		// "experience: 5+ years"
		regexp.MustCompile(`(?i)experience\s*:\s*(\d+)\+?\s*(?:years?|yrs?)?`),
	}

	// Month name to number mapping
	monthMap = map[string]int{
		"jan": 1, "january": 1,
		"feb": 2, "february": 2,
		"mar": 3, "march": 3,
		"apr": 4, "april": 4,
		"may": 5,
		"jun": 6, "june": 6,
		"jul": 7, "july": 7,
		"aug": 8, "august": 8,
		"sep": 9, "september": 9,
		"oct": 10, "october": 10,
		"nov": 11, "november": 11,
		"dec": 12, "december": 12,
	}
)

// ExtractYOEFromExperience extracts total years of experience from resume experience text.
// It parses date ranges and calculates the total duration, handling overlaps.
func ExtractYOEFromExperience(experience string) YOEResult {
	if strings.TrimSpace(experience) == "" {
		return YOEResult{Years: 0, Confident: false}
	}

	// Find all date ranges in the experience text
	ranges := extractDateRanges(experience)
	if len(ranges) == 0 {
		// Fallback: try to extract just years and estimate
		return estimateYOEFromYears(experience)
	}

	// Calculate total duration, merging overlapping periods
	totalMonths := calculateTotalMonths(ranges)
	years := float64(totalMonths) / 12.0

	// Round to 1 decimal place
	years = float64(int(years*10)) / 10.0

	return YOEResult{
		Years:     years,
		Confident: len(ranges) > 0,
	}
}

// dateRange represents a period of employment
type dateRange struct {
	start time.Time
	end   time.Time
}

// extractDateRanges finds all date ranges in the experience text
func extractDateRanges(text string) []dateRange {
	var ranges []dateRange

	matches := dateRangePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			start := parseDate(match[1])
			end := parseDate(match[2])

			if !start.IsZero() && !end.IsZero() && end.After(start) {
				ranges = append(ranges, dateRange{start: start, end: end})
			}
		}
	}

	return ranges
}

// parseDate parses various date formats into time.Time
func parseDate(dateStr string) time.Time {
	dateStr = strings.TrimSpace(dateStr)
	lower := strings.ToLower(dateStr)

	// Check for "present", "current", etc.
	if presentPattern.MatchString(lower) {
		return time.Now()
	}

	// Try month + year pattern
	if matches := datePatternMonthYear.FindStringSubmatch(dateStr); len(matches) >= 3 {
		monthStr := strings.ToLower(matches[1])
		yearStr := matches[2]

		month, ok := monthMap[monthStr]
		if !ok {
			// Try abbreviated version
			if len(monthStr) >= 3 {
				month, ok = monthMap[monthStr[:3]]
			}
		}

		year, err := strconv.Atoi(yearStr)
		if err == nil && ok {
			return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		}
	}

	// Try year-only pattern
	if matches := datePatternYearOnly.FindStringSubmatch(dateStr); len(matches) >= 2 {
		year, err := strconv.Atoi(matches[1])
		if err == nil {
			// Assume middle of the year if only year is given
			return time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC)
		}
	}

	return time.Time{}
}

// calculateTotalMonths calculates total months from date ranges, merging overlaps
func calculateTotalMonths(ranges []dateRange) int {
	if len(ranges) == 0 {
		return 0
	}

	// Sort ranges by start date
	for i := 0; i < len(ranges)-1; i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j].start.Before(ranges[i].start) {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}

	// Merge overlapping ranges
	merged := []dateRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		current := ranges[i]

		// Check if current overlaps with last
		if current.start.Before(last.end) || current.start.Equal(last.end) {
			// Extend the end if current ends later
			if current.end.After(last.end) {
				last.end = current.end
			}
		} else {
			// No overlap, add as new range
			merged = append(merged, current)
		}
	}

	// Calculate total months from merged ranges
	totalMonths := 0
	for _, r := range merged {
		months := int(r.end.Sub(r.start).Hours() / 24 / 30)
		if months > 0 {
			totalMonths += months
		}
	}

	return totalMonths
}

// estimateYOEFromYears is a fallback that estimates YOE from years mentioned
func estimateYOEFromYears(text string) YOEResult {
	years := datePatternYearOnly.FindAllString(text, -1)
	if len(years) < 2 {
		return YOEResult{Years: 0, Confident: false}
	}

	// Find min and max years
	minYear, maxYear := 9999, 0
	for _, y := range years {
		year, err := strconv.Atoi(y)
		if err != nil {
			continue
		}
		if year < minYear {
			minYear = year
		}
		if year > maxYear {
			maxYear = year
		}
	}

	currentYear := time.Now().Year()
	if maxYear > currentYear {
		maxYear = currentYear
	}

	if maxYear > minYear && minYear > 1990 {
		yoe := float64(maxYear - minYear)
		return YOEResult{Years: yoe, Confident: false}
	}

	return YOEResult{Years: 0, Confident: false}
}

// ExtractYOEFromJobDescription extracts years of experience requirement from job description
func ExtractYOEFromJobDescription(description string) JobYOERequirement {
	if strings.TrimSpace(description) == "" {
		return JobYOERequirement{Found: false}
	}

	lower := strings.ToLower(description)

	for _, pattern := range yoePatterns {
		matches := pattern.FindStringSubmatch(lower)
		if len(matches) >= 2 {
			minYears, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				continue
			}

			result := JobYOERequirement{
				MinYears: minYears,
				Found:    true,
			}

			// Check if there's a max years (for ranges like "5-7 years")
			if len(matches) >= 3 && matches[2] != "" {
				maxYears, err := strconv.ParseFloat(matches[2], 64)
				if err == nil && maxYears > minYears {
					result.MaxYears = maxYears
				}
			}

			return result
		}
	}

	return JobYOERequirement{Found: false}
}

// ComputeYOETier determines the YOE tier based on candidate experience vs job requirement
// Returns the tier and the score adjustment
func ComputeYOETier(candidateYOE float64, jobRequirement JobYOERequirement) (YOETier, float64) {
	// If job doesn't specify YOE, treat as Tier 1 with no bonus/penalty
	if !jobRequirement.Found {
		return YOETier1, 0
	}

	// Use minimum required years for comparison
	requiredYOE := jobRequirement.MinYears

	// If candidate YOE is unknown (0), we can't determine tier
	if candidateYOE <= 0 {
		return YOETierUnknown, 0
	}

	// Calculate the gap (positive means job requires more than candidate has)
	gap := requiredYOE - candidateYOE

	// Tier 1: Job requires <= candidate's YOE + buffer (up to M+2)
	// This includes: candidate is overqualified OR slight stretch
	if gap <= float64(yoeTier1Buffer) {
		return YOETier1, yoeTier1Bonus
	}

	// Tier 2: Job requires more but within M+5
	if gap <= float64(yoeTier2Buffer) {
		return YOETier2, yoeTier2Penalty
	}

	// Tier 3: Job requires way more than candidate has
	return YOETier3, 0 // Score will be set to 0 (filtered out)
}

// ComputeYOEScoreAdjustment is a convenience function that returns just the score adjustment
func ComputeYOEScoreAdjustment(candidateYOE float64, jobRequirement JobYOERequirement) float64 {
	_, adjustment := ComputeYOETier(candidateYOE, jobRequirement)
	return adjustment
}

// IsYOETier3 checks if the job should be filtered out due to YOE mismatch
func IsYOETier3(candidateYOE float64, jobRequirement JobYOERequirement) bool {
	tier, _ := ComputeYOETier(candidateYOE, jobRequirement)
	return tier == YOETier3
}
