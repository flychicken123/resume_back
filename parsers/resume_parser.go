package parsers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ResumeData represents the structured resume information
type ResumeData struct {
	Name       string              `json:"name"`
	Email      string              `json:"email"`
	Phone      string              `json:"phone"`
	Summary    string              `json:"summary"`
	Experience []ExperienceEntry   `json:"experience"`
	Education  []EducationEntry    `json:"education"`
	Skills     []string            `json:"skills"`
	RawText    string              `json:"raw_text,omitempty"`
	Sections   map[string]string   `json:"sections,omitempty"`
}

// ExperienceEntry represents a work experience entry
type ExperienceEntry struct {
	Company   string   `json:"company"`
	Role      string   `json:"role"`
	Location  string   `json:"location"`
	StartDate string   `json:"startDate"`
	EndDate   string   `json:"endDate"`
	Bullets   []string `json:"bullets"`
}

// EducationEntry represents an education entry
type EducationEntry struct {
	School    string `json:"school"`
	Degree    string `json:"degree"`
	Field     string `json:"field"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// ResumeParser handles resume text parsing and data extraction
type ResumeParser struct {
	emailRegex *regexp.Regexp
	phoneRegex *regexp.Regexp
	dateRegex  *regexp.Regexp
	nameRegex  *regexp.Regexp
}

// NewResumeParser creates a new resume parser with compiled regexes
func NewResumeParser() *ResumeParser {
	return &ResumeParser{
		emailRegex: regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
		phoneRegex: regexp.MustCompile(`(\+?1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})`),
		dateRegex:  regexp.MustCompile(`(?i)(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*[\s,]*\d{4}|(\d{1,2}[\/\-]\d{1,2}[\/\-]\d{2,4})|(\d{4}[\s\-]\d{4})|present|current|now`),
		nameRegex:  regexp.MustCompile(`^[A-Z][a-zA-Z\s.'-]{1,50}$`),
	}
}

// Parse extracts structured data from resume text
func (p *ResumeParser) Parse(rawText string) (*ResumeData, error) {
	if strings.TrimSpace(rawText) == "" {
		return nil, fmt.Errorf("empty resume text")
	}

	resume := &ResumeData{
		RawText:  rawText,
		Sections: make(map[string]string),
	}

	// Extract basic contact information
	p.extractContactInfo(resume, rawText)

	// Split text into sections
	sections := p.extractSections(rawText)
	resume.Sections = sections

	// Extract structured data from sections
	p.extractExperience(resume, sections)
	p.extractEducation(resume, sections)
	p.extractSkills(resume, sections)
	p.extractSummary(resume, sections)

	return resume, nil
}

// extractContactInfo extracts name, email, and phone from the text
func (p *ResumeParser) extractContactInfo(resume *ResumeData, text string) {
	lines := strings.Split(text, "\n")

	// Extract email
	if email := p.emailRegex.FindString(text); email != "" {
		resume.Email = email
	}

	// Extract phone
	if phone := p.phoneRegex.FindString(text); phone != "" {
		resume.Phone = p.normalizePhone(phone)
	}

	// Extract name (usually in the first few lines)
	for i, line := range lines {
		if i > 5 { // Don't look beyond first 5 lines for name
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "@") || p.phoneRegex.MatchString(line) {
			continue
		}
		
		// Check if line looks like a name
		words := strings.Fields(line)
		if len(words) >= 2 && len(words) <= 4 {
			isName := true
			for _, word := range words {
				if len(word) < 2 || !regexp.MustCompile(`^[A-Za-z'-]+$`).MatchString(word) {
					isName = false
					break
				}
			}
			if isName {
				resume.Name = line
				break
			}
		}
	}
}

