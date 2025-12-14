package services

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"resumeai/models"
	"resumeai/parsers"
	"resumeai/utils"
)

type ResumeProfileBackfillService struct {
	db        *sql.DB
	resumes   *models.ResumeModel
	s3        *S3Service
	logger    *utils.Logger
	extractor *parsers.PDFExtractor
	parser    *parsers.ResumeParser

	mu     sync.Mutex
	status ResumeProfileBackfillStatus
}

type ResumeProfileBackfillStatus struct {
	Running    bool                         `json:"running"`
	StartedAt  *time.Time                   `json:"started_at,omitempty"`
	FinishedAt *time.Time                   `json:"finished_at,omitempty"`
	LastError  string                       `json:"last_error,omitempty"`
	LastResult *ResumeProfileBackfillResult `json:"last_result,omitempty"`
}

type ResumeProfileBackfillResult struct {
	RequestedLimit  int  `json:"requested_limit"`
	DryRun          bool `json:"dry_run"`
	UsersConsidered int  `json:"users_considered"`
	ProfilesCreated int  `json:"profiles_created"`
	Errors          int  `json:"errors"`
}

type ResumeProfileBackfillRequest struct {
	Confirm string `json:"confirm"`
	Limit   int    `json:"limit"`
	DryRun  bool   `json:"dry_run"`
}

func NewResumeProfileBackfillService(db *sql.DB, resumes *models.ResumeModel, s3 *S3Service, logger *utils.Logger) *ResumeProfileBackfillService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &ResumeProfileBackfillService{
		db:        db,
		resumes:   resumes,
		s3:        s3,
		logger:    logger,
		extractor: parsers.NewPDFExtractor(),
		parser:    parsers.NewResumeParser(),
	}
}

func (s *ResumeProfileBackfillService) Status() ResumeProfileBackfillStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *ResumeProfileBackfillService) Trigger(limit int, dryRun bool) (ResumeProfileBackfillStatus, error) {
	if s == nil || s.db == nil || s.resumes == nil || s.s3 == nil {
		return ResumeProfileBackfillStatus{}, errors.New("backfill service not configured")
	}

	s.mu.Lock()
	if s.status.Running {
		current := s.status
		s.mu.Unlock()
		return current, errors.New("backfill already running")
	}
	started := time.Now().UTC()
	s.status = ResumeProfileBackfillStatus{
		Running:   true,
		StartedAt: &started,
	}
	current := s.status
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		result, err := s.run(ctx, limit, dryRun)
		finished := time.Now().UTC()

		s.mu.Lock()
		defer s.mu.Unlock()
		s.status.Running = false
		s.status.FinishedAt = &finished
		if err != nil {
			s.status.LastError = err.Error()
		} else {
			s.status.LastError = ""
		}
		s.status.LastResult = result
	}()

	return current, nil
}

type backfillCandidate struct {
	UserID    int
	HistoryID int
	S3Path    string
}

