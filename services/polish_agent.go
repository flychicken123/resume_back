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

// ChatMessage represents a single message in conversation history.
type ChatMessage struct {
	Role string
	Text string
}

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
func (a *PolishAgent) DetectPolishIntent(ctx context.Context, input string, resumeData map[string]interface{}, history []ChatMessage) (PolishIntent, error) {
	if a == nil || a.llm == nil {
		return PolishIntent{}, fmt.Errorf("polish agent is not initialized")
	}

	resumeContext := buildResumeContextForPolish(resumeData)

	// Build conversation history context
	var historyText string
	if len(history) > 0 {
		var sb strings.Builder
		sb.WriteString("\nConversation context:\n")
		for _, msg := range history {
			role := strings.ToLower(msg.Role)
			if role != "assistant" && role != "bot" {
				role = "user"
			}
			fmt.Fprintf(&sb, "%s: %s\n", role, strings.TrimSpace(msg.Text))
		}
		historyText = sb.String()
	}

	prompt := fmt.Sprintf(`You are analyzing user input to detect if they want to polish, optimize, or improve their resume content.

%s
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

User input: %s`, resumeContext, historyText, input)

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

// PolishExperience polishes a specific experience entry using AI with low temperature and validation.
func (a *PolishAgent) PolishExperience(ctx context.Context, experience map[string]interface{}, jobDescription string, conversationContext string) (map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	description, _ := experience["description"].(string)
	if description == "" {
		return experience, nil // Nothing to polish
	}

	jobTitle, _ := experience["jobTitle"].(string)
	company, _ := experience["company"].(string)

	originalText := fmt.Sprintf("%s at %s:\n%s", jobTitle, company, description)
	prompt := BuildExperienceOptimizationPrompt(jobDescription, originalText) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	// Two-pass validation to catch fabrications
	validated, fabrications, _ := ValidateOutputWithAI(description, polished)
	if len(fabrications) > 0 {
		fmt.Printf("[PolishExperience] Fabrications removed: %v\n", fabrications)
	}

	// Deduplicate bullets — AI sometimes repeats the same lines
	cleaned := cleanupPolishedText(validated)
	cleaned = deduplicateBullets(cleaned)

	// Ensure we don't return more bullets than the original
	origCount := len(splitBullets(description))
	newBullets := splitBullets(cleaned)
	if origCount > 0 && len(newBullets) > origCount {
		newBullets = newBullets[:origCount]
		cleaned = strings.Join(newBullets, "\n")
		fmt.Printf("[PolishExperience] Trimmed from %d to %d bullets to match original\n", len(splitBullets(validated)), origCount)
	}

	// Create a copy with polished description
	result := make(map[string]interface{})
	for k, v := range experience {
		result[k] = v
	}
	result["description"] = cleaned

	return result, nil
}

// PolishEducation polishes education entry using AI with low temperature and validation.
func (a *PolishAgent) PolishEducation(ctx context.Context, education map[string]interface{}, conversationContext string) (map[string]interface{}, error) {
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

	prompt := BuildEducationOptimizationPrompt(educationText, "") + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	// Two-pass validation
	validated, fabrications, _ := ValidateOutputWithAI(educationText, polished)
	if len(fabrications) > 0 {
		fmt.Printf("[PolishEducation] Fabrications removed: %v\n", fabrications)
	}

	// Create a copy with polished honors/achievements
	result := make(map[string]interface{})
	for k, v := range education {
		result[k] = v
	}
	result["honors"] = cleanupPolishedText(validated)

	return result, nil
}

// PolishProject polishes a project entry using AI with low temperature and validation.
func (a *PolishAgent) PolishProject(ctx context.Context, project map[string]interface{}, jobDescription string, conversationContext string) (map[string]interface{}, error) {
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

	prompt := BuildProjectOptimizationPrompt(jobDescription, projectText, "") + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	// Two-pass validation
	validated, fabrications, _ := ValidateOutputWithAI(description, polished)
	if len(fabrications) > 0 {
		fmt.Printf("[PolishProject] Fabrications removed: %v\n", fabrications)
	}

	result := make(map[string]interface{})
	for k, v := range project {
		result[k] = v
	}
	result["description"] = cleanupPolishedText(validated)

	return result, nil
}

// PolishSummary polishes the professional summary using AI with low temperature and validation.
func (a *PolishAgent) PolishSummary(ctx context.Context, summary string, resumeData map[string]interface{}, conversationContext string) (string, error) {
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

	prompt := BuildSummaryOptimizationPrompt(experience, education, skills, summary) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return "", err
	}

	// Two-pass validation
	validated, fabrications, _ := ValidateOutputWithAI(summary, polished)
	if len(fabrications) > 0 {
		fmt.Printf("[PolishSummary] Fabrications removed: %v\n", fabrications)
	}

	return cleanupPolishedText(validated), nil
}