// extractSections splits the resume into logical sections
func (p *ResumeParser) extractSections(text string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(text, "\n")

	// Common section headers
	sectionHeaders := map[string][]string{
		"experience": {"experience", "work experience", "employment", "professional experience", "career history"},
		"education":  {"education", "academic background", "qualifications", "degrees"},
		"skills":     {"skills", "technical skills", "competencies", "expertise", "technologies"},
		"summary":    {"summary", "profile", "objective", "about", "professional summary", "career summary"},
		"projects":   {"projects", "portfolio", "personal projects"},
		"awards":     {"awards", "honors", "achievements", "recognition"},
	}

	currentSection := ""
	currentContent := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this line is a section header
		isHeader := false
		for sectionKey, headers := range sectionHeaders {
			for _, header := range headers {
				if strings.Contains(strings.ToLower(line), header) && len(line) < 50 {
					// Save previous section
					if currentSection != "" && len(currentContent) > 0 {
						sections[currentSection] = strings.Join(currentContent, "\n")
					}
					currentSection = sectionKey
					currentContent = []string{}
					isHeader = true
					break
				}
			}
			if isHeader {
				break
			}
		}

		if !isHeader && currentSection != "" {
			currentContent = append(currentContent, line)
		}
	}

	// Save last section
	if currentSection != "" && len(currentContent) > 0 {
		sections[currentSection] = strings.Join(currentContent, "\n")
	}

	return sections
}

// extractExperience parses work experience from sections
func (p *ResumeParser) extractExperience(resume *ResumeData, sections map[string]string) {
	expText, exists := sections["experience"]
	if !exists {
		return
	}

	lines := strings.Split(expText, "\n")
	var experiences []ExperienceEntry
	var currentExp *ExperienceEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this looks like a job title/company line
		if p.looksLikeJobHeader(line) {
			// Save previous experience
			if currentExp != nil {
				experiences = append(experiences, *currentExp)
			}

			// Parse new experience
			currentExp = &ExperienceEntry{}
			p.parseJobHeader(currentExp, line)
		} else if currentExp != nil {
			// This is likely a bullet point or description
			if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
				currentExp.Bullets = append(currentExp.Bullets, strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "•"), "-"), "*"))
			} else {
				currentExp.Bullets = append(currentExp.Bullets, line)
			}
		}
	}

	// Save last experience
	if currentExp != nil {
		experiences = append(experiences, *currentExp)
	}

	resume.Experience = experiences
}

// looksLikeJobHeader determines if a line looks like a job title/company
func (p *ResumeParser) looksLikeJobHeader(line string) bool {
	// Look for patterns like:
	// "Software Engineer at Google"
	// "Marketing Manager | Adobe"
	// "Data Scientist - Facebook, 2020-2022"
	
	keywords := []string{"at", "with", "|", "-", "•"}
	for _, keyword := range keywords {
		if strings.Contains(line, keyword) {
			return true
		}
	}

	// Check if line contains date patterns
	if p.dateRegex.MatchString(line) {
		return true
	}

	// If it's a short line (likely a title)
	if len(strings.Fields(line)) <= 6 && len(line) > 10 {
		return true
	}

	return false
}

// parseJobHeader extracts role, company, and dates from a job header line
func (p *ResumeParser) parseJobHeader(exp *ExperienceEntry, line string) {
	// Extract dates first
	dates := p.dateRegex.FindAllString(line, -1)
	if len(dates) >= 2 {
		exp.StartDate = dates[0]
		exp.EndDate = dates[len(dates)-1]
		// Remove dates from line for further parsing
		for _, date := range dates {
			line = strings.Replace(line, date, "", 1)
		}
	} else if len(dates) == 1 {
		if strings.Contains(strings.ToLower(dates[0]), "present") ||
		   strings.Contains(strings.ToLower(dates[0]), "current") ||
		   strings.Contains(strings.ToLower(dates[0]), "now") {
			exp.EndDate = "Present"
		} else {
			exp.StartDate = dates[0]
		}
	}

	// Clean up the line
	line = strings.TrimSpace(line)
	line = regexp.MustCompile(`\s+`).ReplaceAllString(line, " ")

	// Try to split role and company
	separators := []string{" at ", " with ", " | ", " - ", " • "}
	for _, sep := range separators {
		if strings.Contains(line, sep) {
			parts := strings.Split(line, sep)
			if len(parts) >= 2 {
				exp.Role = strings.TrimSpace(parts[0])
				exp.Company = strings.TrimSpace(parts[1])
				return
			}
		}
	}

	// If no separator found, assume the whole line is the role
	exp.Role = line
}

