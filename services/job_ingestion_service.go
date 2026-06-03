package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"resumeai/models"
	"resumeai/utils"
)

type JobSyncResult struct {
	JobsFound   int
	JobsCreated int
	JobsUpdated int
	JobsClosed  int
}

type UnsupportedATSProviderError struct {
	CompanyID int
	Provider  string
}

func (e *UnsupportedATSProviderError) Error() string {
	return fmt.Sprintf("unsupported ATS provider: %s", strings.TrimSpace(e.Provider))
}

func shouldKeepCompanyActiveAfterSyncFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transientMarkers := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"timeout exceeded",
		"temporary failure",
		"connection reset",
		"connection refused",
		"tls handshake timeout",
		" returned status 429",
		" returned status 500",
		" returned status 502",
		" returned status 503",
		" returned status 504",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func shouldLogSyncFailureAsError(err error) bool {
	if err == nil {
		return false
	}
	var unsupported *UnsupportedATSProviderError
	if errors.As(err, &unsupported) {
		return false
	}
	if shouldKeepCompanyActiveAfterSyncFailure(err) {
		return false
	}

	msg := strings.ToLower(err.Error())
	softMarkers := []string{
		" returned status 401",
		" returned status 403",
		" returned status 404",
		" returned status 405",
		" returned status 422",
		" returned status 429",
		" returned status 500",
		" returned status 502",
		" returned status 503",
		" returned status 504",
		" could not determine ",
		" could not derive ",
		" board token is required",
		" must be a greenhouse job board url",
		" does not resolve to a workday host",
		" maintenance-page",
		" context deadline exceeded",
		" client.timeout exceeded",
	}
	for _, marker := range softMarkers {
		if strings.Contains(msg, marker) {
			return false
		}
	}

	return true
}

// JobSyncBatchResult captures the aggregate outcome of syncing multiple companies
type JobSyncBatchResult struct {
	TotalCompanies   int                    `json:"total_companies"`
	Ran              int                    `json:"ran"`
	Succeeded        int                    `json:"succeeded"`
	Failed           int                    `json:"failed"`
	TotalJobsFound   int                    `json:"total_jobs_found"`
	TotalJobsCreated int                    `json:"total_jobs_created"`
	TotalJobsUpdated int                    `json:"total_jobs_updated"`
	TotalJobsClosed  int                    `json:"total_jobs_closed"`
	CompanyResults   []JobSyncCompanyResult `json:"company_results"`
}

// JobSyncCompanyResult captures the outcome for a single company sync run
type JobSyncCompanyResult struct {
	CompanyID   int    `json:"company_id"`
	CompanyName string `json:"company_name"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	JobsFound   int    `json:"jobs_found"`
	JobsCreated int    `json:"jobs_created"`
	JobsUpdated int    `json:"jobs_updated"`
	JobsClosed  int    `json:"jobs_closed"`
}

type ATSProvider interface {
	FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error)
}

type ClassifyThrottleConfig struct {
	PerJobDelayMS int
	BatchSize     int
}

type JobIngestionService struct {
	companyModel         *models.JobCompanyModel
	postingModel         *models.JobPostingModel
	syncRunModel         *models.JobSyncRunModel
	providers            map[string]ATSProvider
	logger               *utils.Logger
	clock                func() time.Time
	embeddingSvc         *EmbeddingService
	ctx                  context.Context
	embedMu              sync.Mutex
	embedRunning         bool
	throttle             ClassifyThrottleConfig
	classifyPaused       atomic.Bool
	classifyOneFn        func(ctx context.Context, id int64) error
	classifyQueueMu      sync.Mutex
	classifyQueued       map[int64]struct{}
	classifyInFlight     map[int64]struct{}
	classifyQueueRunning bool
	backlogMu            sync.Mutex
	backlogRunning       bool
	testLogger           retryLogger

	syncAllMu     sync.Mutex
	syncAllStatus SyncAllStatus
	syncAllCancel context.CancelFunc
}

// SyncAllStatus is the snapshot of an async sync-all run.
type SyncAllStatus struct {
	Running    bool                `json:"running"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
	LastError  string              `json:"last_error,omitempty"`
	LastResult *JobSyncBatchResult `json:"last_result,omitempty"`
}

const (
	dailySyncInterval               = 24 * time.Hour
	autoClassifyBacklogSinceDays    = 7
	autoClassifyBacklogDefaultBatch = 20
	jobClassificationLLMTimeout     = 8 * time.Second
)

