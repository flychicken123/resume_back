package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"resumeai/models"
	"resumeai/services"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type chatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type chatRequest struct {
	Message    string                 `json:"message" binding:"required"`
	History    []chatMessage          `json:"history"`
	SessionID  string                 `json:"session_id"`
	PagePath   string                 `json:"page_path"`
	UserEmail  string                 `json:"user_email"`
	ResumeData map[string]interface{} `json:"resume_data"`
}

type chatResponse struct {
	Reply             string                 `json:"reply"`
	IsPolishAction    bool                   `json:"isPolishAction,omitempty"`
	PolishedContent   interface{}            `json:"polishedContent,omitempty"`
	Section           string                 `json:"section,omitempty"`
	EntryIndex        int                    `json:"entryIndex,omitempty"`
	UpdatedResumeData map[string]interface{} `json:"updatedResumeData,omitempty"`
}

type knowledgeEntry struct {
	Title    string
	Summary  string
	Keywords []string
}

var assistantKnowledge = []knowledgeEntry{
	{
		Title:    "Plan overview",
		Summary:  "HiHired has three plans: Free ($0, 1 resume per week), Premium ($7.99/month, first month $1.99, up to 30 resumes with AI-generated cover letters), and Ultimate ($29.99/month, first month $6.99, up to 300 resumes with 24 hours online support).",
		Keywords: []string{"plans", "pricing", "free plan", "premium plan", "ultimate plan", "price", "cost"},
	},
	{
		Title:    "Platform overview",
		Summary:  "HiHired is a free AI-powered resume builder that helps job seekers craft ATS-friendly resumes, customize content for job descriptions, and download polished PDFs without mandatory signup.",
		Keywords: []string{"hihired", "resume", "builder", "ai", "overview", "features", "free"},
	},
	{
		Title:    "Free plan",
		Summary:  "The Free Plan costs $0, includes 1 AI-generated resume per week, provides basic templates, PDF export, and email support—ideal for getting started quickly.",
		Keywords: []string{"free", "plan", "pricing", "cost", "starter", "basic"},
	},
	{
		Title:    "Premium plan",
		Summary:  "Premium Plan is $7.99/month (first month $1.99) and unlocks up to 30 resumes per month with AI-generated cover letters.",
		Keywords: []string{"premium", "upgrade", "monthly", "plan", "membership"},
	},
	{
		Title:    "Ultimate plan",
		Summary:  "Ultimate Plan is $29.99/month, allows up to 200 resumes monthly, and is tailored for power users or agencies that handle many applications.",
		Keywords: []string{"ultimate", "agency", "200", "bulk", "enterprise"},
	},
	{
		Title:    "Support",
		Summary:  "For billing questions, upgrades, or technical help, email hihired_support@tactechs.net or use the in-app Help bubble—responses typically arrive within one business day.",
		Keywords: []string{"support", "contact", "help", "email", "issue"},
	},
	{
		Title:    "Membership upgrade steps",
		Summary:  "To upgrade: log in, open Pricing, choose Pro or Ultimate, and complete checkout. Accounts update instantly and resume limits reset according to the plan period.",
		Keywords: []string{"upgrade", "change", "membership", "checkout", "limit"},
	},
	{
		Title:    "Key AI features",
		Summary:  "HiHired offers AI-generated bullet points, keyword optimization using job descriptions, instant grammar improvements, and professional PDF or DOCX exports.",
		Keywords: []string{"features", "ai", "optimize", "keyword", "grammar", "pdf"},
	},
	{
		Title:    "Resume workflow",
		Summary:  "To generate a resume: click 'Create my resume', choose Import or Start from scratch, paste the job description into the Match panel so AI rewrites bullet points, review/edit sections, then download the PDF or DOCX.",
		Keywords: []string{"steps", "workflow", "generate", "job description", "create", "resume", "import"},
	},
	{
		Title:    "FAQ highlights",
		Summary:  "You can build resumes without creating an account; saved data stays tied to your login; upgrades apply instantly after checkout; reach hihired_support@tactechs.net for any additional questions.",
		Keywords: []string{"faq", "question", "signup", "account", "data", "privacy"},
	},
}

