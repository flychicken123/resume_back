package services

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3ResumeExtractor struct {
	s3Service *S3Service
}

type ExtractedExperience struct {
	JobTitle         string    `json:"job_title"`
	Company          string    `json:"company"`
	Location         string    `json:"location"`
	StartDate        string    `json:"start_date"`
	EndDate          string    `json:"end_date"`
	CurrentlyWorking bool      `json:"currently_working"`
	Description      []string  `json:"description"`
	Duration         string    `json:"duration"`
}

type S3ResumeData struct {
	PersonalInfo struct {
		Name      string   `json:"name"`
		Email     string   `json:"email"`
		Phone     string   `json:"phone"`
		Location  string   `json:"location"`
		LinkedIn  string   `json:"linkedin"`
		GitHub    string   `json:"github"`
		Portfolio string   `json:"portfolio"`
	} `json:"personal_info"`
	
	Summary     string                `json:"summary"`
	Skills      []string              `json:"skills"`
	Experience  []ExtractedExperience `json:"experience"`
	Education   []struct {
		Degree         string `json:"degree"`
		School         string `json:"school"`
		Field          string `json:"field"`
		GraduationYear int    `json:"graduation_year"`
		GPA            string `json:"gpa"`
		Location       string `json:"location"`
	} `json:"education"`
}

func NewS3ResumeExtractor(s3Service *S3Service) *S3ResumeExtractor {
	return &S3ResumeExtractor{
		s3Service: s3Service,
	}
}

func (e *S3ResumeExtractor) ExtractFromS3(s3Path string) (*S3ResumeData, error) {
	// Download resume from S3
	content, err := e.downloadResumeFromS3(s3Path)
	if err != nil {
		return nil, fmt.Errorf("failed to download resume from S3: %v", err)
	}

	// Determine file type and extract accordingly
	if strings.HasSuffix(strings.ToLower(s3Path), ".pdf") {
		return e.extractFromPDF(content)
	} else if strings.HasSuffix(strings.ToLower(s3Path), ".html") {
		return e.extractFromHTML(content)
	} else {
		return nil, fmt.Errorf("unsupported file type: %s", s3Path)
	}
}

func (e *S3ResumeExtractor) downloadResumeFromS3(s3Path string) ([]byte, error) {
	// Use existing S3Service to download file
	bucket := "airesumestorage" // Use from config
	
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Path),
	}

	result, err := e.s3Service.s3Client.GetObject(input)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func (e *S3ResumeExtractor) extractFromHTML(content []byte) (*S3ResumeData, error) {
	htmlContent := string(content)
	resumeData := &S3ResumeData{}

	// Extract personal information
	resumeData.PersonalInfo.Name = e.extractWithRegex(htmlContent, `<h1[^>]*>([^<]+)</h1>`, 1)
	resumeData.PersonalInfo.Email = e.extractWithRegex(htmlContent, `([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`, 1)
	resumeData.PersonalInfo.Phone = e.extractWithRegex(htmlContent, `(\+?1?[-.\s]?\(?[0-9]{3}\)?[-.\s]?[0-9]{3}[-.\s]?[0-9]{4})`, 1)
	
	// Extract LinkedIn
	linkedinRegex := regexp.MustCompile(`(?i)linkedin\.com/in/([a-zA-Z0-9-]+)`)
	if matches := linkedinRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		resumeData.PersonalInfo.LinkedIn = "https://linkedin.com/in/" + matches[1]
	}

	// Extract GitHub
	githubRegex := regexp.MustCompile(`(?i)github\.com/([a-zA-Z0-9-]+)`)
	if matches := githubRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		resumeData.PersonalInfo.GitHub = "https://github.com/" + matches[1]
	}

	// Extract summary/objective
	summaryPatterns := []string{
		`(?i)<h2[^>]*>\s*(?:summary|objective|profile)\s*</h2>\s*<[^>]*>\s*([^<]+)`,
		`(?i)<h3[^>]*>\s*(?:summary|objective|profile)\s*</h3>\s*<[^>]*>\s*([^<]+)`,
	}
	for _, pattern := range summaryPatterns {
		if summary := e.extractWithRegex(htmlContent, pattern, 1); summary != "" {
			resumeData.Summary = e.cleanText(summary)
			break
		}
	}

	// Extract experience section
	resumeData.Experience = e.extractExperienceFromHTML(htmlContent)

	// Extract skills
	resumeData.Skills = e.extractSkillsFromHTML(htmlContent)

	// Extract education
	resumeData.Education = e.extractEducationFromHTML(htmlContent)

	return resumeData, nil
}

