package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"resumeai/services"
)

// ChatIntent represents the classified intent from a user's chat message.
type ChatIntent struct {
	Intent          string            `json:"intent"`
	Confidence      float64           `json:"confidence"`
	ExtractedParams map[string]string `json:"extractedParams"`
}

// Intent constants
const (
	IntentCoverLetter          = "cover_letter"
	IntentRecommendationLetter = "recommendation_letter"
	IntentResumeAdvice         = "resume_advice"
	IntentGenerateSummary      = "generate_summary"
	IntentOptimizeExperience   = "optimize_experience"
	IntentImproveGrammar       = "improve_grammar"
	IntentGenerateSkills       = "generate_skills"
	IntentCategorizeSkills     = "categorize_skills"
	IntentOptimizeProject      = "optimize_project"
	IntentPolish               = "polish"
	IntentGeneralChat          = "general_chat"
)

const intentConfidenceThreshold = 0.7

// classifyIntent uses Gemini Flash to classify the user's message into a supported intent.
func classifyIntent(ctx context.Context, message string, resumeData map[string]interface{}, history []chatMessage) (ChatIntent, error) {
	classifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	hasResumeData := len(resumeData) > 0
	hasJobDescription := false
	if jd, ok := resumeData["jobDescription"].(string); ok && strings.TrimSpace(jd) != "" {
		hasJobDescription = true
	}

	// Build conversation history context (full history)
	var historyText string
	if len(history) > 0 {
		var sb strings.Builder
		for _, msg := range history {
			role := strings.ToLower(msg.Role)
			if role != "assistant" && role != "bot" {
				role = "user"
			}
			fmt.Fprintf(&sb, "%s: %s\n", role, strings.TrimSpace(msg.Text))
		}
		historyText = sb.String()
	}

	prompt := fmt.Sprintf(`You are an intent classifier for a resume-building AI assistant called HiHired.
Given the user's message and conversation history, classify it into exactly one of these intents:

- cover_letter: User wants to generate a cover letter (must be a clear request, not a follow-up reply)
- recommendation_letter: User wants a recommendation/reference letter or third-party letter
- resume_advice: User wants feedback, analysis, or advice on their resume
- generate_summary: User wants to generate or rewrite their professional summary
- optimize_experience: User wants to optimize/tailor work experience specifically for a job description
- improve_grammar: User wants grammar/writing fixes for experience or summary (not content changes)
- generate_skills: User wants to auto-generate or suggest skills from their resume
- categorize_skills: User wants to organize/categorize/group their existing skills
- optimize_project: User wants to optimize/tailor project descriptions for a job
- polish: User wants to polish/enhance/refine/rewrite a resume section for better impact (not job-specific tailoring)
- general_chat: General question, conversational reply, or anything not matching above

IMPORTANT RULES:
- If the user is replying conversationally to a previous message (e.g., "I don't have one", "ok", "thanks", "never mind", "no"), classify as general_chat.
- If the user sends a URL without explicitly asking to generate something, classify as general_chat.
- Only classify as a feature intent if the user is clearly requesting that feature to be performed.
- If the assistant already generated content (e.g., a cover letter) in the recent history and the user is just responding, classify as general_chat.

Also extract any parameters from the message:
- companyName: company name if mentioned
- position: job title/position if mentioned
- section: which resume section if mentioned (experience, education, projects, summary, skills)

Context:
- User has resume data: %v
- User has job description: %v

%sUser message: "%s"

Respond ONLY with valid JSON (no markdown, no code fences):
{"intent": "...", "confidence": 0.0, "extractedParams": {"key": "value"}}`,
		hasResumeData, hasJobDescription,
		func() string {
			if historyText != "" {
				return fmt.Sprintf("Recent conversation:\n%s\n", historyText)
			}
			return ""
		}(),
		message,
	)

	_ = classifyCtx // timeout is applied via context

	raw, err := services.CallGeminiFlash(prompt)
	if err != nil {
		return ChatIntent{Intent: IntentGeneralChat}, fmt.Errorf("gemini flash classification failed: %w", err)
	}

	// Parse JSON response
	raw = strings.TrimSpace(raw)
	// Strip markdown code fences if present
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var intent ChatIntent
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		return ChatIntent{Intent: IntentGeneralChat}, fmt.Errorf("failed to parse intent JSON (%s): %w", raw, err)
	}

	// Validate intent is one of our known intents
	switch intent.Intent {
	case IntentCoverLetter, IntentRecommendationLetter, IntentResumeAdvice,
		IntentGenerateSummary, IntentOptimizeExperience, IntentImproveGrammar,
		IntentGenerateSkills, IntentCategorizeSkills, IntentOptimizeProject,
		IntentPolish, IntentGeneralChat:
		// valid
	default:
		log.Printf("[INTENT] Unknown intent from classifier: %s, treating as general_chat", intent.Intent)
		intent.Intent = IntentGeneralChat
	}

	if intent.ExtractedParams == nil {
		intent.ExtractedParams = make(map[string]string)
	}

	return intent, nil
}

