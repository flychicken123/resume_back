package parsers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ResumeData represents the structured resume information
type ResumeData struct {
	Name       string              `json:"name"`
	Email      string              `json:"email"`
	Phone      string              `json:"phone"`
	Summary    string              `json:"summary"`
	Experience []ExperienceEntry   `json:"experience"`
	Education  []EducationEntry    `json:"education"`
	Projects   []ProjectEntry      `json:"projects"`
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

// ProjectEntry represents a project entry
type ProjectEntry struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	URL          string   `json:"url"`
	Bullets      []string `json:"bullets"`
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

	fmt.Printf("[ResumeParser] Starting parse of %d characters\n", len(rawText))
	
	resume := &ResumeData{
		RawText:  rawText,
		Sections: make(map[string]string),
	}

	// Extract basic contact information
	p.extractContactInfo(resume, rawText)
	fmt.Printf("[ResumeParser] Contact: Name=%s, Email=%s, Phone=%s\n", resume.Name, resume.Email, resume.Phone)

	// Split text into sections
	sections := p.extractSections(rawText)
	resume.Sections = sections
	fmt.Printf("[ResumeParser] Found %d sections\n", len(sections))
	for key := range sections {
		fmt.Printf("  - Section: %s\n", key)
	}

	// Extract structured data from sections
	p.extractExperience(resume, sections)
	fmt.Printf("[ResumeParser] Extracted %d experiences\n", len(resume.Experience))
	for i, exp := range resume.Experience {
		fmt.Printf("  Exp %d: Role='%s', Company='%s', Location='%s', Dates='%s-%s'\n", 
			i+1, exp.Role, exp.Company, exp.Location, exp.StartDate, exp.EndDate)
	}
	
	p.extractEducation(resume, sections)
	p.extractProjects(resume, sections)
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

	// Common section headers - order matters for overlapping keywords
	sectionHeaders := map[string][]string{
		"experience": {"working experience", "work experience", "professional experience", "employment history", "career history", "experience"},
		"education":  {"education", "academic background", "qualifications", "degrees"},
		"skills":     {"technical skills", "core skills", "skills & expertise", "skills", "competencies"},
		"summary":    {"professional summary", "career summary", "summary", "profile", "objective", "about"},
		"projects":   {"projects", "portfolio", "personal projects"},
		"awards":     {"awards", "honors", "achievements", "recognition"},
	}

	currentSection := ""
	currentContent := []string{}

	for i, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)
		
		// Keep empty lines within sections to preserve structure
		if line == "" && currentSection != "" {
			currentContent = append(currentContent, "")
			continue
		} else if line == "" {
			continue
		}

		// Check if this line is a section header
		isHeader := false
		// Normalize the line for comparison (remove extra spaces)
		lineLower := strings.ToLower(strings.Join(strings.Fields(line), " "))
		
		// Skip lines that are too long to be headers (likely content)
		if len(line) > 50 {
			if currentSection != "" {
				currentContent = append(currentContent, originalLine)
			}
			continue
		}
		
		// Check each section type
		for sectionKey, headers := range sectionHeaders {
			for _, header := range headers {
				// Normalize the header too
				headerNorm := strings.ToLower(strings.Join(strings.Fields(header), " "))
				
				// More flexible matching for section headers
				// Must be a standalone header or very short line
				// Avoid false positives like "Tech Stack" matching "skills"
				if (lineLower == headerNorm || 
				    lineLower == headerNorm + ":" || 
				    lineLower == headerNorm + " :" ||
				    strings.HasPrefix(lineLower, "• " + headerNorm) ||
				    strings.HasPrefix(lineLower, "- " + headerNorm) ||
				    strings.HasPrefix(lineLower, headerNorm + " ") && len(lineLower) < len(headerNorm) + 10) &&
				   !strings.Contains(lineLower, "tech stack") &&
				   !strings.Contains(lineLower, "skill set") &&
				   !strings.Contains(lineLower, "years of experience") {
					
					// Save previous section
					if currentSection != "" && len(currentContent) > 0 {
						sections[currentSection] = strings.Join(currentContent, "\n")
						fmt.Printf("[Section] Saved %s section with %d lines\n", currentSection, len(currentContent))
					}
					
					currentSection = sectionKey
					currentContent = []string{}
					isHeader = true
					fmt.Printf("[Section] Found %s header at line %d: %s\n", sectionKey, i, line)
					break
				}
			}
			if isHeader {
				break
			}
		}

		// Add line to current section if not a header
		if !isHeader && currentSection != "" {
			currentContent = append(currentContent, originalLine)
		}
		// Note: Lines before the first section (when !isHeader && currentSection == "")
		// are typically contact info, which is already extracted by extractContactInfo
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
	
	fmt.Printf("[ExtractExperience] Processing %d lines from experience section\n", len(lines))
	
	// Debug: show all lines we're processing
	if len(lines) < 100 {
		fmt.Println("[ExtractExperience] All lines in section:")
		for idx, l := range lines {
			fmt.Printf("  %3d: %s\n", idx, l)
		}
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		
		// Clean up PDF artifacts
		line = strings.ReplaceAll(line, "| Company", "")
		line = strings.ReplaceAll(line, "|Company", "")
		line = strings.TrimSpace(line)
		
		if line == "" || line == "Company" {
			continue
		}

		// Debug output
		fmt.Printf("  Line %d: %s\n", i, line)
		
		// Skip work type indicators early
		lineLower := strings.ToLower(strings.TrimSpace(line))
		if lineLower == "remote" || lineLower == "hybrid" || lineLower == "on-site" || 
		   lineLower == "onsite" || lineLower == "contract" || lineLower == "freelance" {
			// This is a work location/type, not a company
			if currentExp != nil && currentExp.Location == "" {
				currentExp.Location = line
				fmt.Printf("    -> Work type/location: %s\n", line)
			}
			continue
		}
		
		// Skip if this is clearly a date line (not a company)
		if p.dateRegex.MatchString(line) && strings.Contains(line, "-") && len(strings.Fields(line)) <= 3 {
			// This is just a date range, not a company
			if currentExp != nil {
				fmt.Printf("    -> Detected as DATE line\n")
				p.extractDatesFromLine(currentExp, line)
			}
			continue
		}
		
		// Check context to determine if this is likely a company name
		// Look at surrounding lines for clues
		isLikelyCompany := false
		
		// First check: does it look like a company based on patterns?
		if p.looksLikeCompany(line) {
			isLikelyCompany = true
		} else if len(line) < 50 && !strings.HasPrefix(line, "•") && !strings.HasPrefix(line, "-") && 
		         !p.dateRegex.MatchString(line) {  // Make sure it's not a date
			// Check if next non-empty line looks like a role
			for j := i+1; j < len(lines) && j < i+3; j++ {
				nextLine := strings.TrimSpace(lines[j])
				if nextLine != "" {
					if p.looksLikeRole(nextLine) {
						isLikelyCompany = true
						fmt.Printf("    -> Could be company (next line is role)\n")
					}
					break
				}
			}
			
			// Third check: is this a new company after a complete experience?
			if !isLikelyCompany && len(strings.Fields(line)) <= 4 && len(line) > 0 && unicode.IsUpper(rune(line[0])) {
				// If we have a previous complete experience, this might be a new company
				if currentExp != nil && currentExp.Role != "" && len(currentExp.Bullets) > 0 {
					isLikelyCompany = true
					fmt.Printf("    -> Could be company (after complete experience)\n")
				} else if currentExp == nil && !p.looksLikeRole(line) {
					// Or if we don't have any current experience yet (first company)
					isLikelyCompany = true
					fmt.Printf("    -> Could be company (first in list)\n")
				}
			}
		}
		
		// Check if this is "Role at Company" format first
		if strings.Contains(line, " at ") && (p.looksLikeRole(line) || strings.Contains(strings.ToLower(line), "developer") || 
		   strings.Contains(strings.ToLower(line), "engineer")) {
			fmt.Printf("    -> Detected as ROLE at COMPANY format\n")
			// Save previous experience if it has content
			if currentExp != nil && (currentExp.Role != "" || len(currentExp.Bullets) > 0) {
				experiences = append(experiences, *currentExp)
			}
			
			// Parse "Role at Company"
			parts := strings.SplitN(line, " at ", 2)
			currentExp = &ExperienceEntry{
				Role: strings.TrimSpace(parts[0]),
			}
			if len(parts) > 1 {
				currentExp.Company = strings.TrimSpace(parts[1])
			}
			fmt.Printf("    -> Extracted Role: %s, Company: %s\n", currentExp.Role, currentExp.Company)
			continue // Skip to next line
		} else if isLikelyCompany {
			fmt.Printf("    -> Detected as COMPANY\n")
			// Save previous experience if it has content
			if currentExp != nil && (currentExp.Role != "" || len(currentExp.Bullets) > 0) {
				experiences = append(experiences, *currentExp)
			}
			
			// Start new experience with company
			currentExp = &ExperienceEntry{
				Company: line,
			}
			
			// Look ahead for role (usually next non-empty line)
			foundRole := false
			for j := i + 1; j < len(lines) && j < i + 5; j++ {
				nextLine := strings.TrimSpace(lines[j])
				nextLine = strings.ReplaceAll(nextLine, "| Company", "")
				nextLine = strings.ReplaceAll(nextLine, "|Company", "")
				nextLine = strings.TrimSpace(nextLine)
				
				if nextLine == "" || nextLine == "Company" {
					continue
				}
				
				// Check if this is a role or contains role keywords
				if p.looksLikeRole(nextLine) || strings.Contains(nextLine, ",") {
					fmt.Printf("    -> Next line is ROLE: %s\n", nextLine)
					currentExp.Role = nextLine
					// Extract dates from role line
					p.extractDatesFromLine(currentExp, nextLine)
					// Clean role after extracting dates
					currentExp.Role = p.cleanRoleText(currentExp.Role)
					i = j // Skip the role line
					foundRole = true
					break
				} else if p.dateRegex.MatchString(nextLine) && strings.Contains(nextLine, "-") {
					// This might be a date line, look for role before it
					continue
				} else {
					// Not a role, stop looking
					break
				}
			}
			
			// If we didn't find a role but this is a known company, keep it
			if !foundRole && currentExp != nil && currentExp.Company != "" {
				fmt.Printf("    -> Company without immediate role found\n")
			}
		} else if p.looksLikeRole(line) && (currentExp == nil || currentExp.Role == "") {
			fmt.Printf("    -> Detected as ROLE\n")
			// This might be a role without company or role for current company
			if currentExp == nil {
				currentExp = &ExperienceEntry{}
			}
			
			// Check if this is "Role at Company" format
			if strings.Contains(line, " at ") {
				parts := strings.SplitN(line, " at ", 2)
				currentExp.Role = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					currentExp.Company = strings.TrimSpace(parts[1])
					fmt.Printf("    -> Extracted Role: %s, Company: %s\n", currentExp.Role, currentExp.Company)
				}
			} else {
				currentExp.Role = line
			}
			
			p.extractDatesFromLine(currentExp, line)
			currentExp.Role = p.cleanRoleText(currentExp.Role)
		} else if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
			// This is a bullet point
			if currentExp != nil {
				bullet := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "•"), "-"), "*")
				bullet = strings.TrimSpace(bullet)
				if bullet != "" {
					fmt.Printf("    -> Added as BULLET\n")
					currentExp.Bullets = append(currentExp.Bullets, bullet)
				}
			}
		} else if currentExp != nil {
			// Check if this line has dates (might be a standalone date line)
			if p.dateRegex.MatchString(line) && strings.Contains(line, "-") {
				fmt.Printf("    -> Detected as DATE line\n")
				p.extractDatesFromLine(currentExp, line)
			} else if strings.ToLower(line) == "remote" || strings.ToLower(line) == "hybrid" || 
					  strings.Contains(strings.ToLower(line), "on-site") || strings.Contains(strings.ToLower(line), "onsite") {
				// This is a work location/type indicator, add to location
				fmt.Printf("    -> Detected as LOCATION/TYPE\n")
				if currentExp.Location == "" {
					currentExp.Location = line
				}
			} else {
				// This is probably a bullet point without marker or continuation
				if len(line) > 20 { // Skip very short lines that might be fragments
					fmt.Printf("    -> Added as BULLET (no marker)\n")
					currentExp.Bullets = append(currentExp.Bullets, line)
				} else if len(line) > 0 && !p.dateRegex.MatchString(line) {
					// Short line that's not a date - might be a continuation
					fmt.Printf("    -> Short line, might be continuation: %s\n", line)
				}
			}
		} else {
			// No current experience - this might be the start of a new one
			// Check if it's not just a fragment
			if len(line) > 10 && !p.dateRegex.MatchString(line) {
				fmt.Printf("    -> Orphan line (no current exp): %s\n", line)
			}
		}
	}

	// Save last experience
	if currentExp != nil && (currentExp.Role != "" || currentExp.Company != "" || len(currentExp.Bullets) > 0) {
		experiences = append(experiences, *currentExp)
	}

	// Post-process: Try to merge experiences and fix date associations
	var mergedExperiences []ExperienceEntry
	for i := 0; i < len(experiences); i++ {
		exp := experiences[i]
		
		// If this entry has only a company and the next has only a role, merge them
		if exp.Company != "" && exp.Role == "" && i+1 < len(experiences) {
			nextExp := experiences[i+1]
			if nextExp.Role != "" && nextExp.Company == "" {
				// Merge
				exp.Role = nextExp.Role
				exp.StartDate = nextExp.StartDate
				exp.EndDate = nextExp.EndDate
				exp.Location = nextExp.Location
				exp.Bullets = append(exp.Bullets, nextExp.Bullets...)
				mergedExperiences = append(mergedExperiences, exp)
				i++ // Skip the next entry since we merged it
				continue
			}
		}
		
		// If this experience has no dates but the next one only has dates, merge them
		if exp.StartDate == "" && exp.EndDate == "" && i+1 < len(experiences) {
			nextExp := experiences[i+1]
			if nextExp.StartDate != "" || nextExp.EndDate != "" {
				if nextExp.Company == "" && nextExp.Role == "" && len(nextExp.Bullets) == 0 {
					// Next entry is just dates, merge them
					exp.StartDate = nextExp.StartDate
					exp.EndDate = nextExp.EndDate
					if nextExp.Location != "" {
						exp.Location = nextExp.Location
					}
					mergedExperiences = append(mergedExperiences, exp)
					i++ // Skip the next entry
					continue
				}
			}
		}
		
		// Don't add entries that are just location indicators or dates
		if (exp.Company == "Remote" || p.dateRegex.MatchString(exp.Company)) && 
		   exp.Role == "" && len(exp.Bullets) == 0 {
			// This is just a location indicator, skip it
			// Transfer dates to previous experience if available
			if len(mergedExperiences) > 0 && (exp.StartDate != "" || exp.EndDate != "") {
				lastExp := &mergedExperiences[len(mergedExperiences)-1]
				if lastExp.StartDate == "" {
					lastExp.StartDate = exp.StartDate
				}
				if lastExp.EndDate == "" {
					lastExp.EndDate = exp.EndDate
				}
			}
			continue
		}
		
		// Add as-is if no merge needed
		mergedExperiences = append(mergedExperiences, exp)
	}

	resume.Experience = mergedExperiences
	
	fmt.Printf("[ExtractExperience] Final count: %d experiences\n", len(resume.Experience))
}