// extractEducation parses education information
func (p *ResumeParser) extractEducation(resume *ResumeData, sections map[string]string) {
	eduText, exists := sections["education"]
	if !exists {
		return
	}

	lines := strings.Split(eduText, "\n")
	var education []EducationEntry
	var currentEdu *EducationEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for degree patterns
		degreeKeywords := []string{"bachelor", "master", "phd", "doctorate", "associate", "b.s.", "b.a.", "m.s.", "m.a.", "mba"}
		hasDegree := false
		for _, keyword := range degreeKeywords {
			if strings.Contains(strings.ToLower(line), keyword) {
				hasDegree = true
				break
			}
		}

		if hasDegree || p.dateRegex.MatchString(line) {
			// Save previous education
			if currentEdu != nil {
				education = append(education, *currentEdu)
			}

			currentEdu = &EducationEntry{}
			p.parseEducationLine(currentEdu, line)
		} else if currentEdu != nil && currentEdu.School == "" {
			// This might be the school name
			currentEdu.School = line
		}
	}

	// Save last education
	if currentEdu != nil {
		education = append(education, *currentEdu)
	}

	resume.Education = education
}

// parseEducationLine extracts degree and school information
func (p *ResumeParser) parseEducationLine(edu *EducationEntry, line string) {
	// Extract dates
	dates := p.dateRegex.FindAllString(line, -1)
	if len(dates) >= 2 {
		edu.StartDate = dates[0]
		edu.EndDate = dates[len(dates)-1]
	} else if len(dates) == 1 {
		edu.EndDate = dates[0]
	}

	// Remove dates for further parsing
	for _, date := range dates {
		line = strings.Replace(line, date, "", 1)
	}

	line = strings.TrimSpace(line)

	// Try to identify degree and field
	parts := strings.Split(line, ",")
	if len(parts) >= 2 {
		edu.Degree = strings.TrimSpace(parts[0])
		edu.Field = strings.TrimSpace(parts[1])
	} else {
		edu.Degree = line
	}
}

// extractSkills parses skills from sections
func (p *ResumeParser) extractSkills(resume *ResumeData, sections map[string]string) {
	skillsText, exists := sections["skills"]
	if !exists {
		return
	}

	// Split by common delimiters
	skillsText = strings.ReplaceAll(skillsText, ",", "\n")
	skillsText = strings.ReplaceAll(skillsText, ";", "\n")
	skillsText = strings.ReplaceAll(skillsText, "|", "\n")

	lines := strings.Split(skillsText, "\n")
	var skills []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "•")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if line != "" && len(line) > 1 && len(line) < 50 {
			skills = append(skills, line)
		}
	}

	resume.Skills = skills
}

// extractSummary parses summary/objective section
func (p *ResumeParser) extractSummary(resume *ResumeData, sections map[string]string) {
	if summary, exists := sections["summary"]; exists {
		resume.Summary = strings.TrimSpace(summary)
	}
}

// normalizePhone standardizes phone number format
func (p *ResumeParser) normalizePhone(phone string) string {
	// Remove all non-digits
	digits := regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
	
	// Format as (XXX) XXX-XXXX if US number
	if len(digits) == 10 {
		return fmt.Sprintf("(%s) %s-%s", digits[0:3], digits[3:6], digits[6:10])
	} else if len(digits) == 11 && digits[0] == '1' {
		return fmt.Sprintf("+1 (%s) %s-%s", digits[1:4], digits[4:7], digits[7:11])
	}
	
	return phone // Return original if not standard format
}

// ToJSON converts the resume data to JSON
func (r *ResumeData) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}