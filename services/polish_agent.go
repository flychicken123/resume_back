package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// PolishAgent detects resume polish/optimize intents and identifies target sections.
type PolishAgent struct {
	llm llms.Model
}

// PolishIntent represents the parsed intent from user's polish request.
type PolishIntent struct {
	IsPolishRequest bool     `json:"isPolishRequest"` // Whether this is a polish/optimize request
	Section         string   `json:"section"`         // "experience", "education", "projects", "summary", "skills", "all"
	Identifier      string   `json:"identifier"`      // Company name, project name, school name to match
	EntryIndex      int      `json:"entryIndex"`      // Index of entry if identifiable (-1 if not)
	Message         string   `json:"message"`         // Confirmation or clarification message
	NeedsClarification bool  `json:"needsClarification"` // Whether we need user to clarify which entry
	ClarificationQuestion string `json:"clarificationQuestion"` // Question to ask if clarification needed
}

// PolishResult contains the polished content for a resume section.
type PolishResult struct {
	Section         string                   `json:"section"`
	Identifier      string                   `json:"identifier"`
	EntryIndex      int                      `json:"entryIndex"`
	OriginalContent interface{}              `json:"originalContent"`
	PolishedContent interface{}              `json:"polishedContent"`
	Message         string                   `json:"message"`
}

// NewPolishAgent constructs a LangChain-backed agent for polish intent detection.
func NewPolishAgent() (*PolishAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	llm, err := googleai.New(context.Background(), googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for polish agent: %w", err)
	}

	return &PolishAgent{llm: llm}, nil
}

// DetectPolishIntent analyzes user input to detect polish/optimize requests.
func (a *PolishAgent) DetectPolishIntent(ctx context.Context, input string, resumeData map[string]interface{}) (PolishIntent, error) {
	if a == nil || a.llm == nil {
		return PolishIntent{}, fmt.Errorf("polish agent is not initialized")
	}

	resumeContext := buildResumeContextForPolish(resumeData)

	prompt := fmt.Sprintf(`You are analyzing user input to detect if they want to polish, optimize, or improve their resume content.

%s

The user may use words like:
- "polish", "improve", "enhance", "optimize", "make better", "rewrite", "refine"
- Target specific sections: experience, education, projects, summary, skills
- Target specific entries: "my experience at Microsoft", "Google project", "Stanford education"

SECTIONS:
- experience: Work experience entries (identified by company name or job title)
- education: Education entries (identified by school name or degree)
- projects: Project entries (identified by project name)
- summary: Professional summary
- skills: Skills list
- all: Polish entire resume

RULES:
1. Detect if this is a polish/optimize request
2. Identify which section to polish
3. If they mention a specific company/school/project name, extract it as the identifier
4. If NO specific entry is mentioned, set identifier to empty string - this means polish ALL entries in that section
5. Match identifier case-insensitively against the resume data
6. NEVER set needsClarification to true - always proceed with polishing using existing data
7. Always set needsClarification=false and clarificationQuestion=""

EXAMPLES:
- "polish my experience at Microsoft" → isPolishRequest=true, section="experience", identifier="Microsoft", needsClarification=false
- "improve my Google experience" → isPolishRequest=true, section="experience", identifier="Google", needsClarification=false
- "optimize my education" → isPolishRequest=true, section="education", identifier="", needsClarification=false (polish ALL education entries)
- "make my summary better" → isPolishRequest=true, section="summary", needsClarification=false
- "polish all my experiences" → isPolishRequest=true, section="experience", identifier="", needsClarification=false
- "enhance my projects" → isPolishRequest=true, section="projects", identifier="", needsClarification=false (polish ALL projects)
- "improve my AWS Lambda project" → isPolishRequest=true, section="projects", identifier="AWS Lambda", needsClarification=false
- "polish my resume" → isPolishRequest=true, section="all", needsClarification=false
- "what is my name?" → isPolishRequest=false

Return ONLY valid JSON:
{
  "isPolishRequest": <boolean>,
  "section": "<experience|education|projects|summary|skills|all|none>",
  "identifier": "<company/school/project name or empty string>",
  "entryIndex": <index if found, -1 if not>,
  "message": "<confirmation message like 'I will polish your experience at Microsoft' or 'I will polish all your experiences'>",
  "needsClarification": false,
  "clarificationQuestion": ""
}

User input: %s`, resumeContext, input)

	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return PolishIntent{}, err
	}

	clean := sanitizeLLMJSON(raw)

	var result PolishIntent
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return PolishIntent{}, fmt.Errorf("failed to parse polish intent JSON: %w", err)
	}

	// Normalize
	result.Section = strings.TrimSpace(strings.ToLower(result.Section))
	result.Identifier = strings.TrimSpace(result.Identifier)
	result.Message = strings.TrimSpace(result.Message)
	result.ClarificationQuestion = strings.TrimSpace(result.ClarificationQuestion)

	// Try to match identifier to actual entry index
	if result.IsPolishRequest && result.Identifier != "" && result.EntryIndex < 0 {
		result.EntryIndex = findEntryIndex(resumeData, result.Section, result.Identifier)
	}

	return result, nil
}

