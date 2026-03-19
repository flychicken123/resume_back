package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"resumeai/models"
	"resumeai/utils"
)

// ErrAlreadyRunning is returned by Trigger when a backfill is already in progress.
var ErrAlreadyRunning = errors.New("backfill already running")

// ErrEmbeddingSvcNotConfigured is returned by Trigger when no embedding service is available.
var ErrEmbeddingSvcNotConfigured = errors.New("embedding service not configured")

// JobEmbeddingBackfillStatus holds the observable state of a backfill run.
type JobEmbeddingBackfillStatus struct {
	Running    bool       `json:"running"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Processed  int        `json:"processed"`
	Errors     int        `json:"errors"`
	Total      int        `json:"total"`
	SinceDays  int        `json:"since_days"`
}

// JobEmbeddingBackfillService generates embeddings for all active job postings
// that are missing one. Safe to trigger concurrently — only one run at a time.
type JobEmbeddingBackfillService struct {
	mu           sync.Mutex
	status       JobEmbeddingBackfillStatus
	cancel       context.CancelFunc
	embeddingSvc *EmbeddingService
	postings     *models.JobPostingModel
	logger       *utils.Logger
}

// NewJobEmbeddingBackfillService constructs a JobEmbeddingBackfillService.
func NewJobEmbeddingBackfillService(postings *models.JobPostingModel, embeddingSvc *EmbeddingService, logger *utils.Logger) *JobEmbeddingBackfillService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &JobEmbeddingBackfillService{
		embeddingSvc: embeddingSvc,
		postings:     postings,
		logger:       logger,
	}
}

// Trigger starts a background backfill run. Returns ErrEmbeddingSvcNotConfigured if
// the embedding service is nil, or ErrAlreadyRunning if a run is already in progress.
// When sinceDays > 0, only jobs posted within the last sinceDays days are embedded.
func (s *JobEmbeddingBackfillService) Trigger(ctx context.Context, batchSize int, sinceDays int) error {
	if s.embeddingSvc == nil {
		return ErrEmbeddingSvcNotConfigured
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	now := time.Now()
	s.status.Processed = 0
	s.status.Errors = 0
	s.status.Total = 0
	s.status.FinishedAt = nil
	s.status.Running = true
	s.status.StartedAt = &now
	s.status.SinceDays = sinceDays
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go s.run(runCtx, batchSize, sinceDays)
	return nil
}

// Stop cancels a running backfill. Safe to call when no backfill is running.
func (s *JobEmbeddingBackfillService) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Status returns a snapshot of the current backfill run state.
func (s *JobEmbeddingBackfillService) Status() JobEmbeddingBackfillStatus {
	s.mu.Lock()
	snap := s.status
	s.mu.Unlock()
	return snap
}

func (s *JobEmbeddingBackfillService) run(ctx context.Context, batchSize int, sinceDays int) {
	defer func() {
		finishedAt := time.Now()
		s.mu.Lock()
		s.status.Running = false
		s.status.FinishedAt = &finishedAt
		s.mu.Unlock()
	}()

	// Populate total count before starting
	total, err := s.postings.CountActiveWithNullEmbedding(sinceDays)
	if err != nil {
		total = -1
	}
	s.mu.Lock()
	s.status.Total = total
	s.mu.Unlock()

	for {
		ids, err := s.postings.ListActiveWithNullEmbedding(batchSize, sinceDays)
		if err != nil {
			s.logger.Error("backfill: list failed", err)
			return
		}
		if len(ids) == 0 {
			break
		}

		successesThisBatch := 0

		for _, id := range ids {
			select {
			case <-ctx.Done():
				return
			default:
			}

			posting, err := s.postings.GetByIDForEmbedding(ctx, id)
			if err != nil {
				s.logger.Warn("backfill: per-ID error", map[string]interface{}{"id": id, "error": err.Error()})
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			var vec []float32
			var embedErr error
			for attempt := 0; attempt <= 3; attempt++ {
				if attempt > 0 {
					backoff := time.Duration(1<<uint(attempt-1)) * time.Second
					select {
					case <-time.After(backoff):
					case <-ctx.Done():
						return
					}
				}
				vec, embedErr = s.embeddingSvc.EmbedJobPosting(ctx, posting)
				if embedErr == nil {
					break
				}
				var apiErr *googleapi.Error
				if errors.As(embedErr, &apiErr) && apiErr.Code == 429 {
					continue
				}
				if status.Code(embedErr) == codes.ResourceExhausted {
					continue
				}
				break
			}

			if embedErr != nil {
				s.logger.Warn("backfill: per-ID error", map[string]interface{}{"id": id, "error": embedErr.Error()})
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}
			if len(vec) == 0 {
				s.logger.Warn("backfill: per-ID error", map[string]interface{}{"id": id, "error": "empty vector"})
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			if err := s.postings.UpdateEmbedding(ctx, id, vec); err != nil {
				s.logger.Warn("backfill: per-ID error", map[string]interface{}{"id": id, "error": err.Error()})
				s.mu.Lock()
				s.status.Errors++
				s.mu.Unlock()
				continue
			}

			s.mu.Lock()
			s.status.Processed++
			s.mu.Unlock()
			successesThisBatch++

			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}

		if successesThisBatch == 0 {
			s.logger.Warn("backfill: zero progress in batch, stopping", map[string]interface{}{"batch_size": len(ids)})
			break
		}
	}
}
