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
}

// NewJobMatcherService wires a job matcher service.
func NewJobMatcherService(postings *models.JobPostingModel, matches *models.ResumeJobMatchModel, logger *utils.Logger) ResumeJobMatcher {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &jobMatcherService{postings: postings, matches: matches, logger: logger}
}

type scoredJob struct {
	posting *models.JobPosting
	score   float64
}

// MatchAndStore evaluates current postings, stores matches, and returns the enriched view.
func (s *jobMatcherService) MatchAndStore(ctx context.Context, input ResumeJobMatchInput) ([]*models.ResumeJobMatchRecord, error) {
	if input.UserID <= 0 {
		return nil, ErrInvalidUserID
	}
	if strings.TrimSpace(input.ResumeHash) == "" {
		return nil, ErrMissingResumeHash
	}

	limit := input.CandidateJobLimit
	if limit <= 0 {
		limit = 200
	}

	jobs, err := s.postings.ListActive(nil, limit)
	if err != nil {
		return nil, err
	}

	position := strings.ToLower(strings.TrimSpace(input.Position))
	skills := normaliseSkills(input.Skills)
	sourceText := strings.Join([]string{input.Summary, input.Experience, input.Education, input.JobDescription}, " ")
	resumeText := strings.ToLower(strings.TrimSpace(sourceText))
	keywords := extractKeywords(resumeText, 40)
	preferredLocation := strings.ToLower(strings.TrimSpace(input.PreferredLocation))

	scored := make([]scoredJob, 0, len(jobs))
	for _, job := range jobs {
		if !utils.IsSupportedJobLocation(job.Location, job.RemoteType) {
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
		maxResults = 25
	}
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
	return results, nil
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

	score := 0.0
	skillMatches := 0

	for _, skill := range skills {
		if skill == "" {
			continue
		}
		matched := false
		if title != "" && strings.Contains(title, skill) {
			score += 5
			matched = true
		}
		if !matched && description != "" && strings.Contains(description, skill) {
			score += 3
			matched = true
		}
		if !matched && department != "" && strings.Contains(department, skill) {
			score += 1.5
			matched = true
		}
		if matched {
			skillMatches++
		}
	}

	if position != "" && title != "" && strings.Contains(title, position) {
		score += 4
	}

	if len(keywords) > 0 && description != "" {
		keywordHits := 0
		for _, keyword := range keywords {
			if strings.Contains(description, keyword) {
				keywordHits++
			}
		}
		if keywordHits > 0 {
			score += math.Min(float64(keywordHits)*0.6, 6)
		}
	}

	if strings.Contains(resumeText, "remote") {
		if strings.Contains(remote, "remote") || strings.Contains(location, "remote") {
			score += 1
		}
	}

	if preferredLocation != "" {
		score += computeLocationBoost(location, remote, preferredLocation)
	}

	if skillMatches == 0 && (position == "" || !strings.Contains(title, position)) {
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