// PolishSkills polishes the skills section using AI with low temperature.
func (a *PolishAgent) PolishSkills(ctx context.Context, skills string, jobDescription string, conversationContext string) (string, error) {
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

CRITICAL RULES:
1. ONLY include skills that are explicitly listed in "Current Skills" above
2. Do NOT add any new skills, even if they seem implied or relevant to the job
3. Do NOT invent certifications, tools, or technologies not listed
4. Organize and format the existing skills only

Instructions:
1. Organize skills by category if appropriate (e.g., Programming Languages, Frameworks, Tools, Soft Skills)
2. Use proper capitalization and formatting
3. Remove redundant or overly generic skills
4. Keep the most relevant skills for the job description at the top
5. Format as a clean, comma-separated list or categorized format

Return only the polished skills, no explanations.`, jobDescription, skills) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	polished, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
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

// splitBullets splits a description into individual bullet lines.
func splitBullets(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// deduplicateBullets removes duplicate lines from a newline-separated description.
func deduplicateBullets(text string) string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

// PolishExperiencesBatch polishes multiple experiences in a single API call.
// This reduces API calls from N to 1, helping avoid rate limits.
func (a *PolishAgent) PolishExperiencesBatch(ctx context.Context, experiences []map[string]interface{}, jobDescription string, conversationContext string) ([]map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	if len(experiences) == 0 {
		return experiences, nil
	}

	// If only one experience, use the single method
	if len(experiences) == 1 {
		polished, err := a.PolishExperience(ctx, experiences[0], jobDescription, conversationContext)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{polished}, nil
	}

	// Build batch input
	var inputBuilder strings.Builder
	for i, exp := range experiences {
		description, _ := exp["description"].(string)
		if description == "" {
			continue
		}
		jobTitle, _ := exp["jobTitle"].(string)
		company, _ := exp["company"].(string)
		inputBuilder.WriteString(fmt.Sprintf("=== EXPERIENCE %d ===\n", i))
		inputBuilder.WriteString(fmt.Sprintf("Job Title: %s\n", jobTitle))
		inputBuilder.WriteString(fmt.Sprintf("Company: %s\n", company))
		inputBuilder.WriteString(fmt.Sprintf("Description:\n%s\n\n", description))
	}

	if inputBuilder.Len() == 0 {
		return experiences, nil // No descriptions to polish
	}

	prompt := fmt.Sprintf(`You are an expert resume writer. Polish multiple work experience descriptions to be more impactful.

Job Description (for context):
%s

%s

For EACH experience, transform the description using: [ACTION VERB] + [WHAT YOU DID] + [HOW/WHY] + [MEASURABLE IMPACT]

CRITICAL RULES:
1. NEVER invent accomplishments, metrics, or technologies not in the original
2. Start each bullet with strong action verbs (Led, Built, Developed, Implemented)
3. Keep statements qualitative if original lacks metrics - do NOT fabricate numbers
4. Each achievement on a new line, no bullet symbols

Return ONLY valid JSON array with polished descriptions:
[
  {"index": 0, "description": "polished description for experience 0"},
  {"index": 1, "description": "polished description for experience 1"}
]

Return ONLY the JSON array, no explanations.`, jobDescription, inputBuilder.String()) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	clean := sanitizeLLMJSON(raw)

	var polishedResults []struct {
		Index       int    `json:"index"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(clean), &polishedResults); err != nil {
		return nil, fmt.Errorf("failed to parse batch polish JSON: %w", err)
	}

	// Map polished descriptions back to experiences
	result := make([]map[string]interface{}, len(experiences))
	for i, exp := range experiences {
		// Copy original
		result[i] = make(map[string]interface{})
		for k, v := range exp {
			result[i][k] = v
		}
	}

	// Apply polished descriptions with validation
	for _, p := range polishedResults {
		if p.Index >= 0 && p.Index < len(result) {
			originalDesc, _ := experiences[p.Index]["description"].(string)
			validated, fabrications, _ := ValidateOutputWithAI(originalDesc, p.Description)
			if len(fabrications) > 0 {
				fmt.Printf("[PolishExperiencesBatch] Index %d fabrications removed: %v\n", p.Index, fabrications)
			}
			result[p.Index]["description"] = cleanupPolishedText(validated)
		}
	}

	return result, nil
}