// looksLikeCompany checks if a line looks like a company name based on patterns and context
func (p *ResumeParser) looksLikeCompany(line string) bool {
	// Skip if too long (probably a description)
	if len(line) > 100 {
		return false
	}
	
	// Skip if it's a bullet point or description
	if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") {
		return false
	}
	
	// Skip common location/work type indicators
	lineLower := strings.ToLower(line)
	skipWords := []string{"remote", "hybrid", "on-site", "onsite", "contract", "freelance", "part-time", "full-time", "internship", "temporary"}
	for _, word := range skipWords {
		if lineLower == word {
			return false
		}
	}
	
	// Company suffix patterns (Inc, LLC, etc.)
	companyPatterns := []string{
		`\bInc\b\.?`, `\bLLC\b`, `\bLtd\b\.?`, `\bCorp\b\.?`, 
		`\bCorporation\b`, `\bCompany\b`, `\bCo\b\.`, 
		`\bTechnologies\b`, `\bSolutions\b`, `\bServices\b`, 
		`\bGroup\b`, `\bPartners\b`, `\bLabs?\b`, `\bStudios?\b`,
		`\bEnterprise\b`, `\bSystems\b`, `\bNetworks?\b`,
	}
	
	for _, pattern := range companyPatterns {
		if regexp.MustCompile(`(?i)`+pattern).MatchString(line) {
			return true
		}
	}
	
	// Don't treat single common words as companies
	if len(strings.Fields(line)) == 1 && len(line) < 15 {
		return false
	}
	
	return false
}

