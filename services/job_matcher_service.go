package services

import (
	"context"
	"math"
	"sort"
	"strings"
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
}

// ResumeJobMatcher defines the behaviour exposed to handlers.
type ResumeJobMatcher interface {
	MatchAndStore(ctx context.Context, input ResumeJobMatchInput) ([]*models.ResumeJobMatchRecord, error)
}

type jobMatcherService struct {
	postings *models.JobPostingModel
	matches  *models.ResumeJobMatchModel
	logger   *utils.Logger
	copilot  *CopilotAgent
}

// NewJobMatcherService wires a job matcher service.
func NewJobMatcherService(postings *models.JobPostingModel, matches *models.ResumeJobMatchModel, logger *utils.Logger, copilot *CopilotAgent) ResumeJobMatcher {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &jobMatcherService{
		postings: postings,
		matches:  matches,
		logger:   logger,
		copilot:  copilot,
	}
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
	llmReRankTopK            = 15 // Reduced from 30 for faster response
	llmReRankAlpha           = 0.7
	llmReRankBeta            = 0.3
	llmReRankTimeout         = 15 * time.Second
	llmSkillInferenceTimeout = 15 * time.Second
	defaultMaxResults        = 50
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
	// Pass 1: SQL-level filter by skills/position (broader pool)
	// Pass 2: Full heuristic scoring on filtered set

	limit := input.CandidateJobLimit
	if limit <= 0 {
		limit = 1000 // Increased default for two-pass approach
	}

	position := strings.ToLower(strings.TrimSpace(input.Position))
	skills := normaliseSkills(input.Skills)

	// Pass 1: Get jobs filtered by relevance (skills/position match in SQL)
	jobs, err := s.postings.ListActiveByRelevance(skills, input.Position, limit)
	if err != nil {
		// Fallback to original method if new method fails
		s.logger.Warn("ListActiveByRelevance failed, falling back to ListActive", map[string]interface{}{"error": err.Error()})
		jobs, err = s.postings.ListActive(nil, limit)
		if err != nil {
			return nil, err
		}
	}

	// Pass 2: Full heuristic scoring
	sourceText := strings.Join([]string{input.Summary, input.Experience, input.Education, input.JobDescription}, " ")
	resumeText := strings.ToLower(strings.TrimSpace(sourceText))
	keywords := extractKeywords(resumeText, 40)
	preferredLocation := strings.ToLower(strings.TrimSpace(input.PreferredLocation))
	if preferredLocation == "" {
		preferredLocation = "united states"
	}
	candidateLevel := determineCandidateSeniority(input.Position, sourceText)

	scored := make([]scoredJob, 0, len(jobs))
	for _, job := range jobs {
		if !locationCompatible(preferredLocation, job) {
			continue
		}

		jobLevel := determineJobSeniority(job.Title, job.Description)
		if jobLevel == seniorityIntern {
			continue
		}
		if candidateLevel >= senioritySenior && jobLevel <= seniorityEntry {
			continue
		}
		if candidateLevel >= seniorityLead && jobLevel < seniorityLead {
			continue
		}

		score := computeMatchScore(job, position, skills, keywords, resumeText, preferredLocation)
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

	// Wait for skill inference result (with fallback)
	var userSkills []string
	select {
	case sr := <-skillChan:
		if sr.err != nil {
			// Fallback to rule-based inference
			userSkills = ExpandSkillsWithInference(input.Skills)
		} else {
			userSkills = sr.skills
		}
	case <-time.After(llmSkillInferenceTimeout + time.Second):
		// Timeout - use rule-based fallback
		userSkills = ExpandSkillsWithInference(input.Skills)
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

func computeMatchScore(job *models.JobPosting, position string, skills []string, keywords []string, resumeText string, preferredLocation string) float64 {
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

	daysSincePosted := time.Since(postDate).Hours() / 24

	switch {
	case daysSincePosted <= 3:
		return 4.0 // Very fresh: +4 points
	case daysSincePosted <= 7:
		return 3.0 // Fresh: +3 points
	case daysSincePosted <= 14:
		return 2.0 // Recent: +2 points
	case daysSincePosted <= 30:
		return 1.0 // Within month: +1 point
	default:
		return 0.0 // Older jobs: no boost
	}
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
