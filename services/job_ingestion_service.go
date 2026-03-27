package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
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

func shouldLogSyncFailureAsError(err error) bool {
	if err == nil {
		return false
	}
	var unsupported *UnsupportedATSProviderError
	if errors.As(err, &unsupported) {
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

type JobIngestionService struct {
	companyModel *models.JobCompanyModel
	postingModel *models.JobPostingModel
	syncRunModel *models.JobSyncRunModel
	providers    map[string]ATSProvider
	logger       *utils.Logger
	clock        func() time.Time
	embeddingSvc *EmbeddingService
	ctx          context.Context
	embedMu      sync.Mutex
	embedRunning bool
}

const dailySyncInterval = 24 * time.Hour

func NewJobIngestionService(db *sql.DB, logger *utils.Logger, embeddingSvc *EmbeddingService, ctx context.Context) *JobIngestionService {
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
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	service.RegisterProvider("greenhouse", NewGreenhouseProvider(httpClient, logger))
	service.RegisterProvider("workday", NewWorkdayProvider(httpClient, logger))
	service.RegisterProvider("smartrecruiters", NewSmartRecruitersProvider(httpClient, logger))
	service.RegisterProvider("lever", NewLeverProvider(httpClient, logger))
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
			if err := s.companyModel.RecordSyncFailure(company.ID, syncedAt, status, truncated); err != nil {
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
	now := s.clock()

	for _, job := range jobs {
		if job == nil {
			continue
		}
		job.CompanyID = company.ID
		inserted, err := s.postingModel.Upsert(job)
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
	if len(newIDs) > 0 {
		go s.classifyJobPostingsBatch(s.ctx, newIDs)
	}
	return result, nil
}

// classifyJobPostingsBatch classifies career field, extracts skills, and determines seniority for newly inserted jobs.
func (s *JobIngestionService) classifyJobPostingsBatch(ctx context.Context, ids []int64) {
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := s.postingModel.GetByIDForEmbedding(ctx, id)
		if err != nil || job == nil {
			continue
		}
		if job.CareerField != "" && len(job.ExtractedSkills) > 0 && job.Seniority != "" {
			continue
		}

		prompt := BuildJobClassificationPrompt(job.Title, job.Description)

		// Retry with backoff on rate limit
		var raw string
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			raw, callErr = CallGeminiFlashWithTemperature(prompt, 0.0)
			if callErr == nil {
				break
			}
			errMsg := strings.ToLower(callErr.Error())
			if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "rate") || strings.Contains(errMsg, "resource_exhausted") {
				backoff := time.Duration(1<<uint(attempt)) * 2 * time.Second
				s.logger.Warn("job classification rate limited, backing off", map[string]interface{}{
					"jobID":   id,
					"backoff": backoff.String(),
				})
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return
				}
				continue
			}
			break
		}
		if callErr != nil {
			s.logger.Warn("job classification failed", map[string]interface{}{
				"jobID": id,
				"error": callErr.Error(),
			})
			continue
		}

		field, skills, seniority := ParseJobClassificationResponse(raw)
		if err := s.postingModel.UpdateJobClassification(ctx, id, string(field), skills, seniority); err != nil {
			s.logger.Warn("failed to store job classification", map[string]interface{}{
				"jobID": id,
				"error": err.Error(),
			})
		}

		// Rate limit: 200ms delay between jobs to avoid hammering Gemini
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// embedJobPostingsBatch generates and stores embeddings for a set of newly inserted job IDs.
// Only one batch runs at a time; concurrent calls are silently skipped (jobs will be
// picked up by the next backfill run).
func (s *JobIngestionService) embedJobPostingsBatch(ctx context.Context, ids []int64) {
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

// SyncAllCompaniesWithSummary runs a sync for every active company and returns aggregate metrics
func (s *JobIngestionService) SyncAllCompaniesWithSummary(ctx context.Context) (*JobSyncBatchResult, error) {
	companies, err := s.companyModel.ListActive()
	if err != nil {
		return nil, err
	}

	batch := &JobSyncBatchResult{
		TotalCompanies: len(companies),
		CompanyResults: make([]JobSyncCompanyResult, 0, len(companies)),
	}

	for _, company := range companies {
		result := JobSyncCompanyResult{
			CompanyID:   company.ID,
			CompanyName: company.Name,
		}

		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			if ctx.Err() != nil {
				result.Error = ctx.Err().Error()
			}
			batch.Failed++
			batch.CompanyResults = append(batch.CompanyResults, result)
			return batch, ctx.Err()
		default:
		}

		batch.Ran++
		syncResult, syncErr := s.SyncCompany(ctx, company.ID)
		if syncErr != nil {
			var unsupported *UnsupportedATSProviderError
			if errors.As(syncErr, &unsupported) {
				result.Status = "unsupported_provider"
				result.Error = syncErr.Error()
				batch.Failed++
				batch.CompanyResults = append(batch.CompanyResults, result)
				continue
			}
			result.Status = "failed"
			result.Error = syncErr.Error()
			batch.Failed++
			batch.CompanyResults = append(batch.CompanyResults, result)
			continue
		}

		result.Status = "success"
		result.JobsFound = syncResult.JobsFound
		result.JobsCreated = syncResult.JobsCreated
		result.JobsUpdated = syncResult.JobsUpdated
		result.JobsClosed = syncResult.JobsClosed

		batch.Succeeded++
		batch.TotalJobsFound += syncResult.JobsFound
		batch.TotalJobsCreated += syncResult.JobsCreated
		batch.TotalJobsUpdated += syncResult.JobsUpdated
		batch.TotalJobsClosed += syncResult.JobsClosed
		batch.CompanyResults = append(batch.CompanyResults, result)
	}

	return batch, nil
}

func (s *JobIngestionService) StartScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = dailySyncInterval
	} else if interval < dailySyncInterval {
		interval = dailySyncInterval
	}
	go s.schedulerLoop(ctx, interval)
}

func (s *JobIngestionService) schedulerLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := s.syncDueCompanies(ctx); err != nil {
			s.logger.Error("scheduled sync error", err)
		}
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

func (s *JobIngestionService) syncDueCompanies(ctx context.Context) error {
	companies, err := s.companyModel.ListActive()
	if err != nil {
		return err
	}

	now := s.clock()
	for _, company := range companies {
		interval := time.Duration(company.SyncIntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = dailySyncInterval
		} else if interval < dailySyncInterval {
			interval = dailySyncInterval
		}

		if company.LastSyncedAt != nil {
			next := company.LastSyncedAt.Add(interval)
			if now.Before(next) {
				continue
			}
		}

		if _, err := s.SyncCompany(ctx, company.ID); err != nil {
			var unsupported *UnsupportedATSProviderError
			if errors.As(err, &unsupported) {
				continue
			}
			if shouldLogSyncFailureAsError(err) {
				s.logger.Error("scheduled company sync failed", err, map[string]interface{}{"company_id": company.ID})
			}
		}
	}

	return nil
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