// looksLikeRole checks if a line looks like a job role/title
func (p *ResumeParser) looksLikeRole(line string) bool {
	// Role keywords
	roleKeywords := []string{
		"Engineer", "Developer", "Manager", "Lead", "Architect",
		"Analyst", "Designer", "Consultant", "Director", "Specialist",
		"Coordinator", "Administrator", "Officer", "Executive", "Associate",
		"Senior", "Junior", "Staff", "Principal", "Tech", "Software", "Data",
		"Technical", "Product", "Project", "Program", "Systems", "Solutions",
		"Infrastructure", "DevOps", "Backend", "Frontend", "Full Stack",
	}
	
	for _, keyword := range roleKeywords {
		if strings.Contains(line, keyword) {
			return true
		}
	}
	
	// Check if line ends with a comma and date (common pattern: "Software Engineer, May 2022 - Nov 2022")
	if strings.Contains(line, ",") && p.dateRegex.MatchString(line) {
		return true
	}
	
	return false
}

// extractDatesFromLine extracts dates from a line and updates the experience entry
func (p *ResumeParser) extractDatesFromLine(exp *ExperienceEntry, line string) {
	dates := p.dateRegex.FindAllString(line, -1)
	if len(dates) >= 2 {
		exp.StartDate = dates[0]
		exp.EndDate = dates[len(dates)-1]
	} else if len(dates) == 1 {
		if strings.Contains(strings.ToLower(line), "present") ||
		   strings.Contains(strings.ToLower(line), "current") {
			exp.EndDate = "Present"
			exp.StartDate = dates[0]
		} else {
			exp.StartDate = dates[0]
		}
	}
}