func (e *S3ResumeExtractor) extractFromPDF(content []byte) (*S3ResumeData, error) {
	// For PDF extraction, we'd need a PDF parsing library
	// For now, return an error suggesting HTML extraction
	return nil, fmt.Errorf("PDF extraction not yet implemented - please use HTML resume format for automatic extraction")
}

func (e *S3ResumeExtractor) extractExperienceFromHTML(htmlContent string) []ExtractedExperience {
	var experiences []ExtractedExperience

	// Look for experience sections
	expSectionRegex := regexp.MustCompile(`(?i)<h2[^>]*>\s*(?:experience|work\s+experience|employment)\s*</h2>(.*?)(?:<h2|$)`)
	expMatches := expSectionRegex.FindStringSubmatch(htmlContent)
	
	if len(expMatches) < 2 {
		return experiences
	}

	expSection := expMatches[1]

	// Extract individual job entries
	jobRegex := regexp.MustCompile(`<h3[^>]*>([^<]+)</h3>\s*<[^>]*>([^<]*)</[^>]*>\s*<[^>]*>([^<]*)</[^>]*>`)
	jobMatches := jobRegex.FindAllStringSubmatch(expSection, -1)

	for _, match := range jobMatches {
		if len(match) >= 4 {
			exp := ExtractedExperience{
				JobTitle: e.cleanText(match[1]),
				Company:  e.cleanText(match[2]),
			}

			// Parse dates from third match
			dateStr := e.cleanText(match[3])
			exp.StartDate, exp.EndDate, exp.CurrentlyWorking = e.parseDateRange(dateStr)
			exp.Duration = e.calculateDuration(exp.StartDate, exp.EndDate, exp.CurrentlyWorking)

			// Extract location and description (simplified)
			exp.Location = e.extractLocationFromJobEntry(expSection, match[0])
			exp.Description = e.extractJobDescription(expSection, match[0])

			experiences = append(experiences, exp)
		}
	}

	return experiences
}

func (e *S3ResumeExtractor) extractSkillsFromHTML(htmlContent string) []string {
	var skills []string

	// Look for skills section
	skillsRegex := regexp.MustCompile(`(?i)<h2[^>]*>\s*(?:skills|technical\s+skills|core\s+competencies)\s*</h2>(.*?)(?:<h2|$)`)
	skillsMatches := skillsRegex.FindStringSubmatch(htmlContent)
	
	if len(skillsMatches) >= 2 {
		skillsSection := skillsMatches[1]
		
		// Remove HTML tags and split by common delimiters
		cleanSkills := e.stripHTMLTags(skillsSection)
		skillsList := regexp.MustCompile(`[,•|•\n\r]+`).Split(cleanSkills, -1)
		
		for _, skill := range skillsList {
			skill = strings.TrimSpace(skill)
			if len(skill) > 1 && len(skill) < 50 { // Filter reasonable skill lengths
				skills = append(skills, skill)
			}
		}
	}

	return skills
}

