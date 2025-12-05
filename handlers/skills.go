package handlers

import (
	"encoding/json"
	"net/http"
	"resumeai/utils"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// AutoSkillsRequest is the payload sent from the frontend to auto-generate skills.
// It contains the full in-progress resume data plus an optional job description
// for additional context.
type AutoSkillsRequest struct {
	ResumeData     map[string]interface{} `json:"resumeData" binding:"required"`
	JobDescription string                 `json:"jobDescription"`
}

// AutoSkillsResponse is returned to the frontend and includes both a structured
// list of skills and a comma-separated string suitable for binding directly to
// the Skills textbox.
type AutoSkillsResponse struct {
	Skills     []string `json:"skills"`
	SkillsText string   `json:"skillsText"`
	Message    string   `json:"message"`
}

// SkillCategory represents a logical grouping of skills such as "Cloud" or "Languages".
type SkillCategory struct {
	Name   string   `json:"name"`
	Skills []string `json:"skills"`
}

// AutoGenerateSkills uses the LangChain-backed copilot agent to read the user’s
// current resume data and extract a clean, de-duplicated list of skills.
func AutoGenerateSkills(c *gin.Context) {
	var req AutoSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	if len(req.ResumeData) == 0 {
		utils.BadRequestError(c, "resumeData is required to extract skills", nil)
		return
	}

	prompt := buildSkillsExtractionPrompt(req.ResumeData, req.JobDescription)

	raw, err := runCopilotPrompt(c.Request.Context(), prompt)
	if err != nil {
		utils.InternalServerError(c, "Failed to auto-generate skills from resume", err)
		return
	}

	skills, err := parseSkillsFromLLM(raw)
	if err != nil {
		utils.InternalServerError(c, "Failed to interpret AI skills response", err)
		return
	}

	// Normalize and de-duplicate skill strings.
	seen := make(map[string]struct{})
	var normalized []string
	for _, s := range skills {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		lowered := strings.ToLower(trimmed)
		if _, exists := seen[lowered]; exists {
			continue
		}
		seen[lowered] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	skillsText := strings.Join(normalized, ", ")

	response := AutoSkillsResponse{
		Skills:     normalized,
		SkillsText: skillsText,
		Message:    "Skills extracted successfully from resume data.",
	}

	utils.SuccessResponse(c, http.StatusOK, "Skills extracted successfully", response)
}

// CategorizeSkillsRequest is the payload sent from the frontend to bucket existing skills.
type CategorizeSkillsRequest struct {
	SkillsText     string `json:"skillsText" binding:"required"`
	JobDescription string `json:"jobDescription"`
}

// CategorizeSkillsResponse contains grouped categories and a flattened text representation.
type CategorizeSkillsResponse struct {
	Categories    []SkillCategory `json:"categories"`
	FormattedText string          `json:"formattedText"`
	Message       string          `json:"message"`
}

// CategorizeSkills uses the LangChain-backed copilot agent to group skills into buckets
// like "Cloud", "Languages", etc., and returns both structured data and a single-line
// string suitable for the Skills textbox.
func CategorizeSkills(c *gin.Context) {
	var req CategorizeSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, err)
		return
	}

	skillsText := strings.TrimSpace(req.SkillsText)
	if skillsText == "" {
		utils.BadRequestError(c, "skillsText is required for categorization", nil)
		return
	}

	prompt := buildSkillsCategorizationPrompt(skillsText, req.JobDescription)
	raw, err := runCopilotPrompt(c.Request.Context(), prompt)
	if err != nil {
		utils.InternalServerError(c, "Failed to categorize skills", err)
		return
	}

	categories, err := parseCategoriesFromLLM(raw)
	if err != nil {
		utils.InternalServerError(c, "Failed to interpret skill categories from AI", err)
		return
	}

	// Build a compact representation like:
	// "Cloud: Azure, AWS"
	// <blank line>
	// "Languages: Go, Python"
	var parts []string
	for _, cat := range categories {
		if cat.Name == "" || len(cat.Skills) == 0 {
			continue
		}
		var cleaned []string
		for _, s := range cat.Skills {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) == 0 {
			continue
		}
		parts = append(parts, cat.Name+": "+strings.Join(cleaned, ", "))
	}

	// Fallback: if the AI returned nothing useful, keep original text.
	// Use a blank line between records so the textarea is easier to read.
	formatted := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if formatted == "" {
		formatted = skillsText
	}

	response := CategorizeSkillsResponse{
		Categories:    categories,
		FormattedText: formatted,
		Message:       "Skills categorized successfully.",
	}

	utils.SuccessResponse(c, http.StatusOK, "Skills categorized successfully", response)
}