// cleanRoleText removes dates and cleans up role text
func (p *ResumeParser) cleanRoleText(role string) string {
	// Remove dates
	dates := p.dateRegex.FindAllString(role, -1)
	for _, date := range dates {
		role = strings.Replace(role, date, "", 1)
	}
	
	// Clean up
	role = strings.TrimSpace(role)
	role = strings.TrimSuffix(role, ",")
	role = strings.TrimSuffix(role, "-")
	role = strings.TrimSpace(role)
	role = regexp.MustCompile(`\s+`).ReplaceAllString(role, " ")
	
	return role
}

// extractEducation parses education information
func (p *ResumeParser) extractEducation(resume *ResumeData, sections map[string]string) {
	eduText, exists := sections["education"]
	if !exists {
		return
	}

	fmt.Printf("[ExtractEducation] Processing education section\n")
	lines := strings.Split(eduText, "\n")
	var education []EducationEntry
	var currentEdu *EducationEntry

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fmt.Printf("  Edu Line %d: %s\n", i, line)

		// Check for degree patterns - expanded list
		degreeKeywords := []string{"bachelor", "master", "phd", "doctorate", "associate", 
			"b.s.", "b.a.", "m.s.", "m.a.", "mba", "degree", "diploma", "b.s", "b.a", "m.s", "m.a"}
		hasDegree := false
		for _, keyword := range degreeKeywords {
			if strings.Contains(strings.ToLower(line), keyword) {
				hasDegree = true
				break
			}
		}

		// Check for university patterns - expanded list
		universityKeywords := []string{"university", "college", "institute", "school", "academy", "tech"}
		hasUniversity := false
		for _, keyword := range universityKeywords {
			if strings.Contains(strings.ToLower(line), keyword) {
				hasUniversity = true
				break
			}
		}
		
		// Check for field of study keywords
		fieldKeywords := []string{"engineering", "science", "business", "arts", "medicine", 
			"law", "computer", "electrical", "mechanical", "chemical", "civil", "software",
			"mathematics", "math", "physics", "chemistry", "biology", "economics", "psychology",
			"electronic", "electronics"}
		hasField := false
		for _, keyword := range fieldKeywords {
			if strings.Contains(strings.ToLower(line), keyword) {
				hasField = true
				break
			}
		}

		// Start new education entry if we see school+dates or degree line
		if (hasUniversity && p.dateRegex.MatchString(line)) || 
		   (hasDegree && (currentEdu == nil || currentEdu.Degree == "")) {
			// Save previous education
			if currentEdu != nil && (currentEdu.School != "" || currentEdu.Degree != "") {
				education = append(education, *currentEdu)
				fmt.Printf("    -> Saved education: %s, %s, %s\n", currentEdu.School, currentEdu.Degree, currentEdu.Field)
			}

			currentEdu = &EducationEntry{}
			fmt.Printf("    -> Starting new education entry\n")
			
			// Parse the entire line for all components
			p.parseEducationLine(currentEdu, line)
		} else if currentEdu != nil {
			// Add to current education entry
			if hasDegree && currentEdu.Degree == "" {
				// Parse this line for degree info
				p.parseEducationLine(currentEdu, line)
				fmt.Printf("    -> Parsed as DEGREE line\n")
			} else if hasUniversity && currentEdu.School == "" {
				// Extract school name, removing dates
				dates := p.dateRegex.FindAllString(line, -1)
				schoolLine := line
				for _, date := range dates {
					schoolLine = strings.Replace(schoolLine, date, "", 1)
				}
				currentEdu.School = strings.TrimSpace(schoolLine)
				// Also capture dates
				if len(dates) >= 2 {
					currentEdu.StartDate = dates[0]
					currentEdu.EndDate = dates[len(dates)-1]
				} else if len(dates) == 1 {
					currentEdu.EndDate = dates[0]
				}
				fmt.Printf("    -> Set as SCHOOL: %s\n", currentEdu.School)
			} else if (hasField || strings.Contains(line, "Minor")) && currentEdu.Field == "" {
				// This is the field of study
				// Check if this line also contains degree info (like "B.S in Electronic Engineering")
				if strings.Contains(strings.ToLower(line), " in ") {
					p.parseEducationLine(currentEdu, line)
					fmt.Printf("    -> Parsed as DEGREE with FIELD\n")
				} else {
					currentEdu.Field = strings.TrimSpace(line)
					fmt.Printf("    -> Set as FIELD: %s\n", currentEdu.Field)
				}
			} else if currentEdu.School == "" && !p.dateRegex.MatchString(line) {
				// Fallback: might be school (but not if it's just dates)
				currentEdu.School = strings.TrimSpace(line)
				fmt.Printf("    -> Set as SCHOOL (fallback)\n")
			} else if currentEdu.Degree == "" && !p.dateRegex.MatchString(line) {
				// Fallback: might be degree (but not if it's just dates)
				currentEdu.Degree = strings.TrimSpace(line)
				fmt.Printf("    -> Set as DEGREE (fallback)\n")
			} else if currentEdu.Field == "" && len(line) < 100 && !p.dateRegex.MatchString(line) {
				// Fallback: might be field (but not if it's just dates)
				currentEdu.Field = strings.TrimSpace(line)
				fmt.Printf("    -> Set as FIELD (fallback)\n")
			} else if p.dateRegex.MatchString(line) && currentEdu.StartDate == "" && currentEdu.EndDate == "" {
				// This line contains dates - extract them but don't overwrite other fields
				dates := p.dateRegex.FindAllString(line, -1)
				if len(dates) >= 2 {
					currentEdu.StartDate = dates[0]
					currentEdu.EndDate = dates[len(dates)-1]
					fmt.Printf("    -> Extracted dates: %s - %s\n", currentEdu.StartDate, currentEdu.EndDate)
				} else if len(dates) == 1 {
					currentEdu.EndDate = dates[0]
					fmt.Printf("    -> Extracted end date: %s\n", currentEdu.EndDate)
				}
			}
		}
	}

	// Save last education
	if currentEdu != nil && (currentEdu.School != "" || currentEdu.Degree != "") {
		education = append(education, *currentEdu)
		fmt.Printf("    -> Saved final education: %s, %s\n", currentEdu.Degree, currentEdu.School)
	}

	resume.Education = education
	fmt.Printf("[ExtractEducation] Extracted %d education entries\n", len(education))
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

	// Look for common degree patterns
	degreePatterns := []string{
		"B.S.", "B.A.", "BS", "BA", "Bachelor", "Bachelors",
		"M.S.", "M.A.", "MS", "MA", "Master", "Masters",
		"Ph.D.", "PhD", "Ph.D", "Doctorate",
		"M.B.A.", "MBA",
		"Associate", "A.S.", "A.A.", "AS", "AA",
	}
	
	lowerLine := strings.ToLower(line)
	
	// Check if line contains "in" which often separates degree from field
	if strings.Contains(lowerLine, " in ") {
		parts := strings.Split(line, " in ")
		if len(parts) >= 2 {
			edu.Degree = strings.TrimSpace(parts[0])
			// Everything after "in" is the field
			remainingParts := strings.Join(parts[1:], " in ")
			
			// Check if school name is also in this line (often after comma)
			if strings.Contains(remainingParts, ",") {
				fieldParts := strings.Split(remainingParts, ",")
				edu.Field = strings.TrimSpace(fieldParts[0])
				if len(fieldParts) > 1 && edu.School == "" {
					edu.School = strings.TrimSpace(fieldParts[1])
				}
			} else {
				edu.Field = strings.TrimSpace(remainingParts)
			}
			return
		}
	}
	
	// Check for pattern: "Bachelor of Science, Computer Science"
	if strings.Contains(line, ",") {
		parts := strings.Split(line, ",")
		firstPart := strings.TrimSpace(parts[0])
		
		// Check if first part is a degree
		hasDegree := false
		for _, pattern := range degreePatterns {
			if strings.Contains(strings.ToLower(firstPart), strings.ToLower(pattern)) {
				hasDegree = true
				break
			}
		}
		
		if hasDegree {
			edu.Degree = firstPart
			if len(parts) > 1 {
				// Check if second part is field or school
				secondPart := strings.TrimSpace(parts[1])
				// If it looks like a field (shorter, no "University" etc), use as field
				if !strings.Contains(strings.ToLower(secondPart), "university") && 
				   !strings.Contains(strings.ToLower(secondPart), "college") &&
				   !strings.Contains(strings.ToLower(secondPart), "institute") &&
				   len(secondPart) < 50 {
					edu.Field = secondPart
					if len(parts) > 2 {
						edu.School = strings.TrimSpace(parts[2])
					}
				} else {
					edu.School = secondPart
				}
			}
		} else {
			// First part might be the school
			if edu.School == "" {
				edu.School = firstPart
			}
			if len(parts) > 1 {
				edu.Degree = strings.TrimSpace(parts[1])
			}
		}
	} else {
		// No comma, try to extract what we can
		for _, pattern := range degreePatterns {
			if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
				edu.Degree = line
				return
			}
		}
		
		// If no degree pattern found, assume it's the school
		if edu.School == "" {
			edu.School = line
		}
	}
}

