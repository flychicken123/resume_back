package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"resumeai/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Helper: set up a test router with the chat endpoint
// ---------------------------------------------------------------------------

func setupChatTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/assistant/chat", ChatAssistant)
	return r
}

func makeChatRequest(t *testing.T, router *gin.Engine, body chatRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	assert.NoError(t, err)
	req, err := http.NewRequest("POST", "/api/assistant/chat", bytes.NewBuffer(b))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func sampleResumeData() map[string]interface{} {
	return map[string]interface{}{
		"name":    "John Doe",
		"email":   "john@example.com",
		"phone":   "555-1234",
		"summary": "Experienced software engineer with 5 years of backend development.",
		"skills":  "Go, Python, AWS, Docker, Kubernetes",
		"jobDescription": "We are looking for a senior backend engineer with experience in Go and cloud infrastructure.",
		"experiences": []interface{}{
			map[string]interface{}{
				"jobTitle":    "Software Engineer",
				"company":     "TechCorp",
				"description": "Built microservices using Go and deployed on AWS.",
			},
		},
		"projects": []interface{}{
			map[string]interface{}{
				"projectName": "API Gateway",
				"description": "Designed and built a high-performance API gateway.",
			},
		},
		"education": []interface{}{
			map[string]interface{}{
				"school": "MIT",
				"degree": "BS Computer Science",
			},
		},
	}
}

// ===========================================================================
// Test: classifyIntent
// ===========================================================================

func TestClassifyIntent_CoverLetter(t *testing.T) {
	// Test that messages about cover letters are classified as cover_letter intent
	messages := []string{
		"Generate a cover letter for me",
		"Can you write a cover letter?",
		"I need a cover letter for Google",
		"Write me a cover letter for the software engineer position",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			// classifyIntent should return intent=cover_letter with confidence >= 0.7
			// This test will call the actual Gemini Flash API
			// Skip if no API key is set
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentCoverLetter, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_RecommendationLetter(t *testing.T) {
	messages := []string{
		"Write a recommendation letter for me",
		"I need a third-party reference letter",
		"Generate a recommendation letter",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentRecommendationLetter, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_ResumeAdvice(t *testing.T) {
	messages := []string{
		"Analyze my resume and give feedback",
		"What should I improve on my resume?",
		"Give me resume advice",
		"Review my resume",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentResumeAdvice, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_GenerateSummary(t *testing.T) {
	messages := []string{
		"Write me a professional summary",
		"Generate a summary for my resume",
		"Create a summary section",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentGenerateSummary, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_OptimizeExperience(t *testing.T) {
	messages := []string{
		"Optimize my experience for this job",
		"Tailor my work experience to the job description",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentOptimizeExperience, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_ImproveGrammar(t *testing.T) {
	messages := []string{
		"Fix grammar in my experience section",
		"Improve the grammar in my summary",
		"Check my experience for grammar errors",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentImproveGrammar, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_GenerateSkills(t *testing.T) {
	messages := []string{
		"Generate skills from my resume",
		"What skills should I add?",
		"Suggest skills for me",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentGenerateSkills, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_CategorizeSkills(t *testing.T) {
	messages := []string{
		"Categorize my skills",
		"Organize my skills by category",
		"Group my skills into sections",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentCategorizeSkills, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_OptimizeProject(t *testing.T) {
	messages := []string{
		"Optimize my project descriptions",
		"Improve my projects for this job",
		"Tailor my project section",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentOptimizeProject, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_Polish(t *testing.T) {
	messages := []string{
		"Polish my experience section",
		"Enhance my resume content",
		"Refine my education entries",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentPolish, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}

func TestClassifyIntent_GeneralChat(t *testing.T) {
	messages := []string{
		"What is HiHired?",
		"How much does the premium plan cost?",
		"How do I contact support?",
		"Hello, who are you?",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentGeneralChat, intent.Intent)
		})
	}
}

func TestClassifyIntent_WithConversationHistory(t *testing.T) {
	// Test that "yes, do it" after discussing cover letters maps to cover_letter
	history := []chatMessage{
		{Role: "user", Text: "Can you generate a cover letter?"},
		{Role: "assistant", Text: "Sure! I can generate a cover letter for you. Would you like me to proceed?"},
	}
	intent, err := classifyIntent(t.Context(), "Yes, do it", sampleResumeData(), history)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.Equal(t, IntentCoverLetter, intent.Intent)
	assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
}

func TestClassifyIntent_ExtractsCompanyName(t *testing.T) {
	intent, err := classifyIntent(t.Context(), "Write a cover letter for Google", sampleResumeData(), nil)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.Equal(t, IntentCoverLetter, intent.Intent)
	assert.Equal(t, "Google", intent.ExtractedParams["companyName"])
}

// ===========================================================================
// Test: checkMissingParams
// ===========================================================================

func TestCheckMissingParams_CoverLetterWithJobDesc(t *testing.T) {
	intent := ChatIntent{Intent: IntentCoverLetter}
	resumeData := sampleResumeData() // has jobDescription
	missing := checkMissingParams(intent, resumeData)
	assert.Empty(t, missing, "should have no missing params when jobDescription is present")
}

func TestCheckMissingParams_CoverLetterWithoutJobDesc(t *testing.T) {
	// Cover letter should NOT require jobDescription — BuildCoverLetterPrompt handles empty JD
	intent := ChatIntent{Intent: IntentCoverLetter}
	resumeData := sampleResumeData()
	delete(resumeData, "jobDescription")
	missing := checkMissingParams(intent, resumeData)
	assert.Empty(t, missing, "cover letter should not require jobDescription")
}

func TestCheckMissingParams_OptimizeExperienceWithoutJobDesc(t *testing.T) {
	intent := ChatIntent{Intent: IntentOptimizeExperience}
	resumeData := sampleResumeData()
	delete(resumeData, "jobDescription")
	missing := checkMissingParams(intent, resumeData)
	assert.Contains(t, missing, "jobDescription")
}

func TestCheckMissingParams_OptimizeExperienceWithoutExperiences(t *testing.T) {
	intent := ChatIntent{Intent: IntentOptimizeExperience}
	resumeData := sampleResumeData()
	delete(resumeData, "experiences")
	missing := checkMissingParams(intent, resumeData)
	assert.Contains(t, missing, "experiences")
}

func TestCheckMissingParams_GenerateSummaryWithExperience(t *testing.T) {
	intent := ChatIntent{Intent: IntentGenerateSummary}
	resumeData := sampleResumeData()
	missing := checkMissingParams(intent, resumeData)
	assert.Empty(t, missing)
}

func TestCheckMissingParams_GenerateSummaryEmpty(t *testing.T) {
	intent := ChatIntent{Intent: IntentGenerateSummary}
	resumeData := map[string]interface{}{}
	missing := checkMissingParams(intent, resumeData)
	assert.NotEmpty(t, missing, "should require at least some resume data to generate summary")
}

func TestCheckMissingParams_ResumeAdviceNoRequirements(t *testing.T) {
	intent := ChatIntent{Intent: IntentResumeAdvice}
	resumeData := sampleResumeData()
	missing := checkMissingParams(intent, resumeData)
	assert.Empty(t, missing, "resume advice only needs resumeData which is present")
}

func TestCheckMissingParams_CategorizeSkillsWithoutSkills(t *testing.T) {
	intent := ChatIntent{Intent: IntentCategorizeSkills}
	resumeData := sampleResumeData()
	delete(resumeData, "skills")
	missing := checkMissingParams(intent, resumeData)
	assert.Contains(t, missing, "skills")
}

func TestCheckMissingParams_ImproveGrammarWithoutSection(t *testing.T) {
	intent := ChatIntent{
		Intent:          IntentImproveGrammar,
		ExtractedParams: map[string]string{"section": "experience"},
	}
	resumeData := sampleResumeData()
	delete(resumeData, "experiences")
	missing := checkMissingParams(intent, resumeData)
	assert.Contains(t, missing, "experiences")
}

// ===========================================================================
// Test: buildMissingParamResponse
// ===========================================================================

func TestBuildMissingParamResponse_CoverLetter(t *testing.T) {
	intent := ChatIntent{Intent: IntentCoverLetter}
	resp := buildMissingParamResponse(intent)
	assert.NotNil(t, resp)
	assert.Contains(t, resp.Reply, "job description")
	assert.Empty(t, resp.FeatureAction, "should not set featureAction when asking for missing params")
}

func TestBuildMissingParamResponse_OptimizeExperience(t *testing.T) {
	intent := ChatIntent{Intent: IntentOptimizeExperience}
	resp := buildMissingParamResponse(intent)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Reply)
}

func TestBuildMissingParamResponse_GenerateSkills(t *testing.T) {
	intent := ChatIntent{Intent: IntentGenerateSkills}
	resp := buildMissingParamResponse(intent)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Reply)
}

// ===========================================================================
// Test: routeToFeature
// ===========================================================================

func TestRouteToFeature_CoverLetter(t *testing.T) {
	intent := ChatIntent{
		Intent:          IntentCoverLetter,
		Confidence:      0.95,
		ExtractedParams: map[string]string{"companyName": "Google"},
	}
	req := chatRequest{
		Message:    "Generate a cover letter for Google",
		ResumeData: sampleResumeData(),
	}

	resp, err := routeToFeature(t.Context(), intent, req)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.NotNil(t, resp)
	assert.Equal(t, IntentCoverLetter, resp.FeatureAction)
	assert.NotEmpty(t, resp.Reply)
	assert.NotNil(t, resp.FeatureResult)
}

func TestRouteToFeature_ResumeAdvice(t *testing.T) {
	intent := ChatIntent{
		Intent:     IntentResumeAdvice,
		Confidence: 0.9,
	}
	req := chatRequest{
		Message:    "Analyze my resume",
		ResumeData: sampleResumeData(),
	}

	resp, err := routeToFeature(t.Context(), intent, req)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.NotNil(t, resp)
	assert.Equal(t, IntentResumeAdvice, resp.FeatureAction)
	assert.NotEmpty(t, resp.Reply)
}

func TestRouteToFeature_GenerateSummary(t *testing.T) {
	intent := ChatIntent{
		Intent:     IntentGenerateSummary,
		Confidence: 0.85,
	}
	req := chatRequest{
		Message:    "Write me a professional summary",
		ResumeData: sampleResumeData(),
	}

	resp, err := routeToFeature(t.Context(), intent, req)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.NotNil(t, resp)
	assert.Equal(t, IntentGenerateSummary, resp.FeatureAction)
	assert.NotEmpty(t, resp.Reply)
}

func TestRouteToFeature_GenerateSkills(t *testing.T) {
	intent := ChatIntent{
		Intent:     IntentGenerateSkills,
		Confidence: 0.88,
	}
	req := chatRequest{
		Message:    "Generate skills for me",
		ResumeData: sampleResumeData(),
	}

	resp, err := routeToFeature(t.Context(), intent, req)
	if err != nil {
		t.Skipf("Skipping (API call failed): %v", err)
	}
	assert.NotNil(t, resp)
	assert.Equal(t, IntentGenerateSkills, resp.FeatureAction)
}

func TestRouteToFeature_UnsupportedIntent(t *testing.T) {
	intent := ChatIntent{
		Intent:     "unknown_intent",
		Confidence: 0.9,
	}
	req := chatRequest{
		Message:    "Do something weird",
		ResumeData: sampleResumeData(),
	}

	resp, err := routeToFeature(t.Context(), intent, req)
	assert.Error(t, err, "should return error for unsupported intent")
	assert.Nil(t, resp)
}

// ===========================================================================
// Test: Improved tryHandlePolishRequest fallback
// ===========================================================================

func TestPolishFallback_TightenedKeywords_NoFalsePositive(t *testing.T) {
	// "fix my login issue" should NOT trigger polish
	resp := tryHandlePolishRequest(t.Context(), "fix my login issue", sampleResumeData(), nil)
	assert.Nil(t, resp, "generic 'fix' should not trigger polish after keyword tightening")
}

func TestPolishFallback_TightenedKeywords_NoFalsePositive_Improve(t *testing.T) {
	// "improve my chances" should NOT trigger polish
	resp := tryHandlePolishRequest(t.Context(), "improve my chances of getting hired", sampleResumeData(), nil)
	assert.Nil(t, resp, "generic 'improve' should not trigger polish after keyword tightening")
}

func TestPolishFallback_TightenedKeywords_StillMatchesPolish(t *testing.T) {
	// "polish my experience" SHOULD still trigger polish
	resp := tryHandlePolishRequest(t.Context(), "polish my experience", sampleResumeData(), nil)
	// May be nil if polishAgent is not initialized in test, but the keyword check should pass
	// We're testing that the keyword filter doesn't reject it
	if polishAgent == nil {
		t.Skip("polishAgent not initialized in test environment")
	}
	assert.NotNil(t, resp)
}

func TestPolishFallback_EmptyResumeData(t *testing.T) {
	// Should return nil when resumeData is empty (self-contained guard)
	resp := tryHandlePolishRequest(t.Context(), "polish my experience", map[string]interface{}{}, nil)
	assert.Nil(t, resp, "should return nil when resumeData is empty")
}

func TestPolishFallback_NilResumeData(t *testing.T) {
	resp := tryHandlePolishRequest(t.Context(), "polish my experience", nil, nil)
	assert.Nil(t, resp, "should return nil when resumeData is nil")
}

func TestPolishFallback_UnrecognizedSection_AsksUser(t *testing.T) {
	if polishAgent == nil {
		t.Skip("polishAgent not initialized in test environment")
	}
	// When the section is unrecognized, should ask which section instead of polishing all
	resp := tryHandlePolishRequest(t.Context(), "polish my stuff", sampleResumeData(), nil)
	if resp != nil {
		// Should ask the user which section, not silently polish everything
		assert.Contains(t, resp.Reply, "which section",
			"should ask which section to polish instead of defaulting to polish all")
		assert.False(t, resp.IsPolishAction,
			"should not set IsPolishAction when asking for clarification")
	}
}

// ===========================================================================
// Test: Full ChatAssistant flow via HTTP (integration-style)
// ===========================================================================

func TestChatAssistant_EmptyMessage(t *testing.T) {
	router := setupChatTestRouter()
	w := makeChatRequest(t, router, chatRequest{Message: ""})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChatAssistant_InvalidJSON(t *testing.T) {
	router := setupChatTestRouter()
	req, _ := http.NewRequest("POST", "/api/assistant/chat", bytes.NewBuffer([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChatAssistant_GeneralChat_ReturnsReply(t *testing.T) {
	router := setupChatTestRouter()
	w := makeChatRequest(t, router, chatRequest{
		Message:   "What is HiHired?",
		SessionID: "test-session",
	})
	// Should succeed (may fail if no API key, that's ok)
	if w.Code != http.StatusOK {
		t.Skipf("Skipping (API not available): status=%d", w.Code)
	}
	var resp chatResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Reply)
	assert.Empty(t, resp.FeatureAction, "general chat should not have featureAction")
}

func TestChatAssistant_FeatureRoute_CoverLetter(t *testing.T) {
	router := setupChatTestRouter()
	w := makeChatRequest(t, router, chatRequest{
		Message:    "Generate a cover letter for me",
		SessionID:  "test-session",
		ResumeData: sampleResumeData(),
	})
	if w.Code != http.StatusOK {
		t.Skipf("Skipping (API not available): status=%d", w.Code)
	}
	var resp chatResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, IntentCoverLetter, resp.FeatureAction)
	assert.NotEmpty(t, resp.Reply)
	assert.NotNil(t, resp.FeatureResult)
}

func TestChatAssistant_FeatureRoute_MissingJobDesc(t *testing.T) {
	router := setupChatTestRouter()
	resumeData := sampleResumeData()
	delete(resumeData, "jobDescription")
	w := makeChatRequest(t, router, chatRequest{
		Message:    "Generate a cover letter for me",
		SessionID:  "test-session",
		ResumeData: resumeData,
	})
	if w.Code != http.StatusOK {
		t.Skipf("Skipping (API not available): status=%d", w.Code)
	}
	var resp chatResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Contains(t, resp.Reply, "job description",
		"should ask user to provide job description")
}

// ===========================================================================
// Test: Job Application Query
// ===========================================================================

func TestBuildJobApplicationContext_MultipleApps(t *testing.T) {
	apps := []*models.JobApplication{
		{
			JobTitle:    "Software Engineer",
			CompanyName: "Google",
			Status:      "interviewing",
			AppliedAt:   time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			JobURL:      "https://careers.google.com/jobs/123",
			JobLocation: "Mountain View, CA",
		},
		{
			JobTitle:    "Data Analyst",
			CompanyName: "Meta",
			Status:      "applied",
			AppliedAt:   time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
			Notes:       "Referred by John",
		},
	}
	byStatus := map[string]int{"applied": 1, "interviewing": 1}
	got := buildJobApplicationContext(apps, byStatus, 2)

	assert.Contains(t, got, "Total: 2")
	assert.Contains(t, got, "Google")
	assert.Contains(t, got, "Meta")
	assert.Contains(t, got, "interviewing")
	assert.Contains(t, got, "applied")
	assert.Contains(t, got, "Mar 5, 2026")
	assert.Contains(t, got, "Mar 10, 2026")
	assert.Contains(t, got, "https://careers.google.com/jobs/123")
	assert.Contains(t, got, "Mountain View, CA")
	assert.Contains(t, got, "Referred by John")
}

func TestBuildJobApplicationContext_Empty(t *testing.T) {
	got := buildJobApplicationContext(nil, map[string]int{}, 0)
	assert.Contains(t, got, "Total: 0")
	assert.Contains(t, got, "No applications tracked yet")
}

func TestHandleJobApplicationQuery_NoEmail(t *testing.T) {
	req := chatRequest{Message: "How many jobs did I apply to?", UserEmail: ""}
	resp := handleJobApplicationQuery(req)
	assert.NotNil(t, resp)
	assert.Contains(t, resp.Reply, "log in")
	assert.Equal(t, IntentJobApplicationQuery, resp.FeatureAction)
}

func TestHandleJobApplicationQuery_ModelsNotInitialised(t *testing.T) {
	// Save and restore original values
	origUser := chatUserModel
	origApp := chatJobAppModel
	defer func() {
		chatUserModel = origUser
		chatJobAppModel = origApp
	}()

	chatUserModel = nil
	chatJobAppModel = nil

	req := chatRequest{Message: "How many jobs did I apply to?", UserEmail: "test@example.com"}
	resp := handleJobApplicationQuery(req)
	assert.NotNil(t, resp)
	assert.Contains(t, resp.Reply, "unavailable")
	assert.Equal(t, IntentJobApplicationQuery, resp.FeatureAction)
}

// ===========================================================================
// Test: buildStaleAppReminder
// ===========================================================================

func TestBuildStaleAppReminder_Empty(t *testing.T) {
	result := buildStaleAppReminder(nil)
	assert.Empty(t, result)

	result = buildStaleAppReminder([]*models.JobApplication{})
	assert.Empty(t, result)
}

func TestBuildStaleAppReminder_FewApps(t *testing.T) {
	now := time.Now()
	apps := []*models.JobApplication{
		{
			JobTitle:        "Software Engineer",
			CompanyName:     "Google",
			Status:          "screening",
			StatusUpdatedAt: now.Add(-10 * 24 * time.Hour),
		},
		{
			JobTitle:        "Data Analyst",
			CompanyName:     "Meta",
			Status:          "applied",
			StatusUpdatedAt: now.Add(-8 * 24 * time.Hour),
		},
	}
	result := buildStaleAppReminder(apps)

	assert.Contains(t, result, "2 application(s)")
	assert.Contains(t, result, "Google")
	assert.Contains(t, result, "Meta")
	assert.Contains(t, result, "10 days ago")
	assert.Contains(t, result, "8 days ago")
	assert.NotContains(t, result, "...and")
}

func TestBuildStaleAppReminder_MoreThanFive(t *testing.T) {
	now := time.Now()
	apps := make([]*models.JobApplication, 7)
	for i := range apps {
		apps[i] = &models.JobApplication{
			JobTitle:        "Role " + string(rune('A'+i)),
			CompanyName:     "Company " + string(rune('A'+i)),
			Status:          "applied",
			StatusUpdatedAt: now.Add(-time.Duration(14-i) * 24 * time.Hour),
		}
	}
	result := buildStaleAppReminder(apps)

	assert.Contains(t, result, "7 application(s)")
	assert.Contains(t, result, "...and 2 more")
}

func TestBuildStaleAppReminder_SingleApp(t *testing.T) {
	now := time.Now()
	apps := []*models.JobApplication{
		{
			JobTitle:        "Frontend Dev",
			CompanyName:     "Stripe",
			Status:          "interviewing",
			StatusUpdatedAt: now.Add(-9 * 24 * time.Hour),
		},
	}
	result := buildStaleAppReminder(apps)

	assert.Contains(t, result, "1 application(s)")
	assert.Contains(t, result, "Stripe")
	assert.Contains(t, result, "9 days ago")
}

func TestClassifyIntent_JobApplicationQuery(t *testing.T) {
	messages := []string{
		"How many jobs have I applied to?",
		"Have I applied to Google?",
		"What's the status of my applications?",
		"Show me my rejected applications",
	}
	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			intent, err := classifyIntent(t.Context(), msg, sampleResumeData(), nil)
			if err != nil {
				t.Skipf("Skipping (API call failed): %v", err)
			}
			assert.Equal(t, IntentJobApplicationQuery, intent.Intent)
			assert.GreaterOrEqual(t, intent.Confidence, intentConfidenceThreshold)
		})
	}
}
