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

const maxClassifyBackfillErrorLogs = 100

// JobClassifyBackfillErrorLog stores a recent failure from a classify backfill run.
type JobClassifyBackfillErrorLog struct {
	At      time.Time `json:"at"`
	JobID   int64     `json:"job_id,omitempty"`
	Message string    `json:"message"`
}

// JobClassifyBackfillStatus tracks the progress of the classify backfill.
type JobClassifyBackfillStatus struct {
	Running    bool                          `json:"running"`
	StartedAt  *time.Time                    `json:"started_at,omitempty"`
	FinishedAt *time.Time                    `json:"finished_at,omitempty"`
	Processed  int                           `json:"processed"`
	Errors     int                           `json:"errors"`
	ErrorLogs  []JobClassifyBackfillErrorLog `json:"error_logs,omitempty"`
	Total      int                           `json:"total"`
	SinceDays  int                           `json:"since_days"`
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

// Cancel stops the running backfill and reports whether a run was active.
// Returns false if no run was in progress, true if a run was cancelled.
// Safe to call multiple times; only the first call after a Trigger returns true.
func (s *JobClassifyBackfillService) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil {
		return false
	}
	s.cancel()
	s.cancel = nil
	return true
}

// setRunningForTest seeds run state for unit tests without spawning a goroutine.
func (s *JobClassifyBackfillService) setRunningForTest(ctx context.Context, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = cancel
	s.status.Running = true
	_ = ctx
}

// Status returns a snapshot of the current backfill status.
func (s *JobClassifyBackfillService) Status() JobClassifyBackfillStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	if len(status.ErrorLogs) > 0 {
		status.ErrorLogs = append([]JobClassifyBackfillErrorLog(nil), status.ErrorLogs...)
	}
	return status
}

func (s *JobClassifyBackfillService) recordError(jobID int64, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "unknown classify backfill error"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.Errors++
	s.status.ErrorLogs = append(s.status.ErrorLogs, JobClassifyBackfillErrorLog{
		At:      time.Now(),
		JobID:   jobID,
		Message: message,
	})
	if len(s.status.ErrorLogs) > maxClassifyBackfillErrorLogs {
		s.status.ErrorLogs = append([]JobClassifyBackfillErrorLog(nil), s.status.ErrorLogs[len(s.status.ErrorLogs)-maxClassifyBackfillErrorLogs:]...)
	}
}

func (s *JobClassifyBackfillService) run(ctx context.Context, batchSize int, sinceDays int) {
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		now := time.Now()
		s.status.FinishedAt = &now
		processed := s.status.Processed
		errors := s.status.Errors
		s.mu.Unlock()
		s.logger.Info("classify backfill finished", map[string]interface{}{
			"processed": processed,
			"errors":    errors,
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
			s.recordError(0, "failed to list jobs: "+err.Error())
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
				if err != nil {
					s.recordError(id, "failed to fetch job: "+err.Error())
				} else {
					s.recordError(id, "failed to fetch job: not found")
				}
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
				s.recordError(id, "failed to classify job: "+callErr.Error())
				continue
			}

			field, skills, seniority := ParseJobClassificationResponse(raw)
			if err := s.postings.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
				s.recordError(id, "failed to save classification: "+err.Error())
				continue
			}

			s.mu.Lock()
			s.status.Processed++
			s.mu.Unlock()
		}
	}
}
