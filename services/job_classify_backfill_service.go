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
	ErrClassifyDisabled       = errors.New("job auto-classification disabled")
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
	throttle ClassifyThrottleConfig
}

// NewJobClassifyBackfillService creates a new classify backfill service.
func NewJobClassifyBackfillService(postings *models.JobPostingModel, logger *utils.Logger, throttle ...ClassifyThrottleConfig) *JobClassifyBackfillService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	cfg := ClassifyThrottleConfig{
		PerJobDelayMS: 500,
		BatchSize:     20,
	}
	if len(throttle) > 0 {
		if throttle[0].PerJobDelayMS > 0 {
			cfg.PerJobDelayMS = throttle[0].PerJobDelayMS
		}
		if throttle[0].BatchSize > 0 {
			cfg.BatchSize = throttle[0].BatchSize
		}
	}
	return &JobClassifyBackfillService{
		postings: postings,
		logger:   logger,
		throttle: cfg,
	}
}

// Trigger starts the classify backfill in a background goroutine.
func (s *JobClassifyBackfillService) Trigger(ctx context.Context, batchSize int, sinceDays int) error {
	if JobAutoClassificationDisabled() {
		return ErrClassifyDisabled
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status.Running {
		return ErrClassifyAlreadyRunning
	}

	if batchSize <= 0 {
		batchSize = s.throttle.BatchSize
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
	jobClassificationRunMu.Lock()
	defer jobClassificationRunMu.Unlock()

	failedIDs := make(map[int64]struct{})
	excludedIDs := make([]int64, 0)
	recordJobError := func(jobID int64, message string) {
		s.recordError(jobID, message)
		if jobID == 0 {
			return
		}
		if _, exists := failedIDs[jobID]; exists {
			return
		}
		failedIDs[jobID] = struct{}{}
		excludedIDs = append(excludedIDs, jobID)
	}

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

		ids, err := s.postings.ListActiveWithNullClassificationExcluding(batchSize, sinceDays, excludedIDs)
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
					recordJobError(id, "failed to fetch job: "+err.Error())
				} else {
					recordJobError(id, "failed to fetch job: not found")
				}
				if !s.sleepBetweenJobs(ctx) {
					return
				}
				continue
			}

			prompt := BuildJobClassificationPrompt(job.Title, job.Description)

			llmCtx, cancel := context.WithTimeout(ctx, jobClassificationLLMTimeout)
			raw, callErr := CallWithRateLimitRetry(llmCtx, DefaultRetryConfig(),
				func(ctx context.Context) (string, error) {
					return CallGeminiFlashWithTemperatureContext(ctx, prompt, 0.0)
				},
				RetryOpts{Endpoint: "classify_job_backfill", Logger: s.logger},
			)
			cancel()
			if callErr != nil {
				field, skills, seniority := fallbackJobClassification(job)
				if err := validateJobClassificationResult(field, skills, seniority); err != nil {
					recordJobError(id, "failed to classify job: "+callErr.Error())
					if !s.sleepBetweenJobs(ctx) {
						return
					}
					continue
				}
				s.logger.Warn("classify backfill: AI failed; saving fallback classification", map[string]interface{}{
					"job_id": id,
					"error":  callErr.Error(),
				})
				if err := s.postings.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
					recordJobError(id, "failed to save fallback classification: "+err.Error())
					if !s.sleepBetweenJobs(ctx) {
						return
					}
					continue
				}
				s.mu.Lock()
				s.status.Processed++
				s.mu.Unlock()
				if !s.sleepBetweenJobs(ctx) {
					return
				}
				continue
			}

			field, skills, seniority := ParseJobClassificationResponse(raw)
			field, skills, seniority = finalizeJobClassification(job, field, skills, seniority)
			if err := validateJobClassificationResult(field, skills, seniority); err != nil {
				recordJobError(id, err.Error())
				if !s.sleepBetweenJobs(ctx) {
					return
				}
				continue
			}
			if err := s.postings.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
				recordJobError(id, "failed to save classification: "+err.Error())
				if !s.sleepBetweenJobs(ctx) {
					return
				}
				continue
			}

			s.mu.Lock()
			s.status.Processed++
			s.mu.Unlock()
			if !s.sleepBetweenJobs(ctx) {
				return
			}
		}
	}
}

func (s *JobClassifyBackfillService) sleepBetweenJobs(ctx context.Context) bool {
	delayMS := s.throttle.PerJobDelayMS
	if delayMS <= 0 {
		return true
	}
	select {
	case <-time.After(time.Duration(delayMS) * time.Millisecond):
		return true
	case <-ctx.Done():
		return false
	}
}