// PolishProjectsBatch polishes multiple projects in a single API call.
func (a *PolishAgent) PolishProjectsBatch(ctx context.Context, projects []map[string]interface{}, jobDescription string, conversationContext string) ([]map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	if len(projects) == 0 {
		return projects, nil
	}

	// If only one project, use the single method
	if len(projects) == 1 {
		polished, err := a.PolishProject(ctx, projects[0], jobDescription, conversationContext)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{polished}, nil
	}

	// Build batch input
	var inputBuilder strings.Builder
	for i, proj := range projects {
		description, _ := proj["description"].(string)
		if description == "" {
			continue
		}
		projectName, _ := proj["projectName"].(string)
		technologies, _ := proj["technologies"].(string)
		inputBuilder.WriteString(fmt.Sprintf("=== PROJECT %d ===\n", i))
		inputBuilder.WriteString(fmt.Sprintf("Name: %s\n", projectName))
		inputBuilder.WriteString(fmt.Sprintf("Technologies: %s\n", technologies))
		inputBuilder.WriteString(fmt.Sprintf("Description:\n%s\n\n", description))
	}

	if inputBuilder.Len() == 0 {
		return projects, nil
	}

	prompt := fmt.Sprintf(`You are an expert resume writer. Polish multiple project descriptions to be more impactful.

Job Description (for context):
%s

%s

For EACH project, enhance the description to highlight technical achievements and impact.

CRITICAL RULES:
1. NEVER invent features, technologies, or metrics not in the original
2. Use strong technical action verbs (Architected, Engineered, Deployed, Implemented)
3. Keep statements qualitative if original lacks metrics
4. Each achievement on a new line, no bullet symbols

Return ONLY valid JSON array:
[
  {"index": 0, "description": "polished description for project 0"},
  {"index": 1, "description": "polished description for project 1"}
]

Return ONLY the JSON array, no explanations.`, jobDescription, inputBuilder.String()) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	clean := sanitizeLLMJSON(raw)

	var polishedResults []struct {
		Index       int    `json:"index"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(clean), &polishedResults); err != nil {
		return nil, fmt.Errorf("failed to parse batch polish JSON: %w", err)
	}

	// Map polished descriptions back to projects
	result := make([]map[string]interface{}, len(projects))
	for i, proj := range projects {
		result[i] = make(map[string]interface{})
		for k, v := range proj {
			result[i][k] = v
		}
	}

	// Apply polished descriptions with validation
	for _, p := range polishedResults {
		if p.Index >= 0 && p.Index < len(result) {
			originalDesc, _ := projects[p.Index]["description"].(string)
			validated, fabrications, _ := ValidateOutputWithAI(originalDesc, p.Description)
			if len(fabrications) > 0 {
				fmt.Printf("[PolishProjectsBatch] Index %d fabrications removed: %v\n", p.Index, fabrications)
			}
			result[p.Index]["description"] = cleanupPolishedText(validated)
		}
	}

	return result, nil
}

// PolishEducationBatch polishes multiple education entries in a single API call.
func (a *PolishAgent) PolishEducationBatch(ctx context.Context, educations []map[string]interface{}, conversationContext string) ([]map[string]interface{}, error) {
	if a == nil || a.llm == nil {
		return nil, fmt.Errorf("polish agent is not initialized")
	}

	if len(educations) == 0 {
		return educations, nil
	}

	// If only one education, use the single method
	if len(educations) == 1 {
		polished, err := a.PolishEducation(ctx, educations[0], conversationContext)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{polished}, nil
	}

	// Build batch input
	var inputBuilder strings.Builder
	for i, edu := range educations {
		degree, _ := edu["degree"].(string)
		school, _ := edu["school"].(string)
		field, _ := edu["field"].(string)
		honors, _ := edu["honors"].(string)
		gpa, _ := edu["gpa"].(string)

		inputBuilder.WriteString(fmt.Sprintf("=== EDUCATION %d ===\n", i))
		inputBuilder.WriteString(fmt.Sprintf("Degree: %s\n", degree))
		inputBuilder.WriteString(fmt.Sprintf("School: %s\n", school))
		inputBuilder.WriteString(fmt.Sprintf("Field: %s\n", field))
		if gpa != "" {
			inputBuilder.WriteString(fmt.Sprintf("GPA: %s\n", gpa))
		}
		if honors != "" {
			inputBuilder.WriteString(fmt.Sprintf("Honors/Achievements: %s\n", honors))
		}
		inputBuilder.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are an expert resume writer. Polish multiple education entries to be more professional.

%s

For EACH education entry, create or enhance the honors/achievements section.

CRITICAL RULES:
1. NEVER invent achievements, honors, or activities not implied by the original
2. Make descriptions more professional and impactful
3. Focus on relevant coursework, achievements, and honors
4. Keep it concise

Return ONLY valid JSON array:
[
  {"index": 0, "honors": "polished honors/achievements for education 0"},
  {"index": 1, "honors": "polished honors/achievements for education 1"}
]

Return ONLY the JSON array, no explanations.`, inputBuilder.String()) + conversationContext

	// Use low temperature (0.1) to reduce hallucination
	raw, err := llms.GenerateFromSinglePrompt(ctx, a.llm, prompt, llms.WithTemperature(0.1))
	if err != nil {
		return nil, err
	}

	clean := sanitizeLLMJSON(raw)

	var polishedResults []struct {
		Index  int    `json:"index"`
		Honors string `json:"honors"`
	}
	if err := json.Unmarshal([]byte(clean), &polishedResults); err != nil {
		return nil, fmt.Errorf("failed to parse batch polish JSON: %w", err)
	}

	// Map polished honors back to educations
	result := make([]map[string]interface{}, len(educations))
	for i, edu := range educations {
		result[i] = make(map[string]interface{})
		for k, v := range edu {
			result[i][k] = v
		}
	}

	// Apply polished honors with validation
	for _, p := range polishedResults {
		if p.Index >= 0 && p.Index < len(result) {
			originalHonors, _ := educations[p.Index]["honors"].(string)
			validated, fabrications, _ := ValidateOutputWithAI(originalHonors, p.Honors)
			if len(fabrications) > 0 {
				fmt.Printf("[PolishEducationBatch] Index %d fabrications removed: %v\n", p.Index, fabrications)
			}
			result[p.Index]["honors"] = cleanupPolishedText(validated)
		}
	}

	return result, nil
}