// checkMissingParams checks if the required data for a given intent is available in resumeData.
func checkMissingParams(intent ChatIntent, resumeData map[string]interface{}) []string {
	var missing []string

	hasString := func(key string) bool {
		v, ok := resumeData[key].(string)
		return ok && strings.TrimSpace(v) != ""
	}
	hasSlice := func(key string) bool {
		v, ok := resumeData[key].([]interface{})
		return ok && len(v) > 0
	}

	switch intent.Intent {
	// Cover letter and recommendation letter do NOT require jobDescription.
	// BuildCoverLetterPrompt gracefully handles empty JD.
	case IntentOptimizeExperience:
		if !hasString("jobDescription") {
			missing = append(missing, "jobDescription")
		}
		if !hasSlice("experiences") && !hasString("experience") {
			missing = append(missing, "experiences")
		}
	case IntentOptimizeProject:
		if !hasString("jobDescription") {
			missing = append(missing, "jobDescription")
		}
		if !hasSlice("projects") {
			missing = append(missing, "projects")
		}
	case IntentGenerateSummary:
		// Need at least some resume content
		hasContent := hasSlice("experiences") || hasString("experience") ||
			hasSlice("education") || hasString("skills")
		if !hasContent {
			missing = append(missing, "resume content (experience, education, or skills)")
		}
	case IntentImproveGrammar:
		section := intent.ExtractedParams["section"]
		switch section {
		case "summary":
			if !hasString("summary") {
				missing = append(missing, "summary")
			}
		case "experience", "":
			if !hasSlice("experiences") && !hasString("experience") {
				missing = append(missing, "experiences")
			}
		}
	case IntentGenerateSkills:
		// Need some resume data to extract skills from
		hasContent := hasSlice("experiences") || hasString("experience") ||
			hasSlice("projects") || hasSlice("education")
		if !hasContent {
			missing = append(missing, "resume content (experience, projects, or education)")
		}
	case IntentCategorizeSkills:
		if !hasString("skills") {
			missing = append(missing, "skills")
		}
	case IntentResumeAdvice:
		// Resume advice can work with any resume data, just needs something
		if len(resumeData) == 0 {
			missing = append(missing, "resume data")
		}
	}

	return missing
}

// buildMissingParamResponse returns a friendly chat response asking the user to provide missing data.
func buildMissingParamResponse(intent ChatIntent) *chatResponse {
	var message string

	switch intent.Intent {
	case IntentCoverLetter:
		message = "I'd love to generate a cover letter for you! Could you provide the job description? You can paste it into the Job Description field in the builder, or share it with me here."
	case IntentRecommendationLetter:
		message = "I can write a recommendation letter for you! Please provide the job description so I can tailor it to the role."
	case IntentOptimizeExperience:
		message = "I can optimize your experience section! Please make sure you have both your work experience filled in and a job description to tailor it to."
	case IntentOptimizeProject:
		message = "I can optimize your project descriptions! Please make sure you have your projects filled in and a job description to tailor them to."
	case IntentGenerateSummary:
		message = "I can generate a professional summary for you! Please add some experience, education, or skills first so I have content to work with."
	case IntentImproveGrammar:
		message = "I can improve the grammar in your resume! Please make sure the section you'd like me to fix has some content first."
	case IntentGenerateSkills:
		message = "I can suggest skills based on your resume! Please add some experience, projects, or education first so I can extract relevant skills."
	case IntentCategorizeSkills:
		message = "I can organize your skills into categories! Please add some skills first."
	case IntentResumeAdvice:
		message = "I can analyze your resume and provide feedback! Please fill in some resume sections first so I have something to review."
	default:
		message = "I need a bit more information to help you with that. Could you fill in the relevant resume sections first?"
	}

	return &chatResponse{Reply: message}
}

// buildConversationContext creates a text block from the user's message and conversation history.
// This is appended to feature prompts so the AI is aware of the full conversation context,
// including any specific user instructions (e.g., "make it casual", "focus on leadership").
func buildConversationContext(message string, history []chatMessage) string {
	var sb strings.Builder

	if len(history) > 0 {
		sb.WriteString("\n\nConversation context (use this to understand what the user wants):\n")
		for _, msg := range history {
			role := strings.ToLower(msg.Role)
			if role != "assistant" && role != "bot" {
				role = "user"
			}
			fmt.Fprintf(&sb, "%s: %s\n", role, strings.TrimSpace(msg.Text))
		}
	}

	fmt.Fprintf(&sb, "user: %s\n", strings.TrimSpace(message))
	fmt.Fprintf(&sb, "\nIMPORTANT: Follow any specific instructions or preferences the user mentioned in the conversation above (e.g., tone, style, focus areas, things to include or exclude).")

	return sb.String()
}

// routeToFeature executes the feature corresponding to the classified intent.
func routeToFeature(ctx context.Context, intent ChatIntent, req chatRequest) (*chatResponse, error) {
	resumeData := req.ResumeData
	jobDesc, _ := resumeData["jobDescription"].(string)
	conversationCtx := buildConversationContext(req.Message, req.History)

	switch intent.Intent {
	case IntentCoverLetter:
		companyName := intent.ExtractedParams["companyName"]
		// If no JD in resumeData, check if the user's message contains a URL to extract JD from
		if jobDesc == "" {
			if extractedJD, extractedCompany := tryExtractJDFromMessage(req.Message); extractedJD != "" {
				jobDesc = extractedJD
				if companyName == "" && extractedCompany != "" {
					companyName = extractedCompany
				}
				log.Printf("[INTENT] Extracted JD from URL in message (len=%d)", len(extractedJD))
			}
		}
		prompt := services.BuildCoverLetterPrompt(resumeData, jobDesc, companyName) + conversationCtx
		result, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("cover letter generation failed: %w", err)
		}
		return &chatResponse{
			Reply:         "Here's your cover letter:",
			FeatureAction: IntentCoverLetter,
			FeatureResult: result,
		}, nil

	case IntentRecommendationLetter:
		companyName := intent.ExtractedParams["companyName"]
		position := intent.ExtractedParams["position"]
		// If no JD in resumeData, check if the user's message contains a URL
		if jobDesc == "" {
			if extractedJD, extractedCompany := tryExtractJDFromMessage(req.Message); extractedJD != "" {
				jobDesc = extractedJD
				if companyName == "" && extractedCompany != "" {
					companyName = extractedCompany
				}
			}
		}
		prompt := services.BuildRecommendationLetterPrompt(resumeData, jobDesc, companyName, position) + conversationCtx
		result, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("recommendation letter generation failed: %w", err)
		}
		return &chatResponse{
			Reply:         "Here's your recommendation letter:",
			FeatureAction: IntentRecommendationLetter,
			FeatureResult: result,
		}, nil

	case IntentResumeAdvice:
		prompt := services.BuildResumeAdvicePrompt(resumeData, jobDesc) + conversationCtx
		advice, err := services.CallGeminiWithAPIKey(prompt)
		if err != nil {
			return nil, fmt.Errorf("resume advice failed: %w", err)
		}
		return &chatResponse{
			Reply:         advice,
			FeatureAction: IntentResumeAdvice,
			FeatureResult: advice,
		}, nil

	case IntentGenerateSummary:
		experience := extractExperienceText(resumeData)
		education := extractEducationText(resumeData)
		skills := extractSkillsList(resumeData)
		existingSummary, _ := resumeData["summary"].(string)

		prompt := services.BuildSummaryOptimizationPromptWithSkills(
			experience, education, skills, existingSummary, jobDesc, nil, nil,
		) + conversationCtx
		summary, err := services.CallGeminiWithTemperature(prompt, 0.3)
		if err != nil {
			return nil, fmt.Errorf("summary generation failed: %w", err)
		}
		cleaned := cleanupAIResponse(summary)

		updatedData := copyResumeData(resumeData)
		updatedData["summary"] = cleaned

		return &chatResponse{
			Reply:             "Here's your professional summary:",
			FeatureAction:     IntentGenerateSummary,
			FeatureResult:     cleaned,
			UpdatedResumeData: updatedData,
		}, nil

	case IntentOptimizeExperience:
		experience := extractExperienceText(resumeData)
		prompt := services.BuildExperienceOptimizationPromptWithSkills(jobDesc, experience, nil, nil) + conversationCtx
		result, err := services.CallGeminiWithTemperature(prompt, 0.3)
		if err != nil {
			return nil, fmt.Errorf("experience optimization failed: %w", err)
		}
		validated := services.ValidateAndCleanOutput(experience, result)
		cleaned := cleanupAIResponse(validated)

		return &chatResponse{
			Reply:         "Here's your optimized experience:",
			FeatureAction: IntentOptimizeExperience,
			FeatureResult: cleaned,
		}, nil

	case IntentImproveGrammar:
		section := intent.ExtractedParams["section"]
		if section == "summary" {
			summary, _ := resumeData["summary"].(string)
			prompt := services.BuildSummaryGrammarPrompt(summary) + conversationCtx
			improved, err := services.CallGeminiWithTemperature(prompt, 0.3)
			if err != nil {
				return nil, fmt.Errorf("summary grammar improvement failed: %w", err)
			}
			cleaned := cleanupAIResponse(improved)

			updatedData := copyResumeData(resumeData)
			updatedData["summary"] = cleaned

			return &chatResponse{
				Reply:             "I've improved the grammar in your summary:",
				FeatureAction:     IntentImproveGrammar,
				FeatureResult:     cleaned,
				UpdatedResumeData: updatedData,
			}, nil
		}
		// Default to experience grammar
		experience := extractExperienceText(resumeData)
		prompt := services.BuildExperienceGrammarPrompt(experience) + conversationCtx
		improved, err := services.CallGeminiWithTemperature(prompt, 0.3)
		if err != nil {
			return nil, fmt.Errorf("experience grammar improvement failed: %w", err)
		}
		cleaned := cleanupAIResponse(improved)

		return &chatResponse{
			Reply:         "I've improved the grammar in your experience:",
			FeatureAction: IntentImproveGrammar,
			FeatureResult: cleaned,
		}, nil

	case IntentGenerateSkills:
		existingSkills := extractSkillsList(resumeData)
		prompt := buildSkillsExtractionPrompt(resumeData, jobDesc, existingSkills) + conversationCtx
		raw, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("skills generation failed: %w", err)
		}
		skills, err := parseSkillsFromLLM(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse generated skills: %w", err)
		}

		// Normalize and deduplicate
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

		updatedData := copyResumeData(resumeData)
		updatedData["skills"] = skillsText

		return &chatResponse{
			Reply:             fmt.Sprintf("I've generated %d skills for you:", len(normalized)),
			FeatureAction:     IntentGenerateSkills,
			FeatureResult:     normalized,
			UpdatedResumeData: updatedData,
		}, nil

	case IntentCategorizeSkills:
		skillsText, _ := resumeData["skills"].(string)
		prompt := buildSkillsCategorizationPrompt(skillsText, jobDesc) + conversationCtx
		raw, err := runCopilotPrompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("skills categorization failed: %w", err)
		}
		categories, err := parseCategoriesFromLLM(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse skill categories: %w", err)
		}

		// Build formatted text
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
			if len(cleaned) > 0 {
				parts = append(parts, cat.Name+": "+strings.Join(cleaned, ", "))
			}
		}
		formatted := strings.TrimSpace(strings.Join(parts, "\n\n"))
		if formatted == "" {
			formatted = skillsText
		}

		updatedData := copyResumeData(resumeData)
		updatedData["skills"] = formatted

		return &chatResponse{
			Reply:             "I've categorized your skills:",
			FeatureAction:     IntentCategorizeSkills,
			FeatureResult:     categories,
			UpdatedResumeData: updatedData,
		}, nil

	case IntentOptimizeProject:
		projectData := extractProjectText(resumeData)
		prompt := services.BuildProjectOptimizationPromptWithSkills(jobDesc, projectData, "", nil, nil) + conversationCtx
		result, err := services.CallGeminiWithTemperature(prompt, 0.3)
		if err != nil {
			return nil, fmt.Errorf("project optimization failed: %w", err)
		}
		validated := services.ValidateAndCleanOutput(projectData, result)
		cleaned := cleanupAIResponse(validated)

		return &chatResponse{
			Reply:         "Here's your optimized project description:",
			FeatureAction: IntentOptimizeProject,
			FeatureResult: cleaned,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported intent: %s", intent.Intent)
	}
}

