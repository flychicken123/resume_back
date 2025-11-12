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
	Message   string        `json:"message" binding:"required"`
	History   []chatMessage `json:"history"`
	SessionID string        `json:"session_id"`
	PagePath  string        `json:"page_path"`
	UserEmail string        `json:"user_email"`
}

type chatResponse struct {
	Reply string `json:"reply"`
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

	const systemInstructions = `You are HiHired's AI resume assistant. Keep answers short, clear, and friendly (120 words max).
Focus on HiHired features: AI resume builder, templates, PDF export, memberships, workflow steps, and support.
When pricing is mentioned, explicitly list the Free, Premium, and Ultimate plans with their benefits.
When asked how to use the builder, provide step-by-step instructions.
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

	prompt := fmt.Sprintf("%s\n\nAuthoritative product facts:\n%s\nConversation so far:\n%s\nUser: %s\n\nAnswer using only the confirmed HiHired information above. If unsure, say you will connect them with support.",
		systemInstructions, knowledgeContext, historyBuilder.String(), userMessage)

	reply, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cleaned := strings.TrimSpace(reply)
	if cleaned == "" {
		cleaned = "I'm still learning. Please contact us via the Help bubble or at hihired_support@tactechs.net and our team will help you right away."
	}

	if err := persistChatExchange(c.Request.Context(), chatPersistencePayload{
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