// PolishExperience polishes a specific experience entry using AI.
func (a *PolishAgent) PolishExperience(ctx context.Context, experience map[string]interface{}, jobDescription string) (map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	description, _ := experience["description"].(string)
	if description == "" {
		return experience, nil // Nothing to polish
	}

	jobTitle, _ := experience["jobTitle"].(string)
	company, _ := experience["company"].(string)

	prompt := BuildExperienceOptimizationPrompt(jobDescription, fmt.Sprintf("%s at %s:\n%s", jobTitle, company, description))

	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return nil, err
	}

	// Create a copy with polished description
	result := make(map[string]interface{})
	for k, v := range experience {
		result[k] = v
	}
	result["description"] = cleanupPolishedText(polished)

	return result, nil
}

// PolishEducation polishes education entry using AI.
func (a *PolishAgent) PolishEducation(ctx context.Context, education map[string]interface{}) (map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	// Build education string from the entry
	degree, _ := education["degree"].(string)
	school, _ := education["school"].(string)
	field, _ := education["field"].(string)
	honors, _ := education["honors"].(string)
	gpa, _ := education["gpa"].(string)

	educationText := fmt.Sprintf("%s in %s from %s", degree, field, school)
	if gpa != "" {
		educationText += fmt.Sprintf(", GPA: %s", gpa)
	}
	if honors != "" {
		educationText += fmt.Sprintf(", %s", honors)
	}

	prompt := BuildEducationOptimizationPrompt(educationText, "")

	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return nil, err
	}

	// Create a copy with polished honors/achievements
	result := make(map[string]interface{})
	for k, v := range education {
		result[k] = v
	}
	result["honors"] = cleanupPolishedText(polished)

	return result, nil
}

// PolishProject polishes a project entry using AI.
func (a *PolishAgent) PolishProject(ctx context.Context, project map[string]interface{}, jobDescription string) (map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	description, _ := project["description"].(string)
	if description == "" {
		return project, nil
	}

	projectName, _ := project["projectName"].(string)
	technologies, _ := project["technologies"].(string)

	projectText := fmt.Sprintf("Project: %s\nTechnologies: %s\nDescription: %s", projectName, technologies, description)

	prompt := BuildProjectOptimizationPrompt(jobDescription, projectText, "")

	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for k, v := range project {
		result[k] = v
	}
	result["description"] = cleanupPolishedText(polished)

	return result, nil
}

// PolishSummary polishes the professional summary using AI.
func (a *PolishAgent) PolishSummary(ctx context.Context, summary string, resumeData map[string]interface{}) (string, error) {
	if a == nil || a.llm == nil {
		return "", fmt.Errorf("polish agent is not initialized")
	}

	if summary == "" {
		return "", nil
	}

	experience, _ := resumeData["experience"].(string)
	education, _ := resumeData["education"].(string)
	skillsStr, _ := resumeData["skills"].(string)

	// Convert skills string to slice
	var skills []string
	if skillsStr != "" {
		for _, s := range strings.Split(skillsStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				skills = append(skills, s)
			}
		}
	}

	prompt := BuildSummaryOptimizationPrompt(experience, education, skills, summary)

	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return "", err
	}

	return cleanupPolishedText(polished), nil
}