func NewJobIngestionService(db *sql.DB, logger *utils.Logger, embeddingSvc *EmbeddingService, ctx context.Context, throttle ClassifyThrottleConfig) *JobIngestionService {
	if logger == nil {
		logger = utils.NewLogger()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	service := &JobIngestionService{
		companyModel: models.NewJobCompanyModel(db),
		postingModel: models.NewJobPostingModel(db),
		syncRunModel: models.NewJobSyncRunModel(db),
		providers:    map[string]ATSProvider{},
		logger:       logger,
		clock:        time.Now,
		embeddingSvc: embeddingSvc,
		ctx:          ctx,
		throttle:     throttle,
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	leverHTTPClient := &http.Client{Timeout: 60 * time.Second}
	service.RegisterProvider("greenhouse", NewGreenhouseProvider(httpClient, logger))
	service.RegisterProvider("workday", NewWorkdayProvider(httpClient, logger))
	service.RegisterProvider("smartrecruiters", NewSmartRecruitersProvider(httpClient, logger))
	service.RegisterProvider("lever", NewLeverProvider(leverHTTPClient, logger))
	service.RegisterProvider("workable", NewWorkableProvider(httpClient, logger))
	service.RegisterProvider("ashby", NewAshbyProvider(httpClient, logger))
	service.RegisterProvider("bamboohr", NewBambooHRProvider(httpClient, logger))
	service.RegisterProvider("recruitee", NewRecruiteeProvider(httpClient, logger))
	service.RegisterProvider("teamtailor", NewTeamtailorProvider(httpClient, logger))

	return service
}

func (s *JobIngestionService) RegisterProvider(name string, provider ATSProvider) {
	if provider == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return
	}
	s.providers[key] = provider
}

func (s *JobIngestionService) SyncCompany(ctx context.Context, companyID int) (*JobSyncResult, error) {
	company, err := s.companyModel.GetByID(companyID)
	if err != nil {
		return nil, fmt.Errorf("fetching company %d: %w", companyID, err)
	}

	providerKey := strings.ToLower(strings.TrimSpace(company.ATSProvider))
	provider, ok := s.providers[providerKey]
	if !ok {
		unsupportedErr := &UnsupportedATSProviderError{CompanyID: company.ID, Provider: company.ATSProvider}
		syncedAt := s.clock()
		truncated := truncateErrorMessage(unsupportedErr.Error(), 512)
		if err := s.companyModel.RecordSyncFailure(company.ID, syncedAt, "unsupported_provider", truncated); err != nil {
			s.logger.Warn("failed recording sync failure", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
		}

		return nil, unsupportedErr
	}

	run, err := s.syncRunModel.Start(company.ID)
	if err != nil {
		return nil, fmt.Errorf("starting sync run: %w", err)
	}

	result := &JobSyncResult{}
	finish := func(status string, finishErr error) {
		msg := ""
		syncedAt := s.clock()
		if finishErr != nil {
			msg = finishErr.Error()
			truncated := truncateErrorMessage(msg, 512)
			if shouldLogSyncFailureAsError(finishErr) {
				s.logger.Error("job sync run failed", finishErr, map[string]interface{}{"company_id": company.ID, "provider": company.ATSProvider})
			} else {
				s.logger.Warn("job sync run failed", map[string]interface{}{"company_id": company.ID, "provider": company.ATSProvider, "error": finishErr.Error()})
			}
			var err error
			if shouldKeepCompanyActiveAfterSyncFailure(finishErr) {
				err = s.companyModel.RecordTransientSyncFailure(company.ID, syncedAt, status, truncated)
			} else {
				err = s.companyModel.RecordSyncFailure(company.ID, syncedAt, status, truncated)
			}
			if err != nil {
				s.logger.Warn("failed recording sync failure", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			}
		} else {
			if err := s.companyModel.UpdateSyncMetadata(company.ID, syncedAt, status); err != nil {
				s.logger.Warn("failed updating sync metadata", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			}
		}
		metrics := map[string]int{
			"found":   result.JobsFound,
			"created": result.JobsCreated,
			"updated": result.JobsUpdated,
			"closed":  result.JobsClosed,
		}
		_ = s.syncRunModel.Finish(run.ID, status, optionalString(msg), metrics)
	}

	jobs, err := provider.FetchJobs(ctx, company)
	if err != nil {
		finish("failed", err)
		return nil, err
	}

	result.JobsFound = len(jobs)

	activeIDs := make([]string, 0, len(jobs))
	newIDs := make([]int64, 0)
	classifyIDs := make([]int64, 0)
	now := s.clock()

	for _, job := range jobs {
		if job == nil {
			continue
		}
		job.CompanyID = company.ID
		inserted, needsClassification, err := s.postingModel.Upsert(job)
		if err != nil {
			finish("failed", err)
			return nil, err
		}
		if inserted {
			result.JobsCreated++
			newIDs = append(newIDs, job.ID)
		} else {
			result.JobsUpdated++
		}
		if inserted || needsClassification {
			classifyIDs = append(classifyIDs, job.ID)
		}
		activeIDs = append(activeIDs, job.ExternalJobID)
	}

	closedCount, err := s.postingModel.DeactivateMissing(company.ID, activeIDs, now)
	if err != nil {
		finish("failed", err)
		return nil, err
	}
	result.JobsClosed = int(closedCount)

	finish("success", nil)
	if len(newIDs) > 0 && s.embeddingSvc != nil {
		go s.embedJobPostingsBatch(s.ctx, newIDs)
	}
	if len(classifyIDs) > 0 {
		s.enqueueClassifyJobPostings(classifyIDs)
	}
	return result, nil
}

// PauseClassifier stops the classify loop from starting new work. Existing
// in-flight classifications complete normally.
func (s *JobIngestionService) PauseClassifier() {
	s.classifyPaused.Store(true)
}

// ResumeClassifier re-enables classification after a PauseClassifier call.
func (s *JobIngestionService) ResumeClassifier() {
	s.classifyPaused.Store(false)
}

// IsClassifierPaused reports whether the classify loop is currently paused.
func (s *JobIngestionService) IsClassifierPaused() bool {
	return s.classifyPaused.Load()
}

func (s *JobIngestionService) enqueueClassifyJobPostings(ids []int64) {
	if len(ids) == 0 || JobAutoClassificationDisabled() {
		return
	}

	s.classifyQueueMu.Lock()
	if s.classifyQueued == nil {
		s.classifyQueued = make(map[int64]struct{})
	}
	if s.classifyInFlight == nil {
		s.classifyInFlight = make(map[int64]struct{})
	}

	queued := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := s.classifyQueued[id]; exists {
			continue
		}
		if _, exists := s.classifyInFlight[id]; exists {
			continue
		}
		s.classifyQueued[id] = struct{}{}
		queued++
	}
	if queued == 0 || s.classifyQueueRunning {
		s.classifyQueueMu.Unlock()
		return
	}

	s.classifyQueueRunning = true
	s.classifyQueueMu.Unlock()

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go s.drainClassifyQueue(ctx)
}

func (s *JobIngestionService) drainClassifyQueue(ctx context.Context) {
	for {
		if ctx.Err() != nil || JobAutoClassificationDisabled() {
			s.stopClassifyQueue()
			return
		}

		ids := s.takeQueuedClassifyIDs()
		if len(ids) == 0 {
			return
		}

		s.classifyJobPostingsBatch(ctx, ids)
		s.finishQueuedClassifyIDs(ids)
	}
}

func (s *JobIngestionService) takeQueuedClassifyIDs() []int64 {
	s.classifyQueueMu.Lock()
	defer s.classifyQueueMu.Unlock()

	if len(s.classifyQueued) == 0 {
		s.classifyQueueRunning = false
		return nil
	}

	ids := make([]int64, 0, len(s.classifyQueued))
	for id := range s.classifyQueued {
		ids = append(ids, id)
		delete(s.classifyQueued, id)
		s.classifyInFlight[id] = struct{}{}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] > ids[j]
	})
	return ids
}

func (s *JobIngestionService) finishQueuedClassifyIDs(ids []int64) {
	s.classifyQueueMu.Lock()
	defer s.classifyQueueMu.Unlock()

	for _, id := range ids {
		delete(s.classifyInFlight, id)
	}
}

func (s *JobIngestionService) stopClassifyQueue() {
	s.classifyQueueMu.Lock()
	defer s.classifyQueueMu.Unlock()

	s.classifyQueueRunning = false
}

// classifyOne runs a single classification round-trip. Separated out so tests can
// swap it via classifyOneFn without exercising the real Gemini call.
func (s *JobIngestionService) classifyOne(ctx context.Context, id int64) error {
	if s.classifyOneFn != nil {
		return s.classifyOneFn(ctx, id)
	}

	job, err := s.postingModel.GetByIDForEmbedding(ctx, id)
	if err != nil || job == nil {
		return err
	}
	if job.CareerField != "" && job.Seniority != "" {
		return nil
	}

	fallbackField, fallbackSkills, fallbackSeniority := fallbackJobClassification(job)
	if err := validateJobClassificationResult(fallbackField, fallbackSkills, fallbackSeniority); err != nil {
		return err
	}
	if err := s.postingModel.UpdateJobClassification(ctx, id, string(fallbackField), fallbackSkills, fallbackSeniority); err != nil {
		return err
	}

	prompt := BuildJobClassificationPrompt(job.Title, job.Description)

	llmCtx, cancel := context.WithTimeout(ctx, jobClassificationLLMTimeout)
	defer cancel()
	raw, err := CallWithRateLimitRetry(llmCtx, DefaultRetryConfig(),
		func(ctx context.Context) (string, error) {
			return CallGeminiFlashWithTemperatureContext(ctx, prompt, 0.0)
		},
		RetryOpts{Endpoint: "classify_job", Logger: s.logger},
	)
	if err != nil {
		s.logger.Warn("job classification AI failed after fallback classification was saved", map[string]interface{}{
			"jobID": id,
			"error": err.Error(),
		})
		return nil
	}

	field, skills, seniority := ParseJobClassificationResponse(raw)
	field, skills, seniority = finalizeJobClassification(job, field, skills, seniority)
	if err := validateJobClassificationResult(field, skills, seniority); err != nil {
		return err
	}
	return s.postingModel.UpdateJobClassification(ctx, id, string(field), skills, seniority)
}

func enrichJobClassificationSkills(job *models.JobPosting, skills []string) []string {
	if job == nil {
		return skills
	}
	text := strings.Join([]string{job.Title, job.Department, job.Description}, " ")
	return normalizeJobClassificationSkills(job.ExtractedSkills, skills, ExtractSkillsFromText(text))
}

// classifyJobPostingsBatch classifies career field, extracts skills, and determines seniority.
// Throttle defaults (500ms per job, batch size 5) are applied by config.GetAppConfig;
// the service trusts its caller to pass sensible values and respects 0 as "no sleep".
func (s *JobIngestionService) classifyJobPostingsBatch(ctx context.Context, ids []int64) {
	if JobAutoClassificationDisabled() {
		return
	}
	jobClassificationRunMu.Lock()
	defer jobClassificationRunMu.Unlock()
	if JobAutoClassificationDisabled() {
		return
	}

	delayMS := s.throttle.PerJobDelayMS
	batchSize := s.throttle.BatchSize

	s.emitClassifyTick(batchSize, delayMS, len(ids))
	s.fallbackClassifyJobPostingIDs(ctx, ids)

	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if s.classifyPaused.Load() {
			return
		}

		if err := s.classifyOne(ctx, id); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("job classification failed", map[string]interface{}{
				"jobID": id,
				"error": err.Error(),
			})
		}

		if delayMS > 0 {
			select {
			case <-time.After(time.Duration(delayMS) * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *JobIngestionService) KickFallbackClassifyBacklog(ctx context.Context, sinceDays, limit int) {
	if JobAutoClassificationDisabled() || s.postingModel == nil {
		return
	}
	if limit <= 0 {
		limit = autoClassifyBacklogDefaultBatch
	}

	go func() {
		ids, err := s.postingModel.ListActiveWithNullClassificationExcluding(limit, sinceDays, nil)
		if err != nil {
			s.logger.Warn("fallback classify backlog: failed to list jobs", map[string]interface{}{"error": err.Error()})
			return
		}
		s.fallbackClassifyJobPostingIDs(ctx, ids)
	}()
}

func (s *JobIngestionService) fallbackClassifyJobPostingIDs(ctx context.Context, ids []int64) {
	if s.postingModel == nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		job, err := s.postingModel.GetByIDForEmbedding(ctx, id)
		if err != nil || job == nil {
			if err != nil {
				s.logger.Warn("fallback job classification failed to fetch posting", map[string]interface{}{
					"jobID": id,
					"error": err.Error(),
				})
			}
			continue
		}
		if job.CareerField != "" && job.Seniority != "" {
			continue
		}
		field, skills, seniority := fallbackJobClassification(job)
		if err := validateJobClassificationResult(field, skills, seniority); err != nil {
			s.logger.Warn("fallback job classification produced invalid result", map[string]interface{}{
				"jobID": id,
				"error": err.Error(),
			})
			continue
		}
		if err := s.postingModel.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
			s.logger.Warn("fallback job classification failed to save posting", map[string]interface{}{
				"jobID": id,
				"error": err.Error(),
			})
		}
	}
}

func (s *JobIngestionService) kickClassifyBacklog(ctx context.Context, sinceDays int) {
	if aiBackgroundJobsDisabled() || JobAutoClassificationDisabled() {
		return
	}

	s.backlogMu.Lock()
	if s.backlogRunning {
		s.backlogMu.Unlock()
		return
	}
	s.backlogRunning = true
	s.backlogMu.Unlock()

	go func() {
		defer func() {
			s.backlogMu.Lock()
			s.backlogRunning = false
			s.backlogMu.Unlock()
		}()
		s.classifyMissingJobPostings(ctx, sinceDays)
	}()
}

func (s *JobIngestionService) classifyMissingJobPostings(ctx context.Context, sinceDays int) {
	if aiBackgroundJobsDisabled() || JobAutoClassificationDisabled() {
		return
	}
	jobClassificationRunMu.Lock()
	defer jobClassificationRunMu.Unlock()
	if aiBackgroundJobsDisabled() || JobAutoClassificationDisabled() {
		return
	}

	batchSize := s.throttle.BatchSize
	if batchSize < autoClassifyBacklogDefaultBatch {
		batchSize = autoClassifyBacklogDefaultBatch
	}
	delayMS := s.throttle.PerJobDelayMS
	failedIDs := make(map[int64]struct{})
	excludedIDs := make([]int64, 0)

	for {
		if ctx.Err() != nil || aiBackgroundJobsDisabled() || JobAutoClassificationDisabled() || s.classifyPaused.Load() {
			return
		}

		ids, err := s.postingModel.ListActiveWithNullClassificationExcluding(batchSize, sinceDays, excludedIDs)
		if err != nil {
			s.logger.Warn("auto classify backlog: failed to list jobs", map[string]interface{}{"error": err.Error()})
			return
		}
		if len(ids) == 0 {
			return
		}

		s.emitClassifyTick(batchSize, delayMS, len(ids))
		for _, id := range ids {
			if ctx.Err() != nil || s.classifyPaused.Load() {
				return
			}

			if err := s.classifyOne(ctx, id); err != nil {
				s.logger.Warn("auto classify backlog: job classification failed", map[string]interface{}{
					"jobID": id,
					"error": err.Error(),
				})
				if _, exists := failedIDs[id]; !exists {
					failedIDs[id] = struct{}{}
					excludedIDs = append(excludedIDs, id)
				}
				continue
			}

			if delayMS > 0 {
				select {
				case <-time.After(time.Duration(delayMS) * time.Millisecond):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (s *JobIngestionService) emitClassifyTick(batchSize, delayMS, remaining int) {
	fields := map[string]interface{}{
		"batch_size":        batchSize,
		"delay_ms":          delayMS,
		"remaining_backlog": remaining,
	}
	if s.testLogger != nil {
		s.testLogger.Warn("classify_tick", fields)
		return
	}
	if s.logger != nil {
		s.logger.Info("classify_tick", fields)
	}
}

// embedJobPostingsBatch generates and stores embeddings for a set of newly inserted job IDs.
// Only one batch runs at a time; concurrent calls are silently skipped (jobs will be
// picked up by the next backfill run).
func (s *JobIngestionService) embedJobPostingsBatch(ctx context.Context, ids []int64) {
	if aiBackgroundJobsDisabled() {
		return
	}
	s.embedMu.Lock()
	if s.embedRunning {
		s.embedMu.Unlock()
		return
	}
	s.embedRunning = true
	s.embedMu.Unlock()

	defer func() {
		s.embedMu.Lock()
		s.embedRunning = false
		s.embedMu.Unlock()
	}()

	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		default:
		}

		posting, err := s.postingModel.GetByIDForEmbedding(ctx, id)
		if err != nil {
			s.logger.Warn("embedJobPostingsBatch: failed to fetch posting", map[string]interface{}{"id": id, "error": err.Error()})
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
			s.logger.Warn("embedJobPostingsBatch: embedding failed", map[string]interface{}{"id": id, "error": embedErr.Error()})
			continue
		}
		if len(vec) == 0 {
			s.logger.Warn("embedJobPostingsBatch: empty vector", map[string]interface{}{"id": id})
			continue
		}

		if err := s.postingModel.UpdateEmbedding(ctx, id, vec); err != nil {
			s.logger.Warn("embedJobPostingsBatch: failed to store embedding", map[string]interface{}{"id": id, "error": err.Error()})
			continue
		}

		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

func (s *JobIngestionService) SyncAllCompanies(ctx context.Context) error {
	_, err := s.SyncAllCompaniesWithSummary(ctx)
	return err
}

// SyncAllCompaniesWithSummary runs a sync for every active company in
// parallel and returns aggregate metrics. Concurrency is bounded by
// SYNC_WORKER_CONCURRENCY (default 8). Workers process companies sorted by
// stalest LastSyncedAt first so an interrupted run still refreshes the most
// stale companies before the freshest ones.
//
// At 2400+ companies × 5-30s each, sequential execution took 3-10 hours per
// pass and never completed before the next deploy. Concurrency 8 brings a
// full pass to 5-15 minutes.
func (s *JobIngestionService) SyncAllCompaniesWithSummary(ctx context.Context) (*JobSyncBatchResult, error) {
	companies, err := s.companyModel.ListActive()
	if err != nil {
		return nil, err
	}

	// Sort stalest-first so an interrupted run refreshes the worst-off companies first.
	sort.Slice(companies, func(i, j int) bool {
		iNever := companies[i].LastSyncedAt == nil
		jNever := companies[j].LastSyncedAt == nil
		if iNever != jNever {
			return iNever
		}
		if iNever {
			return false
		}
		return companies[i].LastSyncedAt.Before(*companies[j].LastSyncedAt)
	})

	batch := &JobSyncBatchResult{
		TotalCompanies: len(companies),
		CompanyResults: make([]JobSyncCompanyResult, 0, len(companies)),
	}

	if len(companies) == 0 {
		return batch, nil
	}

	concurrency := syncWorkerConcurrencyFromEnv()
	s.logger.Info("sync-all starting", map[string]interface{}{
		"total_companies": len(companies),
		"concurrency":     concurrency,
	})

	type pending struct {
		company *models.JobCompany
	}
	jobs := make(chan pending, len(companies))
	for i := range companies {
		jobs <- pending{company: companies[i]}
	}
	close(jobs)

	results := make(chan JobSyncCompanyResult, len(companies))
	var ran, succeeded, failed atomic.Int64
	var totalFound, totalCreated, totalUpdated, totalClosed atomic.Int64
	var processed atomic.Int64

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				if ctx.Err() != nil {
					return
				}
				company := p.company
				result := JobSyncCompanyResult{
					CompanyID:   company.ID,
					CompanyName: company.Name,
				}

				ran.Add(1)
				syncResult, syncErr := s.SyncCompany(ctx, company.ID)
				if syncErr != nil {
					var unsupported *UnsupportedATSProviderError
					if errors.As(syncErr, &unsupported) {
						result.Status = "unsupported_provider"
					} else {
						result.Status = "failed"
					}
					result.Error = syncErr.Error()
					failed.Add(1)
				} else {
					result.Status = "success"
					result.JobsFound = syncResult.JobsFound
					result.JobsCreated = syncResult.JobsCreated
					result.JobsUpdated = syncResult.JobsUpdated
					result.JobsClosed = syncResult.JobsClosed
					succeeded.Add(1)
					totalFound.Add(int64(syncResult.JobsFound))
					totalCreated.Add(int64(syncResult.JobsCreated))
					totalUpdated.Add(int64(syncResult.JobsUpdated))
					totalClosed.Add(int64(syncResult.JobsClosed))
				}
				results <- result

				if n := processed.Add(1); n%50 == 0 {
					s.logger.Info("sync-all progress", map[string]interface{}{
						"processed": n,
						"total":     len(companies),
						"succeeded": succeeded.Load(),
						"failed":    failed.Load(),
					})
				}
			}
		}()
	}

	go func() { wg.Wait(); close(results) }()
	for r := range results {
		batch.CompanyResults = append(batch.CompanyResults, r)
	}

	batch.Ran = int(ran.Load())
	batch.Succeeded = int(succeeded.Load())
	batch.Failed = int(failed.Load())
	batch.TotalJobsFound = int(totalFound.Load())
	batch.TotalJobsCreated = int(totalCreated.Load())
	batch.TotalJobsUpdated = int(totalUpdated.Load())
	batch.TotalJobsClosed = int(totalClosed.Load())

	s.logger.Info("sync-all finished", map[string]interface{}{
		"total":     len(companies),
		"succeeded": batch.Succeeded,
		"failed":    batch.Failed,
	})

	return batch, nil
}

// TriggerSyncAll starts a sync of every active company in a background goroutine
// and returns immediately with the current status. Subsequent calls while a run
// is in flight return the in-flight status with an error rather than starting a
// second concurrent run. Use SyncAllStatus to poll for progress, CancelSyncAll
// to abort.
//
// Built because synchronous sync-all on a single HTTP request times out for any
// non-trivial company count (we have 2473), and the upstream nginx + browser
// kill the request long before the work finishes.
func (s *JobIngestionService) TriggerSyncAll() (SyncAllStatus, error) {
	s.syncAllMu.Lock()
	defer s.syncAllMu.Unlock()

	if s.syncAllStatus.Running {
		return s.syncAllStatus, errors.New("sync-all already in progress")
	}

	started := time.Now()
	s.syncAllStatus = SyncAllStatus{
		Running:    true,
		StartedAt:  &started,
		FinishedAt: nil,
		LastError:  "",
		LastResult: nil,
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.syncAllCancel = cancel

	go func() {
		result, err := s.SyncAllCompaniesWithSummary(runCtx)
		finished := time.Now()

		s.syncAllMu.Lock()
		defer s.syncAllMu.Unlock()

		s.syncAllStatus.Running = false
		s.syncAllStatus.FinishedAt = &finished
		s.syncAllCancel = nil
		if err != nil {
			s.syncAllStatus.LastError = err.Error()
			s.syncAllStatus.LastResult = result // partial result on error is still useful
			return
		}
		s.syncAllStatus.LastError = ""
		s.syncAllStatus.LastResult = result
	}()

	return s.syncAllStatus, nil
}

// SyncAllStatus returns a snapshot of the current/last sync-all run.
func (s *JobIngestionService) SyncAllStatus() SyncAllStatus {
	s.syncAllMu.Lock()
	defer s.syncAllMu.Unlock()
	return s.syncAllStatus
}

// CancelSyncAll aborts an in-flight sync-all. Returns true if a run was
// active and is now cancelled, false if no run was in progress.
func (s *JobIngestionService) CancelSyncAll() bool {
	s.syncAllMu.Lock()
	defer s.syncAllMu.Unlock()
	if s.syncAllCancel == nil {
		return false
	}
	s.syncAllCancel()
	s.syncAllCancel = nil
	return true
}

func (s *JobIngestionService) StartScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = JobSyncSchedulerIntervalFromEnv()
	}
	s.logger.Info("job ingestion scheduler starting", map[string]interface{}{
		"tick_interval":                 interval.String(),
		"minimum_company_sync_interval": MinimumJobSyncIntervalFromEnv().String(),
		"worker_concurrency":            syncWorkerConcurrencyFromEnv(),
		"scheduler_interval_env":        JobSyncSchedulerIntervalMinutesEnv,
		"minimum_sync_interval_env":     MinJobSyncIntervalMinutesEnv,
		"disable_scheduler_env":         JobSyncSchedulerDisabledEnv,
	})
	go s.schedulerLoop(ctx, interval)
}

func (s *JobIngestionService) schedulerLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.kickClassifyBacklog(ctx, autoClassifyBacklogSinceDays)

	for {
		if err := s.syncDueCompanies(ctx); err != nil {
			s.logger.Error("scheduled sync error", err)
		}
		s.kickClassifyBacklog(ctx, autoClassifyBacklogSinceDays)
		if err := s.purgeOldJobPostings(ctx); err != nil {
			s.logger.Error("scheduled job purge error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *JobIngestionService) purgeOldJobPostings(ctx context.Context) error {
	now := s.clock()
	cutoff := now.AddDate(0, -2, 0)

	var totalDeleted int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		deleted, err := s.postingModel.DeleteInactiveBefore(ctx, cutoff, 5000)
		if err != nil {
			return err
		}
		totalDeleted += deleted

		if deleted == 0 {
			break
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("purged old job postings", map[string]interface{}{
			"deleted": totalDeleted,
			"cutoff":  cutoff.Format(time.RFC3339),
		})
	}
	return nil
}

// syncDueCompanies finds active companies whose sync interval has elapsed and
// syncs them through a worker pool. Prior to parallelization this loop ran
// sequentially over 2400+ companies — a single pass took 3-10h, so any
// container restart interrupted it before completion and stale companies
// at the back of the iteration order never got synced.
//
// Companies are sorted by oldest-LastSyncedAt-first so that if the loop is
// interrupted, the most stale companies have already been refreshed and the
// freshest ones are the ones skipped — graceful degradation under interrupt.
//
// Concurrency is bounded by SYNC_WORKER_CONCURRENCY env var (default 8).
// Each worker calls SyncCompany in its own goroutine. A typical full pass at
// concurrency 8 takes 5-15 min instead of 3-10h.
func (s *JobIngestionService) syncDueCompanies(ctx context.Context) error {
	companies, err := s.companyModel.ListActive()
	if err != nil {
		return err
	}

	// Build the due-list, sorted by oldest LastSyncedAt first (nil = never synced = highest priority).
	type dueEntry struct {
		id           int
		lastSyncedAt time.Time
		neverSynced  bool
	}
	now := s.clock()
	minimumInterval := MinimumJobSyncIntervalFromEnv()
	due := make([]dueEntry, 0, len(companies))
	var nextDueAt *time.Time
	for _, company := range companies {
		interval := effectiveCompanySyncInterval(company.SyncIntervalMinutes, minimumInterval)

		entry := dueEntry{id: company.ID, neverSynced: company.LastSyncedAt == nil}
		if company.LastSyncedAt != nil {
			next := company.LastSyncedAt.Add(interval)
			if now.Before(next) {
				if nextDueAt == nil || next.Before(*nextDueAt) {
					nextCopy := next
					nextDueAt = &nextCopy
				}
				continue
			}
			entry.lastSyncedAt = *company.LastSyncedAt
		}
		due = append(due, entry)
	}

	sort.Slice(due, func(i, j int) bool {
		if due[i].neverSynced != due[j].neverSynced {
			return due[i].neverSynced // never-synced first
		}
		return due[i].lastSyncedAt.Before(due[j].lastSyncedAt) // oldest first
	})

	if len(due) == 0 {
		fields := map[string]interface{}{
			"active_companies":              len(companies),
			"due_companies":                 0,
			"minimum_company_sync_interval": minimumInterval.String(),
		}
		if nextDueAt != nil {
			fields["next_due_at"] = nextDueAt.UTC().Format(time.RFC3339)
		}
		s.logger.Info("scheduled sync skipped", fields)
		return nil
	}

	concurrency := syncWorkerConcurrencyFromEnv()
	s.logger.Info("scheduled sync starting", map[string]interface{}{
		"active_companies":              len(companies),
		"due_companies":                 len(due),
		"concurrency":                   concurrency,
		"minimum_company_sync_interval": minimumInterval.String(),
	})

	jobs := make(chan int, len(due))
	for _, e := range due {
		jobs <- e.id
	}
	close(jobs)

	var wg sync.WaitGroup
	var processed atomic.Int64
	var failed atomic.Int64
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for companyID := range jobs {
				if ctx.Err() != nil {
					return
				}
				if _, err := s.SyncCompany(ctx, companyID); err != nil {
					var unsupported *UnsupportedATSProviderError
					if errors.As(err, &unsupported) {
						continue
					}
					failed.Add(1)
					if shouldLogSyncFailureAsError(err) {
						s.logger.Error("scheduled company sync failed", err, map[string]interface{}{"company_id": companyID})
					}
				}
				if n := processed.Add(1); n%100 == 0 {
					s.logger.Info("scheduled sync progress", map[string]interface{}{
						"processed": n,
						"total":     len(due),
						"failed":    failed.Load(),
					})
				}
			}
		}()
	}
	wg.Wait()

	s.logger.Info("scheduled sync finished", map[string]interface{}{
		"processed": processed.Load(),
		"total":     len(due),
		"failed":    failed.Load(),
	})

	return nil
}

func syncWorkerConcurrencyFromEnv() int {
	const def = 8
	if v := strings.TrimSpace(os.Getenv("SYNC_WORKER_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 64 {
			return n
		}
	}
	return def
}

func truncateErrorMessage(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit])
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}
