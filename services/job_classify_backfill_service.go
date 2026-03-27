package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

var (
	ErrClassifyAlreadyRunning = errors.New("classify backfill already running")
)

// JobClassifyBackfillStatus tracks the progress of the classify backfill.
type JobClassifyBackfillStatus struct {
	Running    bool       `json:"running"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Processed  int        `json:"processed"`
	Errors     int        `json:"errors"`
	Total      int        `json:"total"`
	SinceDays  int        `json:"since_days"`
}

// JobClassifyBackfillService runs background job classification.
type JobClassifyBackfillService struct {
	mu       sync.Mutex
	status   JobClassifyBackfillStatus
	cancel   context.CancelFunc
	postings *models.JobPostingModel
	logger   *utils.Logger
}

// NewJobClassifyBackfillService creates a new classify backfill service.
func NewJobClassifyBackfillService(postings *models.JobPostingModel, logger *utils.Logger) *JobClassifyBackfillService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &JobClassifyBackfillService{
		postings: postings,
		logger:   logger,
	}
}

// Trigger starts the classify backfill in a background goroutine.
func (s *JobClassifyBackfillService) Trigger(ctx context.Context, batchSize int, sinceDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status.Running {
		return ErrClassifyAlreadyRunning
	}

	if batchSize <= 0 {
		batchSize = 20
	}

	total, err := s.postings.CountActiveWithNullClassification(sinceDays)
	if err != nil {
		return err
	}

	now := time.Now()
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.status = JobClassifyBackfillStatus{
		Running:   true,
		StartedAt: &now,
		Total:     total,
		SinceDays: sinceDays,
	}

	go s.run(runCtx, batchSize, sinceDays)
	return nil
}

// Stop cancels the running backfill.
func (s *JobClassifyBackfillService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// Status returns a snapshot of the current backfill status.
func (s *JobClassifyBackfillService) Status() JobClassifyBackfillStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *JobClassifyBackfillService) run(ctx context.Context, batchSize int, sinceDays int) {
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		now := time.Now()
		s.status.FinishedAt = &now
		s.mu.Unlock()
		s.logger.Info("classify backfill finished", map[string]interface{}{
			"processed": s.status.Processed,
			"errors":    s.status.Errors,
		})
	}()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("classify backfill stopped by cancellation", nil)
			return
		default:
		}

		ids, err := s.postings.ListActiveWithNullClassification(batchSize, sinceDays)
		if err != nil {
			s.logger.Warn("classify backfill: failed to list jobs", map[string]interface{}{"error": err.Error()})
			return
		}
		if len(ids) == 0 {
			s.logger.Info("classify backfill: all jobs classified", nil)
			return
		}

		for _, id := range ids {
			select {
			case <-ctx.Done():
				return
			default:
			}

			job, err := s.postings.GetByIDForEmbedding(ctx, id)
			if err != nil || job == nil {
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			prompt := BuildJobClassificationPrompt(job.Title, job.Description)

			var raw string
			var callErr error
			// Retry with exponential backoff on rate limit
			for attempt := 0; attempt < 3; attempt++ {
				raw, callErr = CallGeminiFlashWithTemperature(prompt, 0.0)
				if callErr == nil {
					break
				}
				errMsg := strings.ToLower(callErr.Error())
				if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "resource_exhausted") || strings.Contains(errMsg, "rate") {
					backoff := time.Duration(1<<uint(attempt)) * 5 * time.Second
					s.logger.Warn("classify backfill: rate limited, backing off", map[string]interface{}{
						"jobID":   id,
						"backoff": backoff.String(),
					})
					select {
					case <-time.After(backoff):
					case <-ctx.Done():
						return
					}
				} else {
					break
				}
			}

			if callErr != nil {
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			field, skills, seniority := ParseJobClassificationResponse(raw)
			if err := s.postings.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			s.mu.Lock()
			s.status.Processed++
			s.mu.Unlock()
		}
	}
}