// PolishSkills polishes the skills section using AI.
func (a *PolishAgent) PolishSkills(ctx context.Context, skills string, jobDescription string) (string, error) {
	if a == nil || a.llm == nil {
		return "", fmt.Errorf("polish agent is not initialized")
	}

	if skills == "" {
		return "", nil
	}

	prompt := fmt.Sprintf(`You are an expert resume writer. Polish and organize these skills to be more professional and aligned with industry standards.

Job Description (for context):
%s

Current Skills:
%s

Instructions:
1. Organize skills by category if appropriate (e.g., Programming Languages, Frameworks, Tools, Soft Skills)
2. Use proper capitalization and formatting
3. Remove redundant or overly generic skills
4. Add relevant skills that might be implied but not stated
5. Keep the most relevant skills for the job description at the top
6. Format as a clean, comma-separated list or categorized format

Return only the polished skills, no explanations.`, jobDescription, skills)

	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt)
	if err != nil {
		return "", err
	}

	return cleanupPolishedText(polished), nil
}

func buildResumeContextForPolish(data map[string]interface{}) string {
	if len(data) == 0 {
		return "Current resume data: (empty)"
	}

	var b strings.Builder
	b.WriteString("Current resume data:\n")

	if summary, ok := data["summary"].(string); ok && summary != "" {
		b.WriteString(fmt.Sprintf("- Summary: %s\n", truncateText(summary, 100)))
	}

	if skills, ok := data["skills"].(string); ok && skills != "" {
		b.WriteString(fmt.Sprintf("- Skills: %s\n", truncateText(skills, 100)))
	}

	if experiences, ok := data["experiences"].([]interface{}); ok && len(experiences) > 0 {
		b.WriteString("- Experience entries:\n")
		for i, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				title, _ := expMap["jobTitle"].(string)
				company, _ := expMap["company"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s at %s\n", i+1, title, company))
			}
		}
	}

	if projects, ok := data["projects"].([]interface{}); ok && len(projects) > 0 {
		b.WriteString("- Project entries:\n")
		for i, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				name, _ := projMap["projectName"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, name))
			}
		}
	}

	if education, ok := data["education"].([]interface{}); ok && len(education) > 0 {
		b.WriteString("- Education entries:\n")
		for i, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				degree, _ := eduMap["degree"].(string)
				school, _ := eduMap["school"].(string)
				b.WriteString(fmt.Sprintf("  %d. %s from %s\n", i+1, degree, school))
			}
		}
	}

	return b.String()
}

func findEntryIndex(data map[string]interface{}, section, identifier string) int {
	if identifier == "" {
		return -1
	}

	identifierLower := strings.ToLower(identifier)

	switch section {
	case "experience":
		if experiences, ok := data["experiences"].([]interface{}); ok {
			for i, exp := range experiences {
				if expMap, ok := exp.(map[string]interface{}); ok {
					company, _ := expMap["company"].(string)
					title, _ := expMap["jobTitle"].(string)
					if strings.Contains(strings.ToLower(company), identifierLower) ||
						strings.Contains(strings.ToLower(title), identifierLower) {
						return i
					}
				}
			}
		}
	case "projects":
		if projects, ok := data["projects"].([]interface{}); ok {
			for i, proj := range projects {
				if projMap, ok := proj.(map[string]interface{}); ok {
					name, _ := projMap["projectName"].(string)
					if strings.Contains(strings.ToLower(name), identifierLower) {
						return i
					}
				}
			}
		}
	case "education":
		if education, ok := data["education"].([]interface{}); ok {
			for i, edu := range education {
				if eduMap, ok := edu.(map[string]interface{}); ok {
					school, _ := eduMap["school"].(string)
					degree, _ := eduMap["degree"].(string)
					if strings.Contains(strings.ToLower(school), identifierLower) ||
						strings.Contains(strings.ToLower(degree), identifierLower) {
						return i
					}
				}
			}
		}
	}

	return -1
}

func cleanupPolishedText(text string) string {
	lines := strings.Split(text, "\n")
	var cleanedLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove leading asterisks, bullets
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}
		if strings.HasPrefix(line, "•") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}

		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n\n")
}