// extractProjects parses projects from sections
func (p *ResumeParser) extractProjects(resume *ResumeData, sections map[string]string) {
	// Try different section names for projects
	projectSectionNames := []string{"projects", "project", "academic projects", "personal projects", 
		"technical projects", "key projects", "relevant projects", "portfolio"}
	
	var projectText string
	for _, sectionName := range projectSectionNames {
		if text, exists := sections[sectionName]; exists {
			projectText = text
			break
		}
	}
	
	if projectText == "" {
		return
	}
	
	lines := strings.Split(projectText, "\n")
	var currentProject *ProjectEntry
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Check if this looks like a project header
		if p.looksLikeProjectHeader(line) {
			// Save previous project if exists
			if currentProject != nil && currentProject.Name != "" {
				resume.Projects = append(resume.Projects, *currentProject)
			}
			
			// Start new project
			currentProject = &ProjectEntry{}
			p.parseProjectHeader(currentProject, line)
		} else if currentProject != nil {
			// Check for technologies line
			if strings.HasPrefix(strings.ToLower(line), "technologies:") || 
			   strings.HasPrefix(strings.ToLower(line), "tech:") ||
			   strings.HasPrefix(strings.ToLower(line), "tech stack:") ||
			   strings.HasPrefix(strings.ToLower(line), "built with:") {
				techLine := line[strings.Index(line, ":")+1:]
				currentProject.Technologies = p.parseTechnologies(techLine)
			} else if strings.HasPrefix(strings.ToLower(line), "link:") || 
			          strings.HasPrefix(strings.ToLower(line), "url:") ||
			          strings.HasPrefix(strings.ToLower(line), "github:") {
				currentProject.URL = strings.TrimSpace(line[strings.Index(line, ":")+1:])
			} else if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") || 
			          strings.HasPrefix(line, "*") || strings.HasPrefix(line, "▪") {
				// Bullet point
				bullet := strings.TrimLeft(line, "•-*▪ ")
				currentProject.Bullets = append(currentProject.Bullets, bullet)
			} else if currentProject.Description == "" {
				// First non-bullet line becomes description
				currentProject.Description = line
			} else {
				// Additional lines are added as bullets
				currentProject.Bullets = append(currentProject.Bullets, line)
			}
		}
	}
	
	// Add last project if exists
	if currentProject != nil && currentProject.Name != "" {
		resume.Projects = append(resume.Projects, *currentProject)
	}
}