// buildSkillsExtractionPrompt converts the loosely-typed resumeData map into a
// readable text block and asks the LangChain model to return a JSON list of skills.
func buildSkillsExtractionPrompt(resumeData map[string]interface{}, jobDescription string) string {
	var parts []string

	// Basic identity fields (if present).
	if name, ok := resumeData["name"].(string); ok && strings.TrimSpace(name) != "" {
		parts = append(parts, "Name: "+strings.TrimSpace(name))
	}
	if email, ok := resumeData["email"].(string); ok && strings.TrimSpace(email) != "" {
		parts = append(parts, "Email: "+strings.TrimSpace(email))
	}
	if phone, ok := resumeData["phone"].(string); ok && strings.TrimSpace(phone) != "" {
		parts = append(parts, "Phone: "+strings.TrimSpace(phone))
	}

	if summary, ok := resumeData["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		parts = append(parts, "Summary:\n"+strings.TrimSpace(summary))
	}

	// Experiences: jobTitle, company, description, plus any explicit skills/tools fields.
	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		var sb strings.Builder
		sb.WriteString("Experience:\n")
		for i, exp := range experiences {
			expMap, ok := exp.(map[string]interface{})
			if !ok {
				continue
			}
			sb.WriteString("Experience ")
			sb.WriteString(strings.TrimSpace(toOrdinal(i + 1)))
			sb.WriteString(":\n")

			if title, ok := expMap["jobTitle"].(string); ok && strings.TrimSpace(title) != "" {
				sb.WriteString("  Job Title: ")
				sb.WriteString(strings.TrimSpace(title))
				sb.WriteString("\n")
			}
			if company, ok := expMap["company"].(string); ok && strings.TrimSpace(company) != "" {
				sb.WriteString("  Company: ")
				sb.WriteString(strings.TrimSpace(company))
				sb.WriteString("\n")
			}
			if desc, ok := expMap["description"].(string); ok && strings.TrimSpace(desc) != "" {
				sb.WriteString("  Description: ")
				sb.WriteString(strings.TrimSpace(desc))
				sb.WriteString("\n")
			}
			if expSkills, ok := expMap["skills"].(string); ok && strings.TrimSpace(expSkills) != "" {
				sb.WriteString("  Tagged Skills: ")
				sb.WriteString(strings.TrimSpace(expSkills))
				sb.WriteString("\n")
			}
			if tools, ok := expMap["tools"].(string); ok && strings.TrimSpace(tools) != "" {
				sb.WriteString("  Tools: ")
				sb.WriteString(strings.TrimSpace(tools))
				sb.WriteString("\n")
			}
		}
		parts = append(parts, sb.String())
	}

	// Projects: projectName, technologies, skills, description.
	if projects, ok := resumeData["projects"].([]interface{}); ok && len(projects) > 0 {
		var sb strings.Builder
		sb.WriteString("Projects:\n")
		for i, proj := range projects {
			projMap, ok := proj.(map[string]interface{})
			if !ok {
				continue
			}
			sb.WriteString("Project ")
			sb.WriteString(strings.TrimSpace(toOrdinal(i + 1)))
			sb.WriteString(":\n")

			if name, ok := projMap["projectName"].(string); ok && strings.TrimSpace(name) != "" {
				sb.WriteString("  Name: ")
				sb.WriteString(strings.TrimSpace(name))
				sb.WriteString("\n")
			}
			if tech, ok := projMap["technologies"].(string); ok && strings.TrimSpace(tech) != "" {
				sb.WriteString("  Technologies: ")
				sb.WriteString(strings.TrimSpace(tech))
				sb.WriteString("\n")
			}
			if projSkills, ok := projMap["skills"].(string); ok && strings.TrimSpace(projSkills) != "" {
				sb.WriteString("  Skills: ")
				sb.WriteString(strings.TrimSpace(projSkills))
				sb.WriteString("\n")
			}
			if desc, ok := projMap["description"].(string); ok && strings.TrimSpace(desc) != "" {
				sb.WriteString("  Description: ")
				sb.WriteString(strings.TrimSpace(desc))
				sb.WriteString("\n")
			}
		}
		parts = append(parts, sb.String())
	}

	// Education: degree, school, field, location.
	if education, ok := resumeData["education"].([]interface{}); ok && len(education) > 0 {
		var sb strings.Builder
		sb.WriteString("Education:\n")
		for i, edu := range education {
			eduMap, ok := edu.(map[string]interface{})
			if !ok {
				continue
			}
			sb.WriteString("Education ")
			sb.WriteString(strings.TrimSpace(toOrdinal(i + 1)))
			sb.WriteString(":\n")

			if degree, ok := eduMap["degree"].(string); ok && strings.TrimSpace(degree) != "" {
				sb.WriteString("  Degree: ")
				sb.WriteString(strings.TrimSpace(degree))
				sb.WriteString("\n")
			}
			if field, ok := eduMap["field"].(string); ok && strings.TrimSpace(field) != "" {
				sb.WriteString("  Field: ")
				sb.WriteString(strings.TrimSpace(field))
				sb.WriteString("\n")
			}
			if school, ok := eduMap["school"].(string); ok && strings.TrimSpace(school) != "" {
				sb.WriteString("  School: ")
				sb.WriteString(strings.TrimSpace(school))
				sb.WriteString("\n")
			}
			if location, ok := eduMap["location"].(string); ok && strings.TrimSpace(location) != "" {
				sb.WriteString("  Location: ")
				sb.WriteString(strings.TrimSpace(location))
				sb.WriteString("\n")
			}
		}
		parts = append(parts, sb.String())
	}

	// Existing skills string, if any.
	switch v := resumeData["skills"].(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			parts = append(parts, "Existing Skills Field:\n"+strings.TrimSpace(v))
		}
	case []interface{}:
		var collected []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				collected = append(collected, strings.TrimSpace(s))
			}
		}
		if len(collected) > 0 {
			parts = append(parts, "Existing Skills List:\n"+strings.Join(collected, ", "))
		}
	}

	resumeText := strings.Join(parts, "\n\n")

	jobContext := ""
	if trimmedJD := strings.TrimSpace(jobDescription); trimmedJD != "" {
		jobContext = "\n\nTARGET JOB DESCRIPTION:\n" + trimmedJD
	}

	return `You are an expert resume analyst and career coach.

Your job is to read the candidate's in-progress resume data and (optionally) a target job description, then extract a clean, de-duplicated list of skills and tools.

Return ONLY valid JSON using exactly this schema:
{
  "skills": [
    "Skill One",
    "Skill Two"
  ]
}

Guidelines:
- Include both technical and soft skills that are clearly supported by the resume or helpful for the target job.
- Consolidate near-duplicates into a single, standard label (e.g., "Golang" and "Go" -> "Go"; "Microsoft Azure" and "Azure" -> "Azure").
- Use concise, recruiter-friendly labels (e.g., "Distributed Systems", "REST APIs", "Kubernetes", "React", "Python").
- Do NOT include generic filler words like "team player", "hard-working", "etc".
- Do NOT invent skills that are not implied by the resume or job description.

RESUME DATA:
` + resumeText + `
` + jobContext + `

Now return ONLY the JSON object with the "skills" array.`
}