var chatHistoryModel *models.ChatHistoryModel

// SetChatHistoryModel injects the persistence layer for chat transcripts.
func SetChatHistoryModel(m *models.ChatHistoryModel) {
	chatHistoryModel = m
}

func findRelevantKnowledge(query string, history []chatMessage) []knowledgeEntry {
	q := strings.ToLower(query)
	var textPool []string
	textPool = append(textPool, q)
	for _, msg := range history {
		textPool = append(textPool, strings.ToLower(msg.Text))
	}

	type scoredEntry struct {
		knowledgeEntry
		score int
	}

	var scores []scoredEntry
	for _, entry := range assistantKnowledge {
		score := 0
		for _, text := range textPool {
			for _, kw := range entry.Keywords {
				if strings.Contains(text, kw) {
					score += 2
				}
			}
		}
		if score == 0 && entry.Title == "Platform overview" {
			score = 1
		}
		if score > 0 {
			scores = append(scores, scoredEntry{knowledgeEntry: entry, score: score})
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].Title < scores[j].Title
		}
		return scores[i].score > scores[j].score
	})

	maxFacts := 4
	if len(scores) < maxFacts {
		maxFacts = len(scores)
	}

	results := make([]knowledgeEntry, 0, maxFacts)
	for i := 0; i < maxFacts; i++ {
		results = append(results, scores[i].knowledgeEntry)
	}
	return results
}

func buildKnowledgeContext(entries []knowledgeEntry) string {
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	for _, entry := range entries {
		b.WriteString("- ")
		b.WriteString(entry.Summary)
		b.WriteString("\n")
	}
	return b.String()
}

func buildResumeContext(resumeData map[string]interface{}) string {
	if len(resumeData) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("User's current resume data:\n")

	// Personal details
	if name, ok := resumeData["name"].(string); ok && strings.TrimSpace(name) != "" {
		b.WriteString(fmt.Sprintf("- Name: %s\n", strings.TrimSpace(name)))
	}
	if email, ok := resumeData["email"].(string); ok && strings.TrimSpace(email) != "" {
		b.WriteString(fmt.Sprintf("- Email: %s\n", strings.TrimSpace(email)))
	}
	if phone, ok := resumeData["phone"].(string); ok && strings.TrimSpace(phone) != "" {
		b.WriteString(fmt.Sprintf("- Phone: %s\n", strings.TrimSpace(phone)))
	}

	// Summary
	if summary, ok := resumeData["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		b.WriteString(fmt.Sprintf("- Summary: %s\n", strings.TrimSpace(summary)))
	}

	// Skills
	if skills, ok := resumeData["skills"].(string); ok && strings.TrimSpace(skills) != "" {
		b.WriteString(fmt.Sprintf("- Skills: %s\n", strings.TrimSpace(skills)))
	}

	// Experiences
	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		b.WriteString("- Work Experience:\n")
		for i, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				jobTitle, _ := expMap["jobTitle"].(string)
				company, _ := expMap["company"].(string)
				if jobTitle != "" || company != "" {
					b.WriteString(fmt.Sprintf("  %d. %s at %s\n", i+1, strings.TrimSpace(jobTitle), strings.TrimSpace(company)))
				}
			}
		}
	}

	// Projects
	if projects, ok := resumeData["projects"].([]interface{}); ok && len(projects) > 0 {
		b.WriteString("- Projects:\n")
		for i, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				projectName, _ := projMap["projectName"].(string)
				if projectName != "" {
					b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, strings.TrimSpace(projectName)))
				}
			}
		}
	}

	// Education
	if education, ok := resumeData["education"].([]interface{}); ok && len(education) > 0 {
		b.WriteString("- Education:\n")
		for i, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				degree, _ := eduMap["degree"].(string)
				school, _ := eduMap["school"].(string)
				field, _ := eduMap["field"].(string)
				if degree != "" || school != "" {
					entry := strings.TrimSpace(degree)
					if field != "" {
						entry += " in " + strings.TrimSpace(field)
					}
					if school != "" {
						entry += " from " + strings.TrimSpace(school)
					}
					b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, entry))
				}
			}
		}
	}

	result := b.String()
	if result == "User's current resume data:\n" {
		return ""
	}
	return result
}