// looksLikeProjectHeader checks if a line appears to be a project header
func (p *ResumeParser) looksLikeProjectHeader(line string) bool {
	// Project headers often have:
	// - Title case or all caps
	// - May include dates in parentheses
	// - May include a separator like | or -
	// - Usually doesn't start with a bullet point
	
	if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") || 
	   strings.HasPrefix(line, "*") || strings.HasPrefix(line, "▪") {
		return false
	}
	
	// Check if it contains common project keywords followed by a name
	lowerLine := strings.ToLower(line)
	projectIndicators := []string{
		"project:", "title:", "name:",
	}
	
	for _, indicator := range projectIndicators {
		if strings.HasPrefix(lowerLine, indicator) {
			return true
		}
	}
	
	// Check if line has dates (common in project headers)
	if p.dateRegex.MatchString(line) {
		return true
	}
	
	// Check if it's in title case or all caps (common for project names)
	words := strings.Fields(line)
	if len(words) > 0 && len(words) <= 6 { // Project names are usually short
		titleCase := true
		for _, word := range words {
			if len(word) > 3 && word[0] >= 'a' && word[0] <= 'z' {
				titleCase = false
				break
			}
		}
		if titleCase {
			return true
		}
	}
	
	return false
}

// parseProjectHeader extracts project name and dates from header line
func (p *ResumeParser) parseProjectHeader(proj *ProjectEntry, line string) {
	// Remove common prefixes
	line = strings.TrimPrefix(strings.ToLower(line), "project:")
	line = strings.TrimPrefix(line, "title:")
	line = strings.TrimPrefix(line, "name:")
	line = strings.TrimSpace(line)
	
	// Check for dates and remove them from the line (we no longer store dates for projects)
	dates := p.dateRegex.FindAllString(line, -1)
	// Remove dates from line to get clean project name
	for _, date := range dates {
		line = strings.ReplaceAll(line, date, "")
	}
	
	// Clean up separators and extra spaces
	line = strings.ReplaceAll(line, "|", "")
	line = strings.ReplaceAll(line, "-", " ")
	line = strings.ReplaceAll(line, "()", "")
	line = strings.ReplaceAll(line, "[]", "")
	line = strings.TrimSpace(line)
	
	// What remains is the project name
	proj.Name = line
}

// parseTechnologies extracts technologies from a comma or pipe separated string
func (p *ResumeParser) parseTechnologies(techString string) []string {
	var technologies []string
	
	// Replace common separators with comma
	techString = strings.ReplaceAll(techString, "|", ",")
	techString = strings.ReplaceAll(techString, ";", ",")
	techString = strings.ReplaceAll(techString, " and ", ",")
	techString = strings.ReplaceAll(techString, " & ", ",")
	
	// Split by comma and clean each technology
	techs := strings.Split(techString, ",")
	for _, tech := range techs {
		tech = strings.TrimSpace(tech)
		if tech != "" {
			technologies = append(technologies, tech)
		}
	}
	
	return technologies
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