// buildSkillsCategorizationPrompt asks the LangChain model to group the user's skills
// into clear, recruiter-friendly buckets.
func buildSkillsCategorizationPrompt(skillsText, jobDescription string) string {
	base := strings.TrimSpace(skillsText)
	jd := strings.TrimSpace(jobDescription)

	jobContext := ""
	if jd != "" {
		jobContext = "\n\nTARGET JOB DESCRIPTION:\n" + jd
	}

	return `You are an expert technical recruiter and resume specialist.

The user has provided this raw skills list (comma separated, line separated, or mixed):
` + base + jobContext + `

Your task is to group these skills into clear, recruiter-friendly categories.

Return ONLY valid JSON using exactly this schema:
{
  "categories": [
    {
      "name": "Cloud",
      "skills": ["AWS", "Azure", "GCP"]
    },
    {
      "name": "Languages",
      "skills": ["Go", "Python", "Java"]
    }
  ]
}

Guidelines:
- Use concise category names such as "Cloud", "Languages", "Frameworks", "DevOps", "Databases", "Testing", "Data & Analytics", etc.
- Each skill must appear in at least one category; if you are unsure, place it in a reasonable catch-all like "Other" or "General".
- Do NOT invent new skills that are not present in the input.
- Consolidate near-duplicates (e.g., "Golang" and "Go" -> "Go").
- If a skill naturally belongs in multiple categories, include it in the single most appropriate one.

Now return ONLY the JSON object with the "categories" array.`
}

// parseSkillsFromLLM attempts to interpret the LLM output as the JSON shape
// described in buildSkillsExtractionPrompt. It is resilient to markdown fences
// and falls back to comma/line splitting if JSON parsing fails.
func parseSkillsFromLLM(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)

	// Strip common markdown fences if present.
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}
	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSuffix(trimmed, "```")
	}
	trimmed = strings.TrimSpace(trimmed)

	// First, try to parse as JSON with the expected schema.
	type skillsWrapper struct {
		Skills []string `json:"skills"`
	}

	var wrapper skillsWrapper
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Skills) > 0 {
		return wrapper.Skills, nil
	}

	// Fallback: treat the response as a plain-text list.
	var candidates []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Strip leading bullets like "-", "*", "•", or "1."
		line = strings.TrimLeft(line, "-*• \t")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by comma or semicolon inside the line.
		for _, chunk := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		}) {
			if val := strings.TrimSpace(chunk); val != "" {
				candidates = append(candidates, val)
			}
		}
	}

	if len(candidates) == 0 {
		return []string{}, nil
	}

	return candidates, nil
}

// parseCategoriesFromLLM attempts to interpret the LLM output as
// { "categories": [ { "name": "...", "skills": ["..."] }, ... ] }.
// If parsing fails, it returns an empty slice (the caller can fall back).
func parseCategoriesFromLLM(raw string) ([]SkillCategory, error) {
	trimmed := strings.TrimSpace(raw)

	// Strip markdown fences if present.
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}
	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSuffix(trimmed, "```")
	}
	trimmed = strings.TrimSpace(trimmed)

	type categoriesWrapper struct {
		Categories []SkillCategory `json:"categories"`
	}

	var wrapper categoriesWrapper
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Categories) > 0 {
		return wrapper.Categories, nil
	}

	// If the model returned only a flat skills list, reuse parseSkillsFromLLM
	// and wrap them in a single "General" category.
	skills, err := parseSkillsFromLLM(trimmed)
	if err != nil || len(skills) == 0 {
		return []SkillCategory{}, nil
	}

	return []SkillCategory{
		{
			Name:   "General",
			Skills: skills,
		},
	}, nil
}

// toOrdinal converts an integer index (1-based) into a simple human-readable
// ordinal label (e.g., "1", "2", "3"). We intentionally keep this simple
// because it is only used for contextual labeling in the prompt.
func toOrdinal(n int) string {
	if n <= 0 {
		return "1"
	}
	return strconv.Itoa(n)
}