// ---------------------------------------------------------------------------
// Helpers: extract text from resumeData for prompt building
// ---------------------------------------------------------------------------

func extractExperienceText(resumeData map[string]interface{}) string {
	// Try string field first
	if exp, ok := resumeData["experience"].(string); ok && exp != "" {
		return exp
	}
	// Try experiences array
	if experiences, ok := resumeData["experiences"].([]interface{}); ok {
		var parts []string
		for _, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				title, _ := expMap["jobTitle"].(string)
				company, _ := expMap["company"].(string)
				desc, _ := expMap["description"].(string)
				entry := ""
				if title != "" || company != "" {
					entry = title + " at " + company + "\n"
				}
				if desc != "" {
					entry += desc
				}
				if entry != "" {
					parts = append(parts, strings.TrimSpace(entry))
				}
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

func extractEducationText(resumeData map[string]interface{}) string {
	if edu, ok := resumeData["education"].(string); ok && edu != "" {
		return edu
	}
	if education, ok := resumeData["education"].([]interface{}); ok {
		var parts []string
		for _, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				degree, _ := eduMap["degree"].(string)
				school, _ := eduMap["school"].(string)
				if degree != "" || school != "" {
					parts = append(parts, degree+" from "+school)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func extractSkillsList(resumeData map[string]interface{}) []string {
	if s, ok := resumeData["skills"].(string); ok && s != "" {
		var skills []string
		for _, skill := range strings.Split(s, ",") {
			trimmed := strings.TrimSpace(skill)
			if trimmed != "" {
				skills = append(skills, trimmed)
			}
		}
		return skills
	}
	if s, ok := resumeData["skills"].([]interface{}); ok {
		var skills []string
		for _, skill := range s {
			if str, ok := skill.(string); ok && str != "" {
				skills = append(skills, str)
			}
		}
		return skills
	}
	return nil
}

func extractProjectText(resumeData map[string]interface{}) string {
	if projects, ok := resumeData["projects"].([]interface{}); ok {
		var parts []string
		for _, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				name, _ := projMap["projectName"].(string)
				desc, _ := projMap["description"].(string)
				entry := ""
				if name != "" {
					entry = name + "\n"
				}
				if desc != "" {
					entry += desc
				}
				if entry != "" {
					parts = append(parts, strings.TrimSpace(entry))
				}
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// tryExtractJDFromMessage checks if the message contains a URL and attempts to extract
// a job description from it using the existing FetchAndProcessURL service.
// Returns (jobDescription, companyName). Both may be empty if no URL found or extraction fails.
var urlRegex = regexp.MustCompile(`https?://[^\s]+`)

func tryExtractJDFromMessage(message string) (string, string) {
	match := urlRegex.FindString(message)
	if match == "" {
		return "", ""
	}

	prompt := `Extract the job description from this page. Focus on:
1. Job title
2. Company name
3. Full job description including responsibilities, requirements, and qualifications
4. Any technical skills or technologies mentioned

Return the extracted information as plain text in this format:
Job Title: [title if found]
Company: [company if found]

[Full job description text]

If this is not a job posting page, return "NOT_A_JOB_POSTING"`

	result, err := services.FetchAndProcessURL(match, prompt)
	if err != nil {
		log.Printf("[INTENT] Failed to extract JD from URL %s: %v", match, err)
		return "", ""
	}

	if strings.Contains(result, "NOT_A_JOB_POSTING") {
		return "", ""
	}

	// Parse company name from the result
	var company string
	lines := strings.Split(result, "\n")
	var descriptionStart int
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Company:") {
			company = strings.TrimSpace(strings.TrimPrefix(line, "Company:"))
		} else if line != "" && i > 1 {
			descriptionStart = i
			break
		}
	}

	var description string
	if descriptionStart > 0 {
		description = strings.TrimSpace(strings.Join(lines[descriptionStart:], "\n"))
	} else {
		description = strings.TrimSpace(result)
	}

	return description, company
}

// copyResumeData is defined in polish.go
