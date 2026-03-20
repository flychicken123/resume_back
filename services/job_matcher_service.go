package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"resumeai/models"
	"resumeai/utils"
)

// ResumeJobMatchInput describes the data required to compute matches.
type ResumeJobMatchInput struct {
	UserID            int
	ResumeHash        string
	Position          string
	Summary           string
	Experience        string
	Education         string
	JobDescription    string
	PreferredLocation string
	Skills            []string
	CandidateJobLimit int
	MaxResults        int
	CandidateYOE      float64 // Extracted years of experience (set during matching)
	QuickMode         bool    // If true, skip AI filtering for faster initial results
}

// ResumeJobMatcher defines the behaviour exposed to handlers.
type ResumeJobMatcher interface {
	MatchAndStore(ctx context.Context, input ResumeJobMatchInput) ([]*models.ResumeJobMatchRecord, error)
}

var globalJobMatcherSvc *jobMatcherService

// SetJobMatcherEmbeddingDBCache wires a DB cache into the job matcher service.
func SetJobMatcherEmbeddingDBCache(dbCache EmbeddingDBCache) {
	if globalJobMatcherSvc != nil {
		globalJobMatcherSvc.embeddingDBCache = dbCache
	}
}

type resumeEmbeddingCacheEntry struct {
	vec    []float32
	expiry time.Time
}

// EmbeddingDBCache persists resume embeddings to DB.
type EmbeddingDBCache interface {
	GetEmbedding(resumeHash string) ([]float32, error)
	UpsertEmbedding(resumeHash string, embedding []float32) error
}

type jobMatcherService struct {
	postings         *models.JobPostingModel
	matches          *models.ResumeJobMatchModel
	logger           *utils.Logger
	copilot          *CopilotAgent
	embeddingSvc     *EmbeddingService
	embeddingCache   sync.Map
	embeddingDBCache EmbeddingDBCache
}

const vectorSimilarityThreshold float32 = 0.5

// NewJobMatcherService wires a job matcher service.
func NewJobMatcherService(postings *models.JobPostingModel, matches *models.ResumeJobMatchModel, logger *utils.Logger, copilot *CopilotAgent, embeddingSvc *EmbeddingService, ctx context.Context) ResumeJobMatcher {
	if logger == nil {
		logger = utils.NewLogger()
	}
	svc := &jobMatcherService{
		postings:     postings,
		matches:      matches,
		logger:       logger,
		copilot:      copilot,
		embeddingSvc: embeddingSvc,
	}
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				svc.embeddingCache.Range(func(key, value interface{}) bool {
					entry := value.(resumeEmbeddingCacheEntry)
					if time.Now().After(entry.expiry) {
						svc.embeddingCache.Delete(key)
					}
					return true
				})
			case <-ctx.Done():
				return
			}
		}
	}()
	globalJobMatcherSvc = svc
	return svc
}

type scoredJob struct {
	posting *models.JobPosting
	score   float64
}

const (
	seniorityIntern = iota
	seniorityEntry
	seniorityMid
	senioritySenior
	seniorityLead
)

var (
	seniorityLeadKeywords   = []string{"lead", "principal", "manager", "director", "head", "vp", "svp", "evp", "chief", "cto", "cpo", "cio", "cfo", "coo", "vice president"}
	senioritySeniorKeywords = []string{"senior", "sr", "staff"}
	seniorityEntryKeywords  = []string{"junior", "jr", "associate", "entry", "graduate"}
	seniorityInternKeywords = []string{"intern", "internship", "co-op", "coop", "trainee", "apprentice"}
)

const (
	llmReRankTopK            = 30
	llmReRankAlpha           = 0.7
	llmReRankBeta            = 0.3
	llmReRankTimeout         = 15 * time.Second
	llmSkillInferenceTimeout = 15 * time.Second
	defaultMaxResults        = 100
)

// MatchAndStore evaluates current postings, stores matches, and returns the enriched view.
func (s *jobMatcherService) MatchAndStore(ctx context.Context, input ResumeJobMatchInput) ([]*models.ResumeJobMatchRecord, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if strings.TrimSpace(input.ResumeHash) == "" {
		return nil, ErrMissingResumeHash
	}

	// Two-pass approach:
	// Pass 1: Vector similarity search (or keyword fallback)
	// Pass 2: Full heuristic scoring on filtered set

	limit := input.CandidateJobLimit
	if limit <= 0 {
		limit = 1000 // Increased default for two-pass approach
	}

	position := strings.ToLower(strings.TrimSpace(input.Position))
	skills := normaliseSkills(input.Skills)

	// Attempt to generate a resume embedding for vector search
	var vec []float32
	if s.embeddingSvc != nil {
		// Check cache first
		if input.ResumeHash != "" {
			if v, ok := s.embeddingCache.Load(input.ResumeHash); ok {
				entry := v.(resumeEmbeddingCacheEntry)
				if time.Now().Before(entry.expiry) {
					vec = entry.vec
					s.logger.Info("job match: embedding cache hit, skipping API call")
				}
			}
		}
		// Check DB cache
		if len(vec) == 0 && input.ResumeHash != "" && s.embeddingDBCache != nil {
			if dbVec, err := s.embeddingDBCache.GetEmbedding(input.ResumeHash); err == nil && len(dbVec) > 0 {
				vec = dbVec
				s.embeddingCache.Store(input.ResumeHash, resumeEmbeddingCacheEntry{
					vec:    vec,
					expiry: time.Now().Add(time.Hour),
				})
				s.logger.Info("job match: embedding DB cache hit, skipping API call")
			}
		}
		// Cache miss — call the API
		if len(vec) == 0 {
			embCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			v, err := s.embeddingSvc.EmbedResumeContent(embCtx, input)
			if err != nil || len(v) == 0 {
				s.logger.Warn("job match: resume embedding failed, falling back to keyword search", map[string]interface{}{"error": func() string {
					if err != nil {
						return err.Error()
					}
					return "empty vector"
				}()})
			} else {
				vec = v
				if input.ResumeHash != "" {
					s.embeddingCache.Store(input.ResumeHash, resumeEmbeddingCacheEntry{
						vec:    vec,
						expiry: time.Now().Add(time.Hour),
					})
					// Persist to DB
					if s.embeddingDBCache != nil {
						_ = s.embeddingDBCache.UpsertEmbedding(input.ResumeHash, vec)
					}
				}
			}
		}
	}

	// Pass 1: Three-level candidate retrieval
	var jobs []*models.JobPosting
	if len(vec) > 0 {
		var vecErr error
		jobs, vecErr = s.postings.ListActiveByVectorSimilarity(ctx, vec, limit, vectorSimilarityThreshold)
		if vecErr != nil || len(jobs) == 0 {
			s.logger.Info("job match: vector search empty/failed, falling back to keyword search")
			jobs = nil
		} else {
			s.logger.Info("job match: using vector search", map[string]interface{}{"candidates": len(jobs)})
		}
	}
	if jobs == nil {
		var kwErr error
		jobs, kwErr = s.postings.ListActiveByRelevance(skills, input.Position, limit)
		if kwErr != nil {
			s.logger.Warn("ListActiveByRelevance failed, falling back to ListActive", map[string]interface{}{"error": kwErr.Error()})
			var fallbackErr error
			jobs, fallbackErr = s.postings.ListActive(nil, limit)
			if fallbackErr != nil {
				return nil, fallbackErr
			}
		}
	}

	// Pass 1.5: AI-based career field filtering (filter out unrelated job categories)
	// Skip if QuickMode is enabled for faster initial results
	if !input.QuickMode {
		jobs = s.applyAICareerFieldFilter(ctx, input, jobs)
	} else {
		s.logger.Info("AI career field filter skipped (quick mode)", map[string]interface{}{
			"jobCount": len(jobs),
		})
	}

	// Pass 2: Full heuristic scoring
	sourceText := strings.Join([]string{input.Summary, input.Experience, input.Education}, " ")
	resumeText := strings.ToLower(strings.TrimSpace(sourceText))
	keywords := extractKeywords(resumeText, 40)
	preferredLocation := strings.ToLower(strings.TrimSpace(input.PreferredLocation))
	if preferredLocation == "" {
		preferredLocation = "united states"
	}
	candidateLevel := determineCandidateSeniority(input.Position, sourceText)

	// Extract candidate's years of experience from their work history
	candidateYOE := ExtractYOEFromExperience(input.Experience)
	s.logger.Info("extracted candidate YOE", map[string]interface{}{
		"years":     candidateYOE.Years,
		"confident": candidateYOE.Confident,
	})
	// Set YOE on input for LLM re-ranking prompt
	input.CandidateYOE = candidateYOE.Years

	scored := make([]scoredJob, 0, len(jobs))
	for _, job := range jobs {
		if !locationCompatible(preferredLocation, job) {
			continue
		}

		jobLevel := determineJobSeniority(job.Title, job.Description)
		// Always skip intern roles
		if jobLevel == seniorityIntern {
			continue
		}

		// Seniority level filtering: allow [A-1, A, A+1] range
		// Candidate can see jobs one level below, same level, or one level above
		if candidateLevel >= 0 {
			levelDiff := jobLevel - candidateLevel
			// Skip if job is more than 1 level above or more than 1 level below
			if levelDiff > 1 || levelDiff < -1 {
				continue
			}
			// Special case: Entry level candidates can't see below (there's nothing below except intern)
			// Lead level candidates: levelDiff < -1 already handles skipping mid/entry
		}

		// YOE-based filtering: extract job's YOE requirement and check tier
		jobYOEReq := ExtractYOEFromJobDescription(job.Description)
		if candidateYOE.Years > 0 && jobYOEReq.Found {
			tier, _ := ComputeYOETier(candidateYOE.Years, jobYOEReq)
			// Filter out Tier 3 jobs (YOE gap too large)
			if tier == YOETier3 {
				continue
			}
		}

		score := computeMatchScore(job, position, skills, keywords, resumeText, preferredLocation, candidateYOE.Years)
		// Blend vector embedding similarity into score (+6 to +12 points)
		score += job.EmbeddingSimilarity * 12
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredJob{posting: job, score: score})
	}

	// Always clean up stale matches (> 3 months)
	cutoff := time.Now().AddDate(0, -3, 0)
	if _, err := s.matches.DeleteOlderThan(cutoff); err != nil {
		s.logger.Warn("failed pruning expired resume job matches", map[string]interface{}{"error": err.Error()})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].posting.LastSeenAt.After(scored[j].posting.LastSeenAt)
		}
		return scored[i].score > scored[j].score
	})

	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	// Prepare resume content for skill inference
	resumeContent := ResumeContent{
		Position:   input.Position,
		Summary:    input.Summary,
		Experience: input.Experience,
		Education:  input.Education,
		Skills:     input.Skills,
	}

	// Start LLM skill inference in parallel (with timeout)
	type skillResult struct {
		skills []string
		err    error
	}
	skillChan := make(chan skillResult, 1)
	go func() {
		inferCtx, cancel := context.WithTimeout(ctx, llmSkillInferenceTimeout)
		defer cancel()
		inferrer := GetSkillInferenceLLM()
		skills, err := inferrer.InferSkillsFromResume(inferCtx, resumeContent)
		skillChan <- skillResult{skills: skills, err: err}
	}()

	// Apply LangChain-based re-ranking on the top K matches (runs in parallel with skill inference)
	s.applyLLMReRank(ctx, input, scored, maxResults)

	// Filter out jobs with score 0 (jobs in wrong industry/category as determined by LLM)
	filteredScored := make([]scoredJob, 0, len(scored))
	for _, sj := range scored {
		if sj.score > 0 {
			filteredScored = append(filteredScored, sj)
		}
	}
	scored = filteredScored

	// Limit to at most 3 jobs per company so results stay diverse.
	{
		companyCount := make(map[int]int)
		diverseScored := make([]scoredJob, 0, len(scored))
		for _, sj := range scored {
			cid := sj.posting.CompanyID
			if companyCount[cid] < 3 {
				diverseScored = append(diverseScored, sj)
				companyCount[cid]++
			}
		}
		scored = diverseScored
	}

	// Trim to the requested maximum number of results after re-ranking.
	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	creates := make([]models.JobMatchCreate, len(scored))
	for i, match := range scored {
		creates[i] = models.JobMatchCreate{
			JobPostingID: match.posting.ID,
			MatchScore:   match.score,
		}
	}

	if err := s.matches.UpsertMatches(input.UserID, input.ResumeHash, creates); err != nil {
		return nil, err
	}

	results, err := s.matches.ListByUserAndResume(input.UserID, input.ResumeHash, maxResults)
	if err != nil {
		return nil, err
	}

	// Wait for skill inference result
	sr := <-skillChan
	var userSkills []string
	if sr.err != nil {
		// Fallback to rule-based inference on error
		userSkills = ExpandSkillsWithInference(input.Skills)
	} else {
		userSkills = sr.skills
	}

	// Apply skill gaps using pre-computed skills
	computeSkillGapsFromSkills(results, userSkills)

	return results, nil
}

func locationCompatible(preferred string, job *models.JobPosting) bool {
	if job == nil {
		return false
	}
	loc := job.Location
	remote := job.RemoteType
	combined := strings.ToLower(strings.TrimSpace(loc + " " + remote))
	preferred = strings.ToLower(strings.TrimSpace(preferred))

	// If user provided a preference, require either a match or remote.
	if preferred != "" {
		if strings.Contains(combined, preferred) {
			return true
		}
		if strings.Contains(combined, "remote") {
			return true
		}
		// Fall back to supported geos to avoid obviously bad suggestions.
		return utils.IsSupportedJobLocation(loc, remote)
	}

	// No preference provided: use broad supported geos filter.
	return utils.IsSupportedJobLocation(loc, remote)
}

// ErrInvalidUserID indicates the caller failed to provide a user id.
var ErrInvalidUserID = &MatchError{msg: "user id is required"}

// ErrMissingResumeHash indicates the resume hash is empty.
var ErrMissingResumeHash = &MatchError{msg: "resume hash is required"}

// MatchError captures validation failures when matching resumes.
type MatchError struct {
	msg string
}

func (e *MatchError) Error() string { return e.msg }

func normaliseSkills(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, skill := range raw {
		skill = strings.ToLower(strings.TrimSpace(skill))
		skill = strings.NewReplacer("-", " ", "/", " ").Replace(skill)
		if skill == "" {
			continue
		}
		if _, exists := seen[skill]; exists {
			continue
		}
		seen[skill] = struct{}{}
		out = append(out, skill)
	}
	return out
}

var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "with": {}, "from": {}, "that": {}, "this": {}, "have": {},
	"your": {}, "will": {}, "into": {}, "about": {}, "using": {}, "years": {}, "experience": {},
	"skills": {}, "team": {}, "work": {}, "strong": {}, "ability": {}, "including": {},
	"within": {}, "across": {}, "other": {}, "their": {}, "while": {}, "over": {},
}

func extractKeywords(text string, max int) []string {
	if text == "" || max <= 0 {
		return nil
	}

	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})

	seen := make(map[string]struct{}, len(tokens))
	keywords := make([]string, 0, max)
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if len(token) < 4 {
			continue
		}
		if _, skip := stopWords[token]; skip {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		keywords = append(keywords, token)
		if len(keywords) >= max {
			break
		}
	}
	return keywords
}

func computeMatchScore(job *models.JobPosting, position string, skills []string, keywords []string, resumeText string, preferredLocation string, candidateYOE float64) float64 {
	if job == nil {
		return 0
	}

	title := strings.ToLower(job.Title)
	description := strings.ToLower(job.Description)
	department := strings.ToLower(job.Department)
	location := strings.ToLower(job.Location)
	remote := strings.ToLower(job.RemoteType)

	combinedLocation := strings.TrimSpace(job.Location + " " + job.RemoteType)
	preferred := strings.TrimSpace(preferredLocation)
	if preferred != "" {
		if utils.LooksUSLocation(preferred) && !utils.LooksCanadianLocation(preferred) {
			if utils.LooksCanadianLocation(combinedLocation) && !utils.LooksUSLocation(combinedLocation) {
				return -100
			}
		}
	}

	score := 0.0
	skillMatches := 0

	// Fast path: if job has AI-extracted skills, use set intersection
	if len(job.ExtractedSkills) > 0 && len(skills) > 0 {
		jobSkillSet := make(map[string]bool, len(job.ExtractedSkills))
		for _, s := range job.ExtractedSkills {
			jobSkillSet[strings.ToLower(s)] = true
		}
		for _, skill := range skills {
			if skill == "" {
				continue
			}
			if jobSkillSet[strings.ToLower(skill)] {
				score += 4
				skillMatches++
			}
		}
		// Skip the keyword-based matching below
		goto afterSkillMatching
	}

	for _, skill := range skills {
		if skill == "" {
			continue
		}

		// Expand skill to include synonyms (e.g., "react" -> ["react", "reactjs", "react.js"])
		skillVariations := ExpandSkillWithSynonyms(skill)

		matched := false
		// Check title first (highest weight)
		for _, variant := range skillVariations {
			if title != "" && strings.Contains(title, variant) {
				score += 5
				matched = true
				break
			}
		}
		// Check description if not matched in title
		if !matched {
			for _, variant := range skillVariations {
				if description != "" && strings.Contains(description, variant) {
					score += 3
					matched = true
					break
				}
			}
		}
		// Check department if not matched elsewhere
		if !matched {
			for _, variant := range skillVariations {
				if department != "" && strings.Contains(department, variant) {
					score += 1.5
					matched = true
					break
				}
			}
		}
		if matched {
			skillMatches++
		}
	}

afterSkillMatching:

	positionTokens := tokenizeWords(position)
	titleTokens := tokenizeWords(title)
	descriptionTokens := tokenizeWords(description)
	if jobLooksLikeInternRole(title, description) {
		return 0
	}

	if position != "" && title != "" && strings.Contains(title, position) {
		score += 4
	}

	it := countTokenOverlap(positionTokens, titleTokens)
	if it > 0 {
		score += math.Min(float64(it)*2.5, 6)
	}

	keywordHits := 0
	if len(keywords) > 0 && description != "" {
		for _, keyword := range keywords {
			if strings.Contains(description, keyword) {
				keywordHits++
			}
		}
		if keywordHits > 0 {
			score += math.Min(float64(keywordHits)*0.6, 6)
		}
	}

	descriptionOverlap := countTokenOverlap(positionTokens, descriptionTokens)
	if descriptionOverlap > 0 {
		score += math.Min(float64(descriptionOverlap)*1.2, 4)
	}

	if strings.Contains(resumeText, "remote") {
		if strings.Contains(remote, "remote") || strings.Contains(location, "remote") {
			score += 1
		}
	}

	if preferredLocation != "" {
		score += computeLocationBoost(location, remote, preferredLocation)
	}

	// Recency boost - favor recently posted jobs
	score += computeRecencyBoost(job)

	// YOE alignment bonus/penalty
	if candidateYOE > 0 {
		jobYOEReq := ExtractYOEFromJobDescription(description)
		yoeAdjustment := ComputeYOEScoreAdjustment(candidateYOE, jobYOEReq)
		score += yoeAdjustment
	}

	if skillMatches == 0 && keywordHits == 0 {
		return 0
	}

	if score < 0 {
		score = 0
	}

	return math.Round(score*100) / 100
}

func computeLocationBoost(jobLocation, jobRemoteType, preferredLocation string) float64 {
	pl := strings.TrimSpace(preferredLocation)
	if pl == "" {
		return 0
	}

	pl = strings.ToLower(pl)
	jl := strings.ToLower(strings.TrimSpace(jobLocation))
	jr := strings.ToLower(strings.TrimSpace(jobRemoteType))

	if strings.Contains(pl, "remote") {
		if strings.Contains(jr, "remote") || strings.Contains(jl, "remote") {
			return 2.5
		}
	}

	if jl != "" {
		if strings.Contains(jl, pl) || strings.Contains(pl, jl) {
			return 3
		}
		for _, token := range strings.FieldsFunc(pl, func(r rune) bool {
			return r == ',' || r == '/' || r == '-'
		}) {
			trimmed := strings.TrimSpace(token)
			if trimmed == "" {
				continue
			}
			if strings.Contains(jl, trimmed) {
				return 2.2
			}
		}
	}

	if strings.Contains(jr, "remote") {
		return 1.5
	}

	return -0.5
}

// computeRecencyBoost adds points for recently posted jobs.
// This helps surface fresh opportunities that are more likely to be actively hiring.
func computeRecencyBoost(job *models.JobPosting) float64 {
	if job == nil {
		return 0
	}

	// Use posted_at if available, otherwise fall back to first_seen_at
	var postDate time.Time
	if job.PostedAt != nil {
		postDate = *job.PostedAt
	} else {
		postDate = job.FirstSeenAt
	}

	// Handle zero time (shouldn't happen but be defensive)
	if postDate.IsZero() {
		return 0
	}

	days := time.Since(postDate).Hours() / 24
	if days < 0 {
		days = 0
	}
	// Exponential decay: 4 * e^(-days/7)
	// Day 0: +4.0, Day 3: +2.6, Day 7: +1.5, Day 14: +0.5, Day 30: +0.05
	return 4.0 * math.Exp(-days/7.0)
}

// computeSkillGapsFromSkills calculates skill gaps using pre-computed user skills.
// This is more efficient when skills are already inferred (e.g., from parallel LLM call).
func computeSkillGapsFromSkills(matches []*models.ResumeJobMatchRecord, userSkills []string) {
	if len(matches) == 0 {
		return
	}

	// Build set of user skills
	userSkillSet := make(map[string]bool, len(userSkills))
	for _, s := range userSkills {
		userSkillSet[s] = true
	}

	// Calculate gaps for each match
	for _, match := range matches {
		requiredSkills := ExtractSkillsFromText(match.JobDescription)
		if len(requiredSkills) == 0 {
			continue
		}

		var matched, missing []string
		for _, reqSkill := range requiredSkills {
			if userSkillSet[reqSkill] {
				matched = append(matched, reqSkill)
			} else {
				missing = append(missing, reqSkill)
			}
		}

		match.RequiredSkills = requiredSkills
		match.MatchedSkills = matched
		match.MissingSkills = missing
	}
}

func jobLooksLikeInternRole(title, description string) bool {
	return determineJobSeniority(title, description) == seniorityIntern
}

func determineJobSeniority(title, description string) int {
	level := detectSeniorityFromString(title)
	descriptionLevel := detectSeniorityFromString(description)
	if descriptionLevel > level {
		level = descriptionLevel
	}
	if level < 0 {
		level = seniorityMid
	}
	return level
}

func determineCandidateSeniority(position, resumeText string) int {
	level := detectSeniorityFromString(position)
	resumeLevel := detectSeniorityFromString(resumeText)
	if resumeLevel > level {
		level = resumeLevel
	}
	if level < 0 {
		level = seniorityMid
	}
	return level
}

func detectSeniorityFromString(value string) int {
	normalized := normalizeSeniorityString(value)
	if normalized == "" {
		return -1
	}
	switch {
	case containsAnySeniority(normalized, seniorityInternKeywords):
		return seniorityIntern
	case containsAnySeniority(normalized, senioritySeniorKeywords):
		return senioritySenior
	case containsAnySeniority(normalized, seniorityLeadKeywords):
		return seniorityLead
	case containsAnySeniority(normalized, seniorityEntryKeywords):
		return seniorityEntry
	default:
		return -1
	}
}

func normalizeSeniorityString(value string) string {
	lower := strings.ToLower(value)
	replacer := strings.NewReplacer(".", " ", "-", " ", "_", " ", "/", " ", ",", " ")
	cleaned := replacer.Replace(lower)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned
}

func containsAnySeniority(value string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(value, kw) {
			return true
		}
	}
	return false
}

func tokenizeWords(value string) []string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return nil
	}
	tokens := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '/' || r == '-' || r == '_' || r == ',' || r == '.' || r == '|'
	})
	filtered := make([]string, 0, len(tokens))
	seen := make(map[string]struct{})
	for _, token := range tokens {
		clean := strings.TrimSpace(token)
		if len(clean) < 2 {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		filtered = append(filtered, clean)
	}
	return filtered
}

func countTokenOverlap(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, token := range a {
		set[token] = struct{}{}
	}
	overlap := 0
	for _, token := range b {
		if _, exists := set[token]; exists {
			overlap++
		}
	}
	return overlap
}

const (
	aiCareerFilterBatchSize       = 75               // Number of jobs per AI call
	aiCareerFilterTimeout         = 12 * time.Second // Timeout per AI call
	aiCareerFieldClassifyTimeout  = 8 * time.Second  // Timeout for career field classification
)

// batchResult holds the result of processing a single batch
type batchResult struct {
	batchStart      int
	batchSize       int
	relevantIndices []int
	irrelevantIDs   []int64 // Job IDs that were filtered out (for caching)
	relevantIDs     []int64 // Job IDs that were kept (for caching)
	filteredCount   int
	err             error
}

// classifyCandidateCareerField uses AI to determine the candidate's career field.
// Results are cached by candidate profile hash.
func (s *jobMatcherService) classifyCandidateCareerField(ctx context.Context, input ResumeJobMatchInput) CareerField {
	cache := GetJobFilterCache()

	// Check cache first
	if field, found := cache.GetCareerField(input.Position, input.Skills); found {
		s.logger.Info("career field cache hit", map[string]interface{}{
			"position":    input.Position,
			"careerField": string(field),
		})
		return field
	}

	// Cache miss - call AI to classify
	if s.copilot == nil {
		return CareerFieldUnknown
	}

	prompt := BuildCareerFieldClassificationPrompt(input.Position, input.Skills, input.Summary)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, aiCareerFieldClassifyTimeout)
	defer cancel()

	raw, err := s.copilot.RunPrompt(ctxWithTimeout, prompt)
	if err != nil {
		s.logger.Warn("career field classification failed", map[string]interface{}{
			"error": err.Error(),
		})
		return CareerFieldUnknown
	}

	field := ParseCareerFieldClassificationResponse(raw)

	// Cache the result
	cache.SetCareerField(input.Position, input.Skills, field)

	s.logger.Info("career field classified and cached", map[string]interface{}{
		"position":    input.Position,
		"careerField": string(field),
	})

	return field
}

// applyAICareerFieldFilter uses AI to filter out jobs that don't match the candidate's career field.
// This is a pre-filter that runs before heuristic scoring to eliminate obviously irrelevant jobs
// (e.g., Account Manager jobs for a Software Engineer candidate).
// Features:
// - Two-level caching: career field + job relevance
// - Parallel batch processing for cache misses
func (s *jobMatcherService) applyAICareerFieldFilter(ctx context.Context, input ResumeJobMatchInput, jobs []*models.JobPosting) []*models.JobPosting {
	if s.copilot == nil {
		s.logger.Warn("AI career field filter skipped: copilot not available", nil)
		return jobs
	}
	if len(jobs) == 0 {
		return jobs
	}

	cache := GetJobFilterCache()

	// Step 1: Classify candidate's career field (cached)
	careerField := s.classifyCandidateCareerField(ctx, input)
	if careerField == CareerFieldUnknown {
		s.logger.Warn("could not determine career field, skipping filter", nil)
		return jobs
	}

	// Step 2: Check pre-computed career field first, then cache, then LLM
	var cachedRelevant []*models.JobPosting
	var uncachedJobs []*models.JobPosting

	cacheHits := 0
	cacheMisses := 0
	preComputedHits := 0

	for _, job := range jobs {
		// Fast path: if career_field is pre-computed on the job, compare directly
		if job.CareerField != "" {
			preComputedHits++
			jobField := CareerField(job.CareerField)
			if jobField == careerField || jobField == CareerFieldOther || careerField == CareerFieldOther {
				cachedRelevant = append(cachedRelevant, job)
			}
			continue
		}
		if relevant, found := cache.GetJobRelevance(careerField, job.ID); found {
			cacheHits++
			if relevant {
				cachedRelevant = append(cachedRelevant, job)
			}
			// If not relevant, we skip it (filtered out from cache)
		} else {
			cacheMisses++
			uncachedJobs = append(uncachedJobs, job)
		}
	}

	s.logger.Info("AI career field filter cache check", map[string]interface{}{
		"careerField":   string(careerField),
		"totalJobs":     len(jobs),
		"cacheHits":     cacheHits,
		"cacheMisses":   cacheMisses,
		"cachedRelevant": len(cachedRelevant),
	})

	// If all jobs were cached, return early
	if len(uncachedJobs) == 0 {
		s.logger.Info("AI career field filter completed (all from cache)", map[string]interface{}{
			"filteredCount": len(cachedRelevant),
		})
		return cachedRelevant
	}

	// Step 3: Process uncached jobs in parallel batches
	numBatches := (len(uncachedJobs) + aiCareerFilterBatchSize - 1) / aiCareerFilterBatchSize
	resultsChan := make(chan batchResult, numBatches)

	for batchIdx := 0; batchIdx < numBatches; batchIdx++ {
		batchStart := batchIdx * aiCareerFilterBatchSize
		batchEnd := batchStart + aiCareerFilterBatchSize
		if batchEnd > len(uncachedJobs) {
			batchEnd = len(uncachedJobs)
		}

		batchJobs := uncachedJobs[batchStart:batchEnd]
		batchStartCopy := batchStart

		go func(bStart int, bJobs []*models.JobPosting) {
			candidates := make([]JobRelevanceCandidate, len(bJobs))
			for i, job := range bJobs {
				candidates[i] = JobRelevanceCandidate{
					Index:      bStart + i + 1,
					Title:      job.Title,
					Department: job.Department,
				}
			}

			prompt := BuildJobRelevanceFilterPrompt(input, candidates)
			ctxWithTimeout, cancel := context.WithTimeout(ctx, aiCareerFilterTimeout)
			defer cancel()

			raw, err := s.copilot.RunPrompt(ctxWithTimeout, prompt)

			if err != nil {
				s.logger.Warn("AI career field filter failed for batch, keeping all jobs", map[string]interface{}{
					"error":      err.Error(),
					"batchStart": bStart,
					"batchSize":  len(bJobs),
				})
				// Fail open - keep all jobs, but don't cache (transient failure)
				allIndices := make([]int, len(bJobs))
				for i := range bJobs {
					allIndices[i] = bStart + i + 1
				}
				resultsChan <- batchResult{
					batchStart:      bStart,
					batchSize:       len(bJobs),
					relevantIndices: allIndices,
					filteredCount:   0,
					err:             err,
				}
				return
			}

			parsed := ParseJobRelevanceFilterResponse(raw, len(candidates))

			// Collect job IDs for caching
			relevantSet := make(map[int]bool)
			for _, idx := range parsed.RelevantIndices {
				relevantSet[idx] = true
			}

			var relevantIDs, irrelevantIDs []int64
			for i, job := range bJobs {
				localIdx := bStart + i + 1
				if relevantSet[localIdx] {
					relevantIDs = append(relevantIDs, job.ID)
				} else {
					irrelevantIDs = append(irrelevantIDs, job.ID)
				}
			}

			resultsChan <- batchResult{
				batchStart:      bStart,
				batchSize:       len(bJobs),
				relevantIndices: parsed.RelevantIndices,
				relevantIDs:     relevantIDs,
				irrelevantIDs:   irrelevantIDs,
				filteredCount:   parsed.FilteredCount,
				err:             nil,
			}
		}(batchStartCopy, batchJobs)
	}

	// Collect results
	relevantFromAI := make(map[int]bool)
	successfulBatches := 0
	failedBatches := 0

	for i := 0; i < numBatches; i++ {
		result := <-resultsChan

		for _, idx := range result.relevantIndices {
			relevantFromAI[idx] = true
		}

		if result.err != nil {
			failedBatches++
		} else {
			successfulBatches++
			// Cache the results
			cache.SetBulkJobRelevance(careerField, result.relevantIDs, result.irrelevantIDs)
		}
	}
	close(resultsChan)

	// Step 4: Build final list (cached + newly evaluated)
	var aiRelevant []*models.JobPosting
	for i, job := range uncachedJobs {
		// The index in relevantFromAI is 1-based within each batch
		batchIdx := i / aiCareerFilterBatchSize
		posInBatch := i % aiCareerFilterBatchSize
		globalIdx := batchIdx*aiCareerFilterBatchSize + posInBatch + 1
		if relevantFromAI[globalIdx] {
			aiRelevant = append(aiRelevant, job)
		}
	}

	// Combine cached and AI-evaluated results
	filteredJobs := append(cachedRelevant, aiRelevant...)

	// Safeguard: If AI filter removed more than 80% of jobs, it's likely too aggressive.
	// In this case, return all jobs and let the scoring algorithm handle relevance.
	filterRate := float64(len(jobs)-len(filteredJobs)) / float64(len(jobs))
	if len(jobs) > 20 && filterRate > 0.80 {
		s.logger.Warn("AI career field filter was too aggressive, returning all jobs", map[string]interface{}{
			"careerField":     string(careerField),
			"originalCount":   len(jobs),
			"filteredCount":   len(filteredJobs),
			"filterRate":      filterRate,
		})
		return jobs
	}

	s.logger.Info("AI career field filter completed", map[string]interface{}{
		"careerField":       string(careerField),
		"originalCount":     len(jobs),
		"filteredCount":     len(filteredJobs),
		"fromCache":         len(cachedRelevant),
		"fromAI":            len(aiRelevant),
		"successfulBatches": successfulBatches,
		"failedBatches":     failedBatches,
		"filterRate":        filterRate,
	})

	return filteredJobs
}

// applyLLMReRank uses the LangChain-backed copilot agent (if available) to
// re-rank the top K matches based on a richer understanding of the resume and
// job descriptions. It keeps the existing heuristic score as a baseline and
// blends it with an LLM-provided score.
func (s *jobMatcherService) applyLLMReRank(ctx context.Context, input ResumeJobMatchInput, scored []scoredJob, maxResults int) {
	if s.copilot == nil {
		return
	}
	if len(scored) == 0 {
		return
	}

	k := llmReRankTopK
	if maxResults > 0 && maxResults < k {
		k = maxResults
	}
	if len(scored) < k {
		k = len(scored)
	}
	if k <= 1 {
		return
	}

	candidates := make([]JobReRankCandidate, k)
	for i := 0; i < k; i++ {
		job := scored[i].posting
		candidates[i] = JobReRankCandidate{
			Index:       i + 1,
			JobID:       job.ID,
			Title:       job.Title,
			Company:     "",
			Location:    job.Location,
			Department:  job.Department,
			RemoteType:  job.RemoteType,
			Description: job.Description,
			BaseScore:   scored[i].score,
		}
	}

	prompt := BuildJobReRankingPrompt(input, candidates)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, llmReRankTimeout)
	defer cancel()

	raw, err := s.copilot.RunPrompt(ctxWithTimeout, prompt)
	if err != nil {
		s.logger.Warn("job re-rank LLM failed", map[string]interface{}{"error": err.Error()})
		return
	}

	llmScores := ParseJobReRankingScores(raw, len(candidates))
	if len(llmScores) == 0 {
		return
	}

	for i := 0; i < k; i++ {
		base := scored[i].score
		llmScore, ok := llmScores[i+1]
		if !ok {
			continue
		}
		// If LLM assigns 0, it means the job is in a completely different industry/category
		// from the candidate's background. Set the score to 0 to filter it out.
		if llmScore == 0 {
			scored[i].score = 0
			continue
		}
		combined := llmReRankAlpha*base + llmReRankBeta*llmScore
		if combined < 0 {
			combined = 0
		}
		scored[i].score = math.Round(combined*100) / 100
	}

	// Re-sort only the top K segment by the new combined score, while preserving
	// the original tie-breaker by recency.
	sort.Slice(scored[:k], func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].posting.LastSeenAt.After(scored[j].posting.LastSeenAt)
		}
		return scored[i].score > scored[j].score
	})
}
