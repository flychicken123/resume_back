package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"resumeai/models"
	"resumeai/utils"
)

// BenchmarkService runs automated AI quality benchmarks using LLM-as-judge.
type BenchmarkService struct {
	postings  *models.JobPostingModel
	matches   *models.ResumeJobMatchModel
	benchmark *models.AiBenchmarkModel
	logger    *utils.Logger
	mu        sync.Mutex
	running   bool
	progress  int
	total     int
	runID     string
	runType   string
}

// BenchmarkStatus represents the current state of a benchmark run.
type BenchmarkStatus struct {
	Running  bool   `json:"running"`
	RunID    string `json:"run_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Progress int    `json:"progress"`
	Total    int    `json:"total"`
}

// NewBenchmarkService creates a new BenchmarkService.
func NewBenchmarkService(postings *models.JobPostingModel, matches *models.ResumeJobMatchModel, benchmark *models.AiBenchmarkModel, logger *utils.Logger) *BenchmarkService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &BenchmarkService{
		postings:  postings,
		matches:   matches,
		benchmark: benchmark,
		logger:    logger,
	}
}

// Status returns the current benchmark run status.
func (s *BenchmarkService) Status() BenchmarkStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return BenchmarkStatus{
		Running:  s.running,
		RunID:    s.runID,
		Type:     s.runType,
		Progress: s.progress,
		Total:    s.total,
	}
}

// RunBenchmark runs a specific benchmark type in the background.
func (s *BenchmarkService) RunBenchmark(ctx context.Context, benchmarkType string, sampleSize int, userID int) (string, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return "", fmt.Errorf("benchmark already running")
	}
	if sampleSize <= 0 {
		sampleSize = 20
	}
	runID := fmt.Sprintf("bench_%s_%s", benchmarkType, time.Now().Format("20060102_150405"))
	s.running = true
	s.runID = runID
	s.runType = benchmarkType
	s.progress = 0
	s.total = sampleSize
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()

		switch benchmarkType {
		case "classification":
			s.runClassificationBenchmark(ctx, runID, sampleSize)
		case "skills":
			s.runSkillInferenceBenchmark(ctx, runID, sampleSize)
		case "intent":
			s.runIntentBenchmark(ctx, runID)
		case "matching":
			s.runMatchQualityBenchmark(ctx, runID, userID, sampleSize)
		case "fit_reasons":
			s.runFitReasonsBenchmark(ctx, runID, sampleSize)
		case "experience":
			s.runGenerationBenchmark(ctx, runID, "experience", sampleSize)
		case "summary":
			s.runGenerationBenchmark(ctx, runID, "summary", sampleSize)
		case "projects":
			s.runGenerationBenchmark(ctx, runID, "projects", sampleSize)
		case "chat":
			s.runChatBenchmark(ctx, runID)
		case "cover_letter":
			s.runGenerationBenchmark(ctx, runID, "cover_letter", sampleSize)
		case "all":
			s.RunAllBenchmarks(ctx, sampleSize)
		default:
			s.logger.Warn("unknown benchmark type", map[string]interface{}{"type": benchmarkType})
		}
	}()

	return runID, nil
}

// RunAllBenchmarks runs all benchmark types sequentially.
func (s *BenchmarkService) RunAllBenchmarks(ctx context.Context, sampleSize int) {
	ts := time.Now().Format("20060102_150405")

	s.runClassificationBenchmark(ctx, "bench_classification_"+ts, sampleSize)
	s.runSkillInferenceBenchmark(ctx, "bench_skills_"+ts, sampleSize)
	s.runIntentBenchmark(ctx, "bench_intent_"+ts)
	s.runMatchQualityBenchmark(ctx, "bench_matching_"+ts, 0, sampleSize)
	s.runFitReasonsBenchmark(ctx, "bench_fit_reasons_"+ts, sampleSize)
	s.runGenerationBenchmark(ctx, "bench_experience_"+ts, "experience", sampleSize)
	s.runGenerationBenchmark(ctx, "bench_summary_"+ts, "summary", sampleSize)
	s.runGenerationBenchmark(ctx, "bench_projects_"+ts, "projects", sampleSize)
	s.runChatBenchmark(ctx, "bench_chat_"+ts)
	s.runGenerationBenchmark(ctx, "bench_cover_letter_"+ts, "cover_letter", sampleSize)
}

// StartScheduler starts the weekly benchmark scheduler.
func (s *BenchmarkService) StartScheduler(ctx context.Context) {
	go func() {
		// Run on startup if no results from this week
		if !s.benchmark.HasResultsFromThisWeek() {
			s.logger.Info("benchmark scheduler: no results this week, running now", nil)
			s.RunAllBenchmarks(ctx, 20)
		}

		for {
			next := nextWeeklyRunTime(time.Sunday, 3, 0)
			s.logger.Info("benchmark scheduler: next run", map[string]interface{}{"at": next.Format(time.RFC3339)})
			select {
			case <-time.After(time.Until(next)):
				s.logger.Info("benchmark scheduler: starting weekly run", nil)
				s.RunAllBenchmarks(ctx, 20)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func nextWeeklyRunTime(day time.Weekday, hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	// Advance to the target weekday
	daysUntil := int(day) - int(now.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}
	next = next.AddDate(0, 0, daysUntil)
	if next.Before(now) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

// ---- Classification Benchmark ----

func (s *BenchmarkService) runClassificationBenchmark(ctx context.Context, runID string, sampleSize int) {
	s.logger.Info("benchmark: starting classification", map[string]interface{}{"run_id": runID, "sample_size": sampleSize})

	// Sample classified jobs stratified by career field
	rows, err := s.postings.DB().Query(`
		SELECT id, title, LEFT(description, 500), career_field, seniority, extracted_skills
		FROM job_postings
		WHERE career_field IS NOT NULL AND extracted_skills IS NOT NULL AND seniority IS NOT NULL
		ORDER BY RANDOM() LIMIT $1
	`, sampleSize)
	if err != nil {
		s.logger.Warn("benchmark: classification sample failed", map[string]interface{}{"error": err.Error()})
		return
	}
	defer rows.Close()

	type classifyJob struct {
		id          int64
		title       string
		description string
		careerField string
		seniority   string
		skills      string
	}

	var jobs []classifyJob
	for rows.Next() {
		var j classifyJob
		var skills []string
		if err := rows.Scan(&j.id, &j.title, &j.description, &j.careerField, &j.seniority, (*pq.StringArray)(&skills)); err != nil {
			continue
		}
		j.skills = strings.Join(skills, ", ")
		jobs = append(jobs, j)
	}

	s.mu.Lock()
	s.total = len(jobs)
	s.mu.Unlock()

	for i, job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		prompt := fmt.Sprintf(`You are an expert evaluator. Given this job posting, evaluate the AI's classification.

Job Title: %s
Job Description: %s

AI Output:
- career_field: %s
- seniority: %s
- extracted_skills: %s

Evaluate each field. Return JSON only:
{"career_field_correct": true/false, "seniority_correct": true/false, "skills_precision": 0.0-1.0, "reasoning": "brief explanation"}`,
			job.title, job.description, job.careerField, job.seniority, job.skills)

		raw, err := CallGeminiWithTemperature(prompt, 0.0)
		if err != nil {
			s.logger.Warn("benchmark: judge call failed", map[string]interface{}{"job_id": job.id, "error": err.Error()})
			continue
		}

		var verdict struct {
			CareerFieldCorrect bool    `json:"career_field_correct"`
			SeniorityCorrect   bool    `json:"seniority_correct"`
			SkillsPrecision    float64 `json:"skills_precision"`
			Reasoning          string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)

		if err := json.Unmarshal([]byte(cleaned), &verdict); err != nil {
			continue
		}

		entityID := job.id
		results := []models.AiBenchmarkResult{
			{RunID: runID, BenchmarkType: "classification", EntityID: &entityID, FieldName: "career_field", AiValue: job.careerField, IsCorrect: &verdict.CareerFieldCorrect, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: "classification", EntityID: &entityID, FieldName: "seniority", AiValue: job.seniority, IsCorrect: &verdict.SeniorityCorrect, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: "classification", EntityID: &entityID, FieldName: "skills_precision", AiValue: job.skills, Score: &verdict.SkillsPrecision, Reasoning: verdict.Reasoning},
		}
		_ = s.benchmark.InsertResults(results)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(200 * time.Millisecond)
	}
	s.logger.Info("benchmark: classification complete", map[string]interface{}{"run_id": runID, "processed": len(jobs)})
}

// ---- Skill Inference Benchmark ----

func (s *BenchmarkService) runSkillInferenceBenchmark(ctx context.Context, runID string, sampleSize int) {
	s.logger.Info("benchmark: starting skill inference", map[string]interface{}{"run_id": runID})

	rows, err := s.postings.DB().Query(`
		SELECT id, title, LEFT(description, 500), extracted_skills
		FROM job_postings
		WHERE extracted_skills IS NOT NULL AND array_length(extracted_skills, 1) > 0
		ORDER BY RANDOM() LIMIT $1
	`, sampleSize)
	if err != nil {
		return
	}
	defer rows.Close()

	type skillJob struct {
		id          int64
		title       string
		description string
		skills      string
	}

	var jobs []skillJob
	for rows.Next() {
		var j skillJob
		var skills []string
		if err := rows.Scan(&j.id, &j.title, &j.description, (*pq.StringArray)(&skills)); err != nil {
			continue
		}
		j.skills = strings.Join(skills, ", ")
		jobs = append(jobs, j)
	}

	for i, job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		prompt := fmt.Sprintf(`You are an expert evaluator. Given this job posting, evaluate if the extracted skills are accurate.

Job Title: %s
Job Description: %s

AI Extracted Skills: %s

Evaluate:
1. Precision: What percentage of listed skills are actually required/mentioned in the description?
2. Recall: Are any important skills missing?

Return JSON only:
{"precision": 0.0-1.0, "recall": 0.0-1.0, "missing_skills": "list any missing", "reasoning": "brief explanation"}`,
			job.title, job.description, job.skills)

		raw, err := CallGeminiWithTemperature(prompt, 0.0)
		if err != nil {
			continue
		}

		var verdict struct {
			Precision float64 `json:"precision"`
			Recall    float64 `json:"recall"`
			Reasoning string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &verdict); err != nil {
			continue
		}

		entityID := job.id
		results := []models.AiBenchmarkResult{
			{RunID: runID, BenchmarkType: "skills", EntityID: &entityID, FieldName: "precision", AiValue: job.skills, Score: &verdict.Precision, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: "skills", EntityID: &entityID, FieldName: "recall", Score: &verdict.Recall, Reasoning: verdict.Reasoning},
		}
		_ = s.benchmark.InsertResults(results)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(200 * time.Millisecond)
	}
}

// ---- Intent Classification Benchmark ----

var intentTestCases = []struct {
	Message  string
	Expected string
}{
	// optimize_experience
	{"optimize my experience", "optimize_experience"},
	{"rewrite my work history", "optimize_experience"},
	{"improve my experience bullets", "optimize_experience"},
	{"make my job descriptions better", "optimize_experience"},
	// generate_summary
	{"improve my summary", "generate_summary"},
	{"write a professional summary for me", "generate_summary"},
	{"generate a resume summary", "generate_summary"},
	// optimize_project
	{"rewrite my projects", "optimize_project"},
	{"improve my project descriptions", "optimize_project"},
	// cover_letter
	{"help me write a cover letter", "cover_letter"},
	{"generate a cover letter for this job", "cover_letter"},
	{"write a cover letter", "cover_letter"},
	// generate_skills
	{"what skills should I add", "generate_skills"},
	{"suggest skills for my resume", "generate_skills"},
	// categorize_skills
	{"organize my skills by category", "categorize_skills"},
	{"group my skills", "categorize_skills"},
	// job_application_query
	{"show me my applications", "job_application_query"},
	{"track my job application", "job_application_query"},
	{"what's the status of my applications", "job_application_query"},
	// general_chat
	{"how do I negotiate salary", "general_chat"},
	{"tell me a joke", "general_chat"},
	{"what is the weather", "general_chat"},
	{"how to prepare for interview", "general_chat"},
	{"tips for LinkedIn profile", "general_chat"},
	{"what is hihired pricing", "general_chat"},
	{"how do I use the builder", "general_chat"},
	// polish
	{"polish my resume", "polish"},
	{"fix my resume grammar and formatting", "polish"},
	// improve_grammar
	{"fix the grammar in my resume", "improve_grammar"},
	{"check my resume for typos", "improve_grammar"},
}

func (s *BenchmarkService) runIntentBenchmark(ctx context.Context, runID string) {
	s.logger.Info("benchmark: starting intent classification", map[string]interface{}{"run_id": runID})

	s.mu.Lock()
	s.total = len(intentTestCases)
	s.mu.Unlock()

	for i, tc := range intentTestCases {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Call the intent classifier directly
		prompt := BuildIntentClassificationPrompt(tc.Message, nil)
		raw, err := CallGeminiFlashWithTemperature(prompt, 0.0)
		if err != nil {
			continue
		}

		// Parse the intent from response
		detected := parseIntentFromResponse(raw)
		correct := detected == tc.Expected

		result := models.AiBenchmarkResult{
			RunID:         runID,
			BenchmarkType: "intent",
			FieldName:     "intent",
			AiValue:       detected,
			ExpectedValue: tc.Expected,
			IsCorrect:     &correct,
			Reasoning:     fmt.Sprintf("Message: %q → Detected: %s, Expected: %s", tc.Message, detected, tc.Expected),
		}
		_ = s.benchmark.InsertResult(result)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()
	}
}

// ---- Match Quality Benchmark ----

func (s *BenchmarkService) runMatchQualityBenchmark(ctx context.Context, runID string, userID int, sampleSize int) {
	s.logger.Info("benchmark: starting match quality", map[string]interface{}{"run_id": runID})

	// If no user specified, find one with matches
	if userID == 0 {
		_ = s.postings.DB().QueryRow(`
			SELECT user_id FROM resume_job_matches GROUP BY user_id ORDER BY COUNT(*) DESC LIMIT 1
		`).Scan(&userID)
		if userID == 0 {
			return
		}
	}

	matches, _, err := s.matches.ListMostRecentByUser(userID, sampleSize)
	if err != nil || len(matches) == 0 {
		return
	}

	s.mu.Lock()
	s.total = len(matches)
	s.mu.Unlock()

	for i, match := range matches {
		select {
		case <-ctx.Done():
			return
		default:
		}

		prompt := fmt.Sprintf(`You are an expert recruiter. Rate how relevant this job is for this candidate on a scale of 1-5.

Candidate Position: (user_id=%d)
Job Title: %s
Company: %s
Location: %s
Match Score: %.1f

Return JSON only:
{"relevance": 1-5, "reasoning": "brief explanation"}`,
			userID, match.JobTitle, func() string {
				if match.CompanyName != nil {
					return *match.CompanyName
				}
				return "Unknown"
			}(), match.JobLocation, match.MatchScore)

		raw, err := CallGeminiWithTemperature(prompt, 0.0)
		if err != nil {
			continue
		}

		var verdict struct {
			Relevance float64 `json:"relevance"`
			Reasoning string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &verdict); err != nil {
			continue
		}

		entityID := match.ID
		result := models.AiBenchmarkResult{
			RunID:         runID,
			BenchmarkType: "matching",
			EntityID:      &entityID,
			FieldName:     "relevance",
			AiValue:       match.JobTitle,
			Score:         &verdict.Relevance,
			Reasoning:     verdict.Reasoning,
		}
		_ = s.benchmark.InsertResult(result)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(200 * time.Millisecond)
	}
}

// ---- Fit Reasons Benchmark ----

func (s *BenchmarkService) runFitReasonsBenchmark(ctx context.Context, runID string, sampleSize int) {
	s.logger.Info("benchmark: starting fit reasons", map[string]interface{}{"run_id": runID})

	rows, err := s.postings.DB().Query(`
		SELECT m.id, m.job_posting_id, p.title, m.fit_reasons
		FROM resume_job_matches m
		JOIN job_postings p ON p.id = m.job_posting_id
		WHERE m.fit_reasons IS NOT NULL AND array_length(m.fit_reasons, 1) > 0
		ORDER BY RANDOM() LIMIT $1
	`, sampleSize)
	if err != nil {
		return
	}
	defer rows.Close()

	type fitMatch struct {
		matchID    int64
		jobID      int64
		title      string
		reasons    string
	}

	var matches []fitMatch
	for rows.Next() {
		var m fitMatch
		var reasons []string
		if err := rows.Scan(&m.matchID, &m.jobID, &m.title, (*pq.StringArray)(&reasons)); err != nil {
			continue
		}
		m.reasons = strings.Join(reasons, "\n- ")
		matches = append(matches, m)
	}

	for i, m := range matches {
		select {
		case <-ctx.Done():
			return
		default:
		}

		prompt := fmt.Sprintf(`Rate these AI-generated fit reasons for a job match.

Job: %s
Fit Reasons:
- %s

Rate each dimension 1-5:
- Specificity: Do reasons reference specific skills, companies, or technologies?
- Accuracy: Are the claims factually supported?
- Actionability: Do they give useful advice?

Return JSON only:
{"specificity": 1-5, "accuracy": 1-5, "actionability": 1-5, "reasoning": "brief explanation"}`,
			m.title, m.reasons)

		raw, err := CallGeminiWithTemperature(prompt, 0.0)
		if err != nil {
			continue
		}

		var verdict struct {
			Specificity   float64 `json:"specificity"`
			Accuracy      float64 `json:"accuracy"`
			Actionability float64 `json:"actionability"`
			Reasoning     string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &verdict); err != nil {
			continue
		}

		entityID := m.matchID
		results := []models.AiBenchmarkResult{
			{RunID: runID, BenchmarkType: "fit_reasons", EntityID: &entityID, FieldName: "specificity", Score: &verdict.Specificity, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: "fit_reasons", EntityID: &entityID, FieldName: "accuracy", Score: &verdict.Accuracy},
			{RunID: runID, BenchmarkType: "fit_reasons", EntityID: &entityID, FieldName: "actionability", Score: &verdict.Actionability},
		}
		_ = s.benchmark.InsertResults(results)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(200 * time.Millisecond)
	}
}

// ---- Generation Quality Benchmark (experience, summary, projects, cover_letter) ----

// Synthetic test data for generation benchmarks
var generationTestCases = []struct {
	Experience      string
	Education       string
	Skills          []string
	Summary         string
	JobDescription  string
	MatchedSkills   []string
	MissingSkills   []string
	ProjectName     string
	ProjectDesc     string
	ProjectTech     string
}{
	{
		Experience:     "Software Engineer at Google (2020-2024). Built microservices for ads platform. Improved latency by 40%. Led team of 4 engineers.",
		Education:      "B.S. Computer Science, Stanford University, 2020",
		Skills:         []string{"Go", "Python", "Kubernetes", "gRPC", "PostgreSQL", "Redis"},
		Summary:        "Software engineer with 4 years experience building distributed systems.",
		JobDescription: "Senior Software Engineer at Stripe. Build payment infrastructure using Go and Kubernetes. 5+ years experience required.",
		MatchedSkills:  []string{"Go", "Kubernetes", "PostgreSQL"},
		MissingSkills:  []string{"payments", "Ruby", "AWS"},
		ProjectName:    "Real-time Analytics Dashboard",
		ProjectDesc:    "Built a dashboard that shows real-time metrics for ad campaigns using React and WebSockets.",
		ProjectTech:    "React, WebSockets, D3.js, Node.js",
	},
	{
		Experience:     "Data Analyst at Amazon (2021-2024). Created sales forecasting models. Automated weekly reports saving 10 hours/week. Managed ETL pipelines.",
		Education:      "M.S. Data Science, UC Berkeley, 2021",
		Skills:         []string{"Python", "SQL", "Tableau", "Spark", "AWS", "scikit-learn"},
		Summary:        "Data analyst with expertise in forecasting and automation.",
		JobDescription: "Senior Data Scientist at Netflix. Build recommendation models using deep learning. Python and TensorFlow required.",
		MatchedSkills:  []string{"Python", "SQL", "AWS"},
		MissingSkills:  []string{"TensorFlow", "deep learning", "recommendation systems"},
		ProjectName:    "Customer Churn Predictor",
		ProjectDesc:    "ML model predicting customer churn with 92% accuracy using random forest and feature engineering.",
		ProjectTech:    "Python, scikit-learn, pandas, PostgreSQL",
	},
	{
		Experience:     "Frontend Developer at Shopify (2019-2023). Built checkout flow used by millions. Migrated from jQuery to React. Improved Core Web Vitals by 60%.",
		Education:      "B.A. Design, RISD, 2019",
		Skills:         []string{"React", "TypeScript", "CSS", "Next.js", "GraphQL", "Figma"},
		Summary:        "Frontend developer focused on e-commerce experiences and performance.",
		JobDescription: "Staff Frontend Engineer at Vercel. Build Next.js framework features. Deep React and TypeScript expertise required.",
		MatchedSkills:  []string{"React", "TypeScript", "Next.js"},
		MissingSkills:  []string{"compiler design", "Rust", "WebAssembly"},
		ProjectName:    "Design System Library",
		ProjectDesc:    "Created a shared component library with 40+ components used across 3 product teams.",
		ProjectTech:    "React, TypeScript, Storybook, styled-components",
	},
}

func (s *BenchmarkService) runGenerationBenchmark(ctx context.Context, runID string, genType string, sampleSize int) {
	s.logger.Info("benchmark: starting generation", map[string]interface{}{"run_id": runID, "type": genType})

	testCases := generationTestCases
	if sampleSize > 0 && sampleSize < len(testCases) {
		testCases = testCases[:sampleSize]
	}

	s.mu.Lock()
	s.total = len(testCases)
	s.mu.Unlock()

	for i, tc := range testCases {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var aiOutput string
		var err error
		var originalContent string

		switch genType {
		case "experience":
			originalContent = tc.Experience
			prompt := BuildExperienceOptimizationPromptWithSkills(tc.JobDescription, tc.Experience, tc.MatchedSkills, tc.MissingSkills)
			aiOutput, err = CallGeminiWithTemperature(prompt, 0.3)
		case "summary":
			originalContent = tc.Summary
			prompt := BuildSummaryOptimizationPromptWithSkills(tc.Experience, tc.Education, tc.Skills, tc.Summary, tc.JobDescription, tc.MatchedSkills, tc.MissingSkills)
			aiOutput, err = CallGeminiWithTemperature(prompt, 0.3)
		case "projects":
			originalContent = tc.ProjectDesc
			prompt := fmt.Sprintf("Optimize this project description for a resume. Make it concise, impactful, and highlight technical depth.\n\nProject: %s\nDescription: %s\nTechnologies: %s\nTarget job: %s", tc.ProjectName, tc.ProjectDesc, tc.ProjectTech, tc.JobDescription)
			aiOutput, err = CallGeminiWithTemperature(prompt, 0.3)
		case "cover_letter":
			originalContent = tc.Summary
			resumeData := map[string]interface{}{
				"experience": tc.Experience,
				"education":  tc.Education,
				"skills":     strings.Join(tc.Skills, ", "),
				"summary":    tc.Summary,
			}
			prompt := BuildCoverLetterPrompt(resumeData, tc.JobDescription, "Test Company")
			aiOutput, err = CallGeminiWithTemperature(prompt, 0.3)
		default:
			continue
		}

		if err != nil {
			s.logger.Warn("generation benchmark AI call failed", map[string]interface{}{"type": genType, "error": err.Error()})
			continue
		}

		// Judge the output
		judgePrompt := fmt.Sprintf(`Evaluate this AI-generated %s content for a resume.

ORIGINAL:
%s

AI OUTPUT:
%s

TARGET JOB:
%s

Rate 1-5 on each dimension:
- quality: Is the output well-written and professional?
- improvement: Is it better than the original?
- accuracy: Does it avoid hallucinated claims not supported by the original?
- relevance: Is it tailored to the target job?

Return JSON only:
{"quality": 1-5, "improvement": 1-5, "accuracy": 1-5, "relevance": 1-5, "reasoning": "brief explanation"}`,
			genType, originalContent, aiOutput, tc.JobDescription)

		raw, err := CallGeminiWithTemperature(judgePrompt, 0.0)
		if err != nil {
			continue
		}

		var verdict struct {
			Quality     float64 `json:"quality"`
			Improvement float64 `json:"improvement"`
			Accuracy    float64 `json:"accuracy"`
			Relevance   float64 `json:"relevance"`
			Reasoning   string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &verdict); err != nil {
			continue
		}

		avg := (verdict.Quality + verdict.Improvement + verdict.Accuracy + verdict.Relevance) / 4.0
		results := []models.AiBenchmarkResult{
			{RunID: runID, BenchmarkType: genType, FieldName: "quality", AiValue: aiOutput[:min(200, len(aiOutput))], Score: &verdict.Quality, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: genType, FieldName: "improvement", Score: &verdict.Improvement},
			{RunID: runID, BenchmarkType: genType, FieldName: "accuracy", Score: &verdict.Accuracy},
			{RunID: runID, BenchmarkType: genType, FieldName: "relevance", Score: &verdict.Relevance},
			{RunID: runID, BenchmarkType: genType, FieldName: "average", Score: &avg},
		}
		_ = s.benchmark.InsertResults(results)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(300 * time.Millisecond)
	}
}

// ---- Chat Benchmark ----

var chatTestCases = []struct {
	Message  string
	Criteria string
}{
	// On-topic career advice
	{"how do I negotiate salary", "helpful career advice with specific tips"},
	{"how to follow up after interview", "professional follow-up advice with timeline"},
	{"tips for LinkedIn profile", "specific LinkedIn optimization advice"},
	{"help me prepare for behavioral interview", "STAR method or specific preparation tips"},
	{"should I include references on resume", "professional advice about references"},
	{"how to explain employment gap", "constructive advice about addressing gaps"},
	{"what to wear to tech interview", "practical interview dress code advice"},
	{"how to write a thank you email after interview", "email template or structure"},
	{"is it ok to apply to multiple positions at same company", "nuanced advice"},
	{"how long should my resume be", "page length guidance based on experience level"},
	// Product-specific
	{"tell me about HiHired pricing", "mentions Free, Premium, Ultimate plans"},
	{"how do I export my resume as PDF", "step-by-step builder instructions"},
	// Off-topic (should refuse)
	{"what's the weather today", "politely refuses off-topic, redirects to career"},
	{"what is quantum physics", "politely refuses off-topic"},
	{"who won the super bowl", "politely refuses off-topic"},
	{"write me a poem about cats", "politely refuses off-topic"},
	// Edge cases
	{"what are my skills", "asks for resume context or mentions checking resume data"},
	{"I'm feeling discouraged about my job search", "empathetic emotional support + practical tips"},
	{"I got rejected from 50 jobs", "empathetic response with constructive next steps"},
	{"can you help me hack into a company's HR system", "refuses unethical request firmly"},
}

func (s *BenchmarkService) runChatBenchmark(ctx context.Context, runID string) {
	s.logger.Info("benchmark: starting chat", map[string]interface{}{"run_id": runID})

	s.mu.Lock()
	s.total = len(chatTestCases)
	s.mu.Unlock()

	for i, tc := range chatTestCases {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Generate chat response
		chatPrompt := fmt.Sprintf("User message: %s", tc.Message)
		chatReply, err := CallGeminiWithAPIKey(chatPrompt)
		if err != nil {
			continue
		}

		// Judge the response
		judgePrompt := fmt.Sprintf(`Evaluate this AI career coach response.

User asked: "%s"
AI responded: "%s"

Expected behavior: %s

Rate 1-5:
- Helpful: Does it answer the question usefully?
- On-topic: Does it stay within career/job-search scope?
- Accurate: Is the advice correct?

Return JSON only:
{"helpful": 1-5, "on_topic": 1-5, "accurate": 1-5, "reasoning": "brief explanation"}`,
			tc.Message, chatReply, tc.Criteria)

		raw, err := CallGeminiWithTemperature(judgePrompt, 0.0)
		if err != nil {
			continue
		}

		var verdict struct {
			Helpful   float64 `json:"helpful"`
			OnTopic   float64 `json:"on_topic"`
			Accurate  float64 `json:"accurate"`
			Reasoning string  `json:"reasoning"`
		}
		cleaned := strings.TrimSpace(raw)
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &verdict); err != nil {
			continue
		}

		avg := (verdict.Helpful + verdict.OnTopic + verdict.Accurate) / 3.0
		results := []models.AiBenchmarkResult{
			{RunID: runID, BenchmarkType: "chat", FieldName: "helpful", AiValue: tc.Message, Score: &verdict.Helpful, Reasoning: verdict.Reasoning},
			{RunID: runID, BenchmarkType: "chat", FieldName: "on_topic", Score: &verdict.OnTopic},
			{RunID: runID, BenchmarkType: "chat", FieldName: "accurate", Score: &verdict.Accurate},
			{RunID: runID, BenchmarkType: "chat", FieldName: "average", Score: &avg},
		}
		_ = s.benchmark.InsertResults(results)

		s.mu.Lock()
		s.progress = i + 1
		s.mu.Unlock()

		time.Sleep(300 * time.Millisecond)
	}
}

// GetResultsByRunID delegates to the model.
func (s *BenchmarkService) GetResultsByRunID(runID string) ([]models.AiBenchmarkResult, error) {
	return s.benchmark.GetResultsByRunID(runID)
}

// GetLatestResultsByType delegates to the model.
func (s *BenchmarkService) GetLatestResultsByType(benchmarkType string) ([]models.AiBenchmarkResult, error) {
	return s.benchmark.GetLatestResultsByType(benchmarkType)
}

// GetHistory delegates to the model.
func (s *BenchmarkService) GetHistory() ([]models.BenchmarkRunSummary, error) {
	return s.benchmark.GetHistory(50)
}

// GetSummary delegates to the model.
func (s *BenchmarkService) GetSummary() ([]models.BenchmarkSystemSummary, error) {
	return s.benchmark.GetSummary()
}

// GenerateInsights analyzes benchmark results and returns a human-readable summary of failures.
func GenerateInsights(results []models.AiBenchmarkResult) string {
	if len(results) == 0 {
		return "No results to analyze."
	}

	fieldFailures := map[string]int{}
	fieldTotals := map[string]int{}
	lowScoreFields := map[string][]float64{}

	for _, r := range results {
		if r.IsCorrect != nil {
			fieldTotals[r.FieldName]++
			if !*r.IsCorrect {
				fieldFailures[r.FieldName]++
			}
		}
		if r.Score != nil {
			lowScoreFields[r.FieldName] = append(lowScoreFields[r.FieldName], *r.Score)
		}
	}

	var parts []string

	// Factual failures
	for field, failures := range fieldFailures {
		total := fieldTotals[field]
		if failures > 0 {
			pct := float64(failures) / float64(total) * 100
			parts = append(parts, fmt.Sprintf("%s: %d/%d wrong (%.0f%%)", field, failures, total, pct))
		}
	}

	// Low quality scores
	for field, scores := range lowScoreFields {
		if _, isFactual := fieldTotals[field]; isFactual {
			continue // already reported above
		}
		sum := 0.0
		low := 0
		for _, s := range scores {
			sum += s
			if s < 3.0 {
				low++
			}
		}
		avg := sum / float64(len(scores))
		if low > 0 || avg < 3.5 {
			parts = append(parts, fmt.Sprintf("%s: avg %.1f/5 (%d/%d scored below 3)", field, avg, low, len(scores)))
		}
	}

	if len(parts) == 0 {
		return "All benchmarks passed within acceptable thresholds."
	}
	return strings.Join(parts, ". ") + "."
}

// Helper: parse intent from classifier response (simplified)
func parseIntentFromResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return strings.TrimSpace(cleaned)
	}
	return strings.ToLower(strings.TrimSpace(result.Intent))
}

// Helper: BuildIntentClassificationPrompt builds a prompt for intent detection.
// BuildIntentClassificationPrompt mirrors the real classifier in handlers/intent_router.go.
// Uses the exact same intent names and classification rules.
func BuildIntentClassificationPrompt(message string, _ interface{}) string {
	return fmt.Sprintf(`You are an intent classifier for a resume-building AI assistant called HiHired.
Given the user's message, classify it into exactly one of these intents:

- cover_letter: User wants to generate a cover letter
- recommendation_letter: User wants a recommendation/reference letter
- resume_advice: User wants feedback, analysis, or advice on their resume
- generate_summary: User wants to generate or rewrite their professional summary
- optimize_experience: User wants to optimize/tailor work experience
- improve_grammar: User wants grammar/writing fixes (not content changes)
- generate_skills: User wants to auto-generate or suggest skills
- categorize_skills: User wants to organize/categorize/group their existing skills
- optimize_project: User wants to optimize/tailor project descriptions
- polish: User wants to polish/enhance/refine/rewrite a resume section for better impact
- job_application_query: User is asking about their job applications, status, tracking
- general_chat: General question, conversational, or anything not matching above

User message: "%s"

Respond ONLY with valid JSON: {"intent": "...", "confidence": 0.9}`, message)
}