func ChatAssistant(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	userMessage := strings.TrimSpace(req.Message)
	if userMessage == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message cannot be empty"})
		return
	}

	sessionID := ensureSessionID(req.SessionID)
	pagePath := strings.TrimSpace(req.PagePath)
	userEmail := strings.TrimSpace(req.UserEmail)
	ctx := c.Request.Context()

	// Check if this is a polish/optimize request
	log.Printf("[DEBUG] Polish check: polishAgent=%v, resumeDataLen=%d, message=%q", polishAgent != nil, len(req.ResumeData), userMessage)
	if polishAgent != nil && len(req.ResumeData) > 0 {
		log.Printf("[DEBUG] Attempting polish request handling")
		if polishResponse := tryHandlePolishRequest(ctx, userMessage, req.ResumeData, req.History); polishResponse != nil {
			log.Printf("[DEBUG] Polish request handled successfully: %s", polishResponse.Reply)
			// Persist chat exchange
			if err := persistChatExchange(ctx, chatPersistencePayload{
				sessionID:  sessionID,
				pagePath:   pagePath,
				userEmail:  userEmail,
				userInput:  userMessage,
				assistant:  polishResponse.Reply,
				historyLen: len(req.History),
			}); err != nil {
				log.Printf("failed to persist chat exchange: %v", err)
			}
			c.JSON(http.StatusOK, polishResponse)
			return
		}
	}

	const systemInstructions = `You are HiHired's AI resume assistant. Keep answers short, clear, and friendly (120 words max).
Focus on HiHired features: AI resume builder, templates, PDF export, memberships, workflow steps, and support.
When pricing is mentioned, explicitly list the Free, Premium, and Ultimate plans with their benefits.
When asked how to use the builder, provide step-by-step instructions.
When the user asks about their resume data (like "what is my name", "what did I enter", "what are my skills"), refer to the user's current resume data provided below and answer accurately.
If the question is outside scope, briefly say you can only help with HiHired resumes and suggest contacting us via the Help bubble or at hihired_support@tactechs.net.`

	var historyBuilder strings.Builder
	maxHistory := 6
	history := req.History
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	titleCaser := cases.Title(language.English)
	for _, msg := range history {
		role := strings.ToLower(msg.Role)
		if role != "assistant" && role != "bot" {
			role = "user"
		}
		roleTitle := titleCaser.String(role)
		fmt.Fprintf(&historyBuilder, "%s: %s\n", roleTitle, strings.TrimSpace(msg.Text))
	}

	knowledgeContext := buildKnowledgeContext(findRelevantKnowledge(userMessage, history))
	resumeContext := buildResumeContext(req.ResumeData)

	var prompt string
	if resumeContext != "" {
		prompt = fmt.Sprintf("%s\n\nAuthoritative product facts:\n%s\n%s\nConversation so far:\n%s\nUser: %s\n\nAnswer using the confirmed HiHired information and user's resume data above. If the user asks about their resume data, refer to it accurately.",
			systemInstructions, knowledgeContext, resumeContext, historyBuilder.String(), userMessage)
	} else {
		prompt = fmt.Sprintf("%s\n\nAuthoritative product facts:\n%s\nConversation so far:\n%s\nUser: %s\n\nAnswer using only the confirmed HiHired information above. If unsure, say you will connect them with support.",
			systemInstructions, knowledgeContext, historyBuilder.String(), userMessage)
	}

	reply, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cleaned := strings.TrimSpace(reply)
	if cleaned == "" {
		cleaned = "I'm still learning. Please contact us via the Help bubble or at hihired_support@tactechs.net and our team will help you right away."
	}

	if err := persistChatExchange(ctx, chatPersistencePayload{
		sessionID:  sessionID,
		pagePath:   pagePath,
		userEmail:  userEmail,
		userInput:  userMessage,
		assistant:  cleaned,
		historyLen: len(history),
	}); err != nil {
		log.Printf("failed to persist chat exchange: %v", err)
	}

	c.JSON(http.StatusOK, chatResponse{Reply: cleaned})
}