func (e *S3ResumeExtractor) extractEducationFromHTML(htmlContent string) []struct {
	Degree         string `json:"degree"`
	School         string `json:"school"`
	Field          string `json:"field"`
	GraduationYear int    `json:"graduation_year"`
	GPA            string `json:"gpa"`
	Location       string `json:"location"`
} {
	var education []struct {
		Degree         string `json:"degree"`
		School         string `json:"school"`
		Field          string `json:"field"`
		GraduationYear int    `json:"graduation_year"`
		GPA            string `json:"gpa"`
		Location       string `json:"location"`
	}

	// Look for education section
	eduRegex := regexp.MustCompile(`(?i)<h2[^>]*>\s*(?:education|academic\s+background)\s*</h2>(.*?)(?:<h2|$)`)
	eduMatches := eduRegex.FindStringSubmatch(htmlContent)
	
	if len(eduMatches) >= 2 {
		eduSection := eduMatches[1]
		
		// Extract degree entries
		degreeRegex := regexp.MustCompile(`<h3[^>]*>([^<]+)</h3>\s*<[^>]*>([^<]*)</[^>]*>`)
		degreeMatches := degreeRegex.FindAllStringSubmatch(eduSection, -1)

		for _, match := range degreeMatches {
			if len(match) >= 3 {
				edu := struct {
					Degree         string `json:"degree"`
					School         string `json:"school"`
					Field          string `json:"field"`
					GraduationYear int    `json:"graduation_year"`
					GPA            string `json:"gpa"`
					Location       string `json:"location"`
				}{
					Degree: e.cleanText(match[1]),
					School: e.cleanText(match[2]),
				}

				// Try to extract graduation year
				yearRegex := regexp.MustCompile(`(20\d{2}|19\d{2})`)
				if yearMatch := yearRegex.FindString(match[0]); yearMatch != "" {
					if year, err := strconv.Atoi(yearMatch); err == nil {
						edu.GraduationYear = year
					}
				}

				education = append(education, edu)
			}
		}
	}

	return education
}

// Helper methods

func (e *S3ResumeExtractor) extractWithRegex(content, pattern string, group int) string {
	regex := regexp.MustCompile(pattern)
	matches := regex.FindStringSubmatch(content)
	if len(matches) > group {
		return e.cleanText(matches[group])
	}
	return ""
}

func (e *S3ResumeExtractor) cleanText(text string) string {
	// Remove HTML tags
	text = e.stripHTMLTags(text)
	// Clean up whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func (e *S3ResumeExtractor) stripHTMLTags(content string) string {
	regex := regexp.MustCompile(`<[^>]*>`)
	return regex.ReplaceAllString(content, "")
}

func (e *S3ResumeExtractor) parseDateRange(dateStr string) (start, end string, current bool) {
	// Parse date ranges like "Jan 2020 - Present", "2019 - 2022", etc.
	dateStr = strings.ToLower(dateStr)
	
	if strings.Contains(dateStr, "present") || strings.Contains(dateStr, "current") {
		current = true
	}

	// Extract years
	yearRegex := regexp.MustCompile(`(20\d{2}|19\d{2})`)
	years := yearRegex.FindAllString(dateStr, -1)
	
	if len(years) >= 1 {
		start = years[0]
	}
	if len(years) >= 2 && !current {
		end = years[1]
	}

	return start, end, current
}

func (e *S3ResumeExtractor) calculateDuration(start, end string, current bool) string {
	if start == "" {
		return ""
	}

	startYear, _ := strconv.Atoi(start)
	var endYear int
	
	if current {
		endYear = time.Now().Year()
	} else if end != "" {
		endYear, _ = strconv.Atoi(end)
	} else {
		return ""
	}

	duration := endYear - startYear
	if duration <= 0 {
		return "Less than 1 year"
	} else if duration == 1 {
		return "1 year"
	} else {
		return fmt.Sprintf("%d years", duration)
	}
}

func (e *S3ResumeExtractor) extractLocationFromJobEntry(section, jobEntry string) string {
	// Simple location extraction - look for patterns like "City, State"
	locationRegex := regexp.MustCompile(`([A-Z][a-z]+,\s*[A-Z]{2})`)
	if match := locationRegex.FindString(jobEntry); match != "" {
		return match
	}
	return ""
}

func (e *S3ResumeExtractor) extractJobDescription(section, jobEntry string) []string {
	// Extract bullet points or paragraphs following the job entry
	// This is a simplified implementation
	var descriptions []string
	
	// Look for list items or paragraphs
	descRegex := regexp.MustCompile(`<li[^>]*>([^<]+)</li>`)
	matches := descRegex.FindAllStringSubmatch(section, -1)
	
	for _, match := range matches {
		if len(match) > 1 {
			desc := e.cleanText(match[1])
			if len(desc) > 10 { // Filter out very short descriptions
				descriptions = append(descriptions, desc)
			}
		}
	}

	return descriptions
}