func (s *ResumeProfileBackfillService) run(ctx context.Context, limit int, dryRun bool) (*ResumeProfileBackfillResult, error) {
	candidates, err := s.listCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}

	result := &ResumeProfileBackfillResult{
		RequestedLimit:  limit,
		DryRun:          dryRun,
		UsersConsidered: len(candidates),
	}

	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		key := strings.TrimSpace(candidate.S3Path)
		if key == "" {
			result.Errors++
			continue
		}

		data, err := s.s3.DownloadBytes(key)
		if err != nil {
			result.Errors++
			s.logger.Warn("resume backfill: s3 download failed", map[string]interface{}{"user_id": candidate.UserID, "history_id": candidate.HistoryID, "error": err.Error()})
			continue
		}

		tmp, err := os.CreateTemp("", "resume-history-*.pdf")
		if err != nil {
			result.Errors++
			continue
		}
		tmpName := tmp.Name()
		_ = tmp.Close()
		if writeErr := os.WriteFile(tmpName, data, 0600); writeErr != nil {
			_ = os.Remove(tmpName)
			result.Errors++
			continue
		}

		text, err := s.extractor.ExtractText(tmpName)
		_ = os.Remove(tmpName)
		if err != nil {
			result.Errors++
			s.logger.Warn("resume backfill: pdf extract failed", map[string]interface{}{"user_id": candidate.UserID, "history_id": candidate.HistoryID, "error": err.Error()})
			continue
		}

		parsed, err := s.parser.Parse(text)
		if err != nil {
			result.Errors++
			s.logger.Warn("resume backfill: parse failed", map[string]interface{}{"user_id": candidate.UserID, "history_id": candidate.HistoryID, "error": err.Error()})
			continue
		}

		input := resumeProfileInputFromParsed(parsed)
		if dryRun {
			continue
		}

		if _, err := s.resumes.SaveProfile(ctx, candidate.UserID, input); err != nil {
			result.Errors++
			s.logger.Warn("resume backfill: save profile failed", map[string]interface{}{"user_id": candidate.UserID, "history_id": candidate.HistoryID, "error": err.Error()})
			continue
		}
		result.ProfilesCreated++
	}

	return result, nil
}

func (s *ResumeProfileBackfillService) listCandidates(ctx context.Context, limit int) ([]backfillCandidate, error) {
	if limit < 0 {
		limit = 0
	}
	// Hard cap to avoid accidental enormous runs over HTTP.
	if limit == 0 || limit > 500 {
		limit = 500
	}

	query := `
		SELECT DISTINCT ON (rh.user_id)
			rh.user_id,
			rh.id,
			rh.s3_path
		FROM resume_history rh
		LEFT JOIN resumes r ON r.user_id = rh.user_id
		WHERE r.user_id IS NULL
		ORDER BY rh.user_id, rh.generated_at DESC
		LIMIT $1
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []backfillCandidate
	for rows.Next() {
		var candidate backfillCandidate
		if err := rows.Scan(&candidate.UserID, &candidate.HistoryID, &candidate.S3Path); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func resumeProfileInputFromParsed(parsed *parsers.ResumeData) models.ResumeProfileSaveInput {
	input := models.ResumeProfileSaveInput{
		Name:           strings.TrimSpace(parsed.Name),
		Email:          strings.TrimSpace(parsed.Email),
		Phone:          strings.TrimSpace(parsed.Phone),
		Summary:        strings.TrimSpace(parsed.Summary),
		SelectedFormat: "",
		Skills:         parsed.Skills,
		Experiences:    nil,
	}

	if len(parsed.Experience) > 0 {
		exps := make([]models.ExperienceInput, 0, len(parsed.Experience))
		for _, exp := range parsed.Experience {
			city, state := splitLocation(exp.Location)
			exps = append(exps, models.ExperienceInput{
				JobTitle:         strings.TrimSpace(exp.Role),
				Company:          strings.TrimSpace(exp.Company),
				City:             city,
				State:            state,
				StartDate:        strings.TrimSpace(exp.StartDate),
				EndDate:          strings.TrimSpace(exp.EndDate),
				CurrentlyWorking: looksCurrent(exp.EndDate),
				Description:      strings.TrimSpace(strings.Join(exp.Bullets, "\n")),
			})
		}
		input.Experiences = exps
	}

	if len(parsed.Education) > 0 {
		var b strings.Builder
		for _, edu := range parsed.Education {
			line := strings.TrimSpace(strings.Join([]string{edu.Degree, edu.Field, edu.School}, " - "))
			line = strings.Trim(line, " -")
			if line == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(line)
		}
		input.Education = b.String()
	}

	return input
}

func looksCurrent(endDate string) bool {
	v := strings.ToLower(strings.TrimSpace(endDate))
	return v == "present" || v == "current" || v == "now"
}

func splitLocation(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.Split(raw, ",")
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), ""
	}
	city := strings.TrimSpace(parts[0])
	state := strings.TrimSpace(parts[1])
	return city, state
}