// tryHandlePolishRequest checks if the message is a polish request and handles it.
// Returns nil if not a polish request, allowing normal chat flow to continue.
// Simplified approach: uses existing resumeData directly, polishes all entries if none specified.
func tryHandlePolishRequest(ctx context.Context, message string, resumeData map[string]interface{}, history []chatMessage) *chatResponse {
	msgLower := strings.ToLower(message)

	// Quick keyword check to avoid unnecessary AI calls
	polishKeywords := []string{"polish", "improve", "enhance", "optimize", "rewrite", "refine", "make better", "fix"}
	hasPolishKeyword := false
	for _, kw := range polishKeywords {
		if strings.Contains(msgLower, kw) {
			hasPolishKeyword = true
			break
		}
	}
	if !hasPolishKeyword {
		log.Printf("[DEBUG] No polish keyword found in message")
		return nil
	}

	log.Printf("[DEBUG] Polish keyword detected, processing...")
	log.Printf("[DEBUG] ResumeData keys: %v", getMapKeys(resumeData))

	// Use the PolishAgent to detect intent
	intent, err := polishAgent.DetectPolishIntent(ctx, message, resumeData)
	if err != nil {
		log.Printf("[DEBUG] polish intent detection failed: %v", err)
		// Even if LLM fails, try to detect section from keywords
		intent = detectSectionFromKeywords(msgLower, resumeData)
	}

	log.Printf("[DEBUG] Intent detected: section=%s, identifier=%s, entryIndex=%d", intent.Section, intent.Identifier, intent.EntryIndex)

	// Force polish if keywords detected - don't rely on LLM's IsPolishRequest
	// The LLM may incorrectly return false or ask for clarification
	intent.IsPolishRequest = true // Always true if we have polish keywords

	// Always try keyword-based detection as fallback/override
	if intent.Section == "" || intent.Section == "none" {
		detected := detectSectionFromKeywords(msgLower, resumeData)
		intent.Section = detected.Section
		intent.Identifier = detected.Identifier
		intent.EntryIndex = detected.EntryIndex
		log.Printf("[DEBUG] Keyword fallback: section=%s, identifier=%s, entryIndex=%d", intent.Section, intent.Identifier, intent.EntryIndex)
	}

	// Force clear any clarification flags - we never ask for clarification
	intent.NeedsClarification = false
	intent.ClarificationQuestion = ""
	intent.Message = "" // Clear any clarification message from LLM
	log.Printf("[DEBUG] Final intent: section=%s, proceeding to polish", intent.Section)

	// Get job description from resume data if available
	jobDesc, _ := resumeData["jobDescription"].(string)

	// Create updated data copy
	updatedData := make(map[string]interface{})
	for k, v := range resumeData {
		updatedData[k] = v
	}

	var reply string
	var polishedContent interface{}
	var entryIndex int = -1

	// If identifier provided, try to find the specific entry
	if intent.Identifier != "" && intent.EntryIndex < 0 {
		intent.EntryIndex = findEntryIndexByIdentifier(resumeData, intent.Section, intent.Identifier)
	}

	switch intent.Section {
	case "experience":
		polishedContent, reply, entryIndex, err = polishExperiencesInChat(ctx, intent, resumeData, jobDesc, updatedData)
	case "education":
		polishedContent, reply, entryIndex, err = polishEducationInChat(ctx, intent, resumeData, updatedData)
	case "projects":
		polishedContent, reply, entryIndex, err = polishProjectsInChat(ctx, intent, resumeData, jobDesc, updatedData)
	case "summary":
		summary, _ := resumeData["summary"].(string)
		if summary == "" {
			return &chatResponse{Reply: "I don't see a professional summary to polish. Please add one first."}
		}
		polished, polishErr := polishAgent.PolishSummary(ctx, summary, resumeData)
		if polishErr != nil {
			err = polishErr
		} else {
			polishedContent = polished
			updatedData["summary"] = polished
			reply = "I've polished your professional summary to be more impactful and professional."
		}
	case "skills":
		skills, _ := resumeData["skills"].(string)
		if skills == "" {
			return &chatResponse{Reply: "I don't see any skills to polish. Please add some skills first."}
		}
		polished, polishErr := polishAgent.PolishSkills(ctx, skills, jobDesc)
		if polishErr != nil {
			err = polishErr
		} else {
			polishedContent = polished
			updatedData["skills"] = polished
			reply = "I've polished and organized your skills section."
		}
	case "all":
		reply, err = polishAllInChat(ctx, resumeData, jobDesc, updatedData)
		polishedContent = updatedData
	default:
		// Fallback: if section is unrecognized but we have polish keywords, polish everything
		log.Printf("Unrecognized section '%s', falling back to polish all", intent.Section)
		reply, err = polishAllInChat(ctx, resumeData, jobDesc, updatedData)
		polishedContent = updatedData
	}

	if err != nil {
		log.Printf("polish operation failed: %v", err)
		return &chatResponse{
			Reply: "I encountered an issue while polishing your content. Please try again.",
		}
	}

	return &chatResponse{
		Reply:             reply,
		IsPolishAction:    true,
		PolishedContent:   polishedContent,
		Section:           intent.Section,
		EntryIndex:        entryIndex,
		UpdatedResumeData: updatedData,
	}
}

// findEntryIndexByIdentifier searches for an entry matching the identifier in the given section.
func findEntryIndexByIdentifier(data map[string]interface{}, section, identifier string) int {
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

// detectSectionFromKeywords uses simple keyword matching to detect which section to polish.
// This is a fallback when LLM detection fails or returns unclear results.
func detectSectionFromKeywords(msgLower string, resumeData map[string]interface{}) services.PolishIntent {
	intent := services.PolishIntent{
		IsPolishRequest: true,
		EntryIndex:      -1,
	}

	// Check for section keywords
	if strings.Contains(msgLower, "experience") || strings.Contains(msgLower, "work") || strings.Contains(msgLower, "job") {
		intent.Section = "experience"
		// Try to find company name identifier from resume data
		if experiences, ok := resumeData["experiences"].([]interface{}); ok {
			for i, exp := range experiences {
				if expMap, ok := exp.(map[string]interface{}); ok {
					company, _ := expMap["company"].(string)
					if company != "" && strings.Contains(msgLower, strings.ToLower(company)) {
						intent.Identifier = company
						intent.EntryIndex = i
						break
					}
				}
			}
		}
	} else if strings.Contains(msgLower, "education") || strings.Contains(msgLower, "school") || strings.Contains(msgLower, "degree") {
		intent.Section = "education"
		if education, ok := resumeData["education"].([]interface{}); ok {
			for i, edu := range education {
				if eduMap, ok := edu.(map[string]interface{}); ok {
					school, _ := eduMap["school"].(string)
					if school != "" && strings.Contains(msgLower, strings.ToLower(school)) {
						intent.Identifier = school
						intent.EntryIndex = i
						break
					}
				}
			}
		}
	} else if strings.Contains(msgLower, "project") {
		intent.Section = "projects"
		if projects, ok := resumeData["projects"].([]interface{}); ok {
			for i, proj := range projects {
				if projMap, ok := proj.(map[string]interface{}); ok {
					name, _ := projMap["projectName"].(string)
					if name != "" && strings.Contains(msgLower, strings.ToLower(name)) {
						intent.Identifier = name
						intent.EntryIndex = i
						break
					}
				}
			}
		}
	} else if strings.Contains(msgLower, "summary") || strings.Contains(msgLower, "profile") || strings.Contains(msgLower, "objective") {
		intent.Section = "summary"
	} else if strings.Contains(msgLower, "skill") {
		intent.Section = "skills"
	} else if strings.Contains(msgLower, "resume") || strings.Contains(msgLower, "all") || strings.Contains(msgLower, "everything") {
		intent.Section = "all"
	} else {
		// Default to polishing all if no specific section mentioned
		intent.Section = "all"
	}

	return intent
}

func polishExperiencesInChat(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (interface{}, string, int, error) {
	experiences, ok := resumeData["experiences"].([]interface{})
	if !ok || len(experiences) == 0 {
		return nil, "I don't see any work experiences to polish.", -1, nil
	}

	// If specific entry identified
	if intent.EntryIndex >= 0 && intent.EntryIndex < len(experiences) {
		expMap, ok := experiences[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "I couldn't read that experience entry.", -1, nil
		}

		polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc)
		if err != nil {
			return nil, "", -1, err
		}

		company, _ := expMap["company"].(string)

		// Update the experiences array
		updatedExperiences := make([]interface{}, len(experiences))
		copy(updatedExperiences, experiences)
		updatedExperiences[intent.EntryIndex] = polished
		updatedData["experiences"] = updatedExperiences

		return polished, fmt.Sprintf("I've polished your experience at %s with impact-focused bullet points. The changes are ready to apply.", company), intent.EntryIndex, nil
	}

	// Polish all experiences (when user says "all" or doesn't specify)
	polishedExperiences := make([]interface{}, 0, len(experiences))
	for _, exp := range experiences {
		expMap, ok := exp.(map[string]interface{})
		if !ok {
			polishedExperiences = append(polishedExperiences, exp)
			continue
		}

		polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc)
		if err != nil {
			polishedExperiences = append(polishedExperiences, exp)
			continue
		}
		polishedExperiences = append(polishedExperiences, polished)
	}

	updatedData["experiences"] = polishedExperiences
	return polishedExperiences, fmt.Sprintf("I've polished all %d of your work experiences with impact-focused bullet points. The changes are ready to apply.", len(experiences)), -1, nil
}

func polishEducationInChat(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, updatedData map[string]interface{}) (interface{}, string, int, error) {
	education, ok := resumeData["education"].([]interface{})
	if !ok || len(education) == 0 {
		return nil, "I don't see any education entries to polish.", -1, nil
	}

	if intent.EntryIndex >= 0 && intent.EntryIndex < len(education) {
		eduMap, ok := education[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "I couldn't read that education entry.", -1, nil
		}

		polished, err := polishAgent.PolishEducation(ctx, eduMap)
		if err != nil {
			return nil, "", -1, err
		}

		school, _ := eduMap["school"].(string)

		updatedEducation := make([]interface{}, len(education))
		copy(updatedEducation, education)
		updatedEducation[intent.EntryIndex] = polished
		updatedData["education"] = updatedEducation

		return polished, fmt.Sprintf("I've polished your education at %s. The changes are ready to apply.", school), intent.EntryIndex, nil
	}

	// Polish all education entries
	polishedEducation := make([]interface{}, 0, len(education))
	for _, edu := range education {
		eduMap, ok := edu.(map[string]interface{})
		if !ok {
			polishedEducation = append(polishedEducation, edu)
			continue
		}

		polished, err := polishAgent.PolishEducation(ctx, eduMap)
		if err != nil {
			polishedEducation = append(polishedEducation, edu)
			continue
		}
		polishedEducation = append(polishedEducation, polished)
	}

	updatedData["education"] = polishedEducation
	return polishedEducation, "I've polished all your education entries. The changes are ready to apply.", -1, nil
}

func polishProjectsInChat(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (interface{}, string, int, error) {
	projects, ok := resumeData["projects"].([]interface{})
	if !ok || len(projects) == 0 {
		return nil, "I don't see any projects to polish.", -1, nil
	}

	if intent.EntryIndex >= 0 && intent.EntryIndex < len(projects) {
		projMap, ok := projects[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "I couldn't read that project entry.", -1, nil
		}

		polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc)
		if err != nil {
			return nil, "", -1, err
		}

		projectName, _ := projMap["projectName"].(string)

		updatedProjects := make([]interface{}, len(projects))
		copy(updatedProjects, projects)
		updatedProjects[intent.EntryIndex] = polished
		updatedData["projects"] = updatedProjects

		return polished, fmt.Sprintf("I've polished your project '%s'. The changes are ready to apply.", projectName), intent.EntryIndex, nil
	}

	// Polish all projects
	polishedProjects := make([]interface{}, 0, len(projects))
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			polishedProjects = append(polishedProjects, proj)
			continue
		}

		polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc)
		if err != nil {
			polishedProjects = append(polishedProjects, proj)
			continue
		}
		polishedProjects = append(polishedProjects, polished)
	}

	updatedData["projects"] = polishedProjects
	return polishedProjects, fmt.Sprintf("I've polished all %d of your projects. The changes are ready to apply.", len(projects)), -1, nil
}

func polishAllInChat(ctx context.Context, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (string, error) {
	var polishedSections []string

	// Polish experiences
	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		polishedExperiences := make([]interface{}, 0, len(experiences))
		for _, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc)
				if err != nil {
					polishedExperiences = append(polishedExperiences, exp)
				} else {
					polishedExperiences = append(polishedExperiences, polished)
				}
			} else {
				polishedExperiences = append(polishedExperiences, exp)
			}
		}
		updatedData["experiences"] = polishedExperiences
		polishedSections = append(polishedSections, "work experiences")
	}

	// Polish education
	if education, ok := resumeData["education"].([]interface{}); ok && len(education) > 0 {
		polishedEducation := make([]interface{}, 0, len(education))
		for _, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishEducation(ctx, eduMap)
				if err != nil {
					polishedEducation = append(polishedEducation, edu)
				} else {
					polishedEducation = append(polishedEducation, polished)
				}
			} else {
				polishedEducation = append(polishedEducation, edu)
			}
		}
		updatedData["education"] = polishedEducation
		polishedSections = append(polishedSections, "education")
	}

	// Polish projects
	if projects, ok := resumeData["projects"].([]interface{}); ok && len(projects) > 0 {
		polishedProjects := make([]interface{}, 0, len(projects))
		for _, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc)
				if err != nil {
					polishedProjects = append(polishedProjects, proj)
				} else {
					polishedProjects = append(polishedProjects, polished)
				}
			} else {
				polishedProjects = append(polishedProjects, proj)
			}
		}
		updatedData["projects"] = polishedProjects
		polishedSections = append(polishedSections, "projects")
	}

	// Polish summary
	if summary, ok := resumeData["summary"].(string); ok && summary != "" {
		polished, err := polishAgent.PolishSummary(ctx, summary, resumeData)
		if err == nil {
			updatedData["summary"] = polished
			polishedSections = append(polishedSections, "summary")
		}
	}

	// Polish skills
	if skills, ok := resumeData["skills"].(string); ok && skills != "" {
		polished, err := polishAgent.PolishSkills(ctx, skills, jobDesc)
		if err == nil {
			updatedData["skills"] = polished
			polishedSections = append(polishedSections, "skills")
		}
	}

	if len(polishedSections) == 0 {
		return "I couldn't find any content to polish in your resume.", nil
	}

	return fmt.Sprintf("I've polished your %s. The changes are ready to apply to your resume.", strings.Join(polishedSections, ", ")), nil
}

type chatPersistencePayload struct {
	sessionID  string
	pagePath   string
	userEmail  string
	userInput  string
	assistant  string
	historyLen int
}

func persistChatExchange(ctx context.Context, payload chatPersistencePayload) error {
	if chatHistoryModel == nil {
		return nil
	}

	userMeta := map[string]interface{}{
		"history_length": payload.historyLen,
		"recorded_at":    time.Now().UTC().Format(time.RFC3339),
	}

	messages := []models.ChatMessageRecord{
		{
			SessionID: payload.sessionID,
			Role:      "user",
			Message:   payload.userInput,
			UserEmail: nullableSQLString(payload.userEmail),
			PagePath:  nullableSQLString(payload.pagePath),
			Metadata:  userMeta,
		},
		{
			SessionID: payload.sessionID,
			Role:      "assistant",
			Message:   payload.assistant,
			UserEmail: nullableSQLString(payload.userEmail),
			PagePath:  nullableSQLString(payload.pagePath),
			Metadata: map[string]interface{}{
				"history_length": payload.historyLen,
				"recorded_at":    time.Now().UTC().Format(time.RFC3339),
			},
		},
	}

	err := chatHistoryModel.SaveMessages(ctx, messages)
	return err
}

func ensureSessionID(sessionID string) string {
	s := strings.TrimSpace(sessionID)
	if s != "" {
		return s
	}
	return fmt.Sprintf("anon-%d", time.Now().UnixNano())
}

func nullableSQLString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.TrimSpace(value), Valid: true}
}

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
