package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		" returned status 403",
		" returned status 404",
		" returned status 429",
		" could not determine ",
		" could not derive ",
		" board token is required",
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
}

const dailySyncInterval = 24 * time.Hour

func NewJobIngestionService(db *sql.DB, logger *utils.Logger) *JobIngestionService {
	if logger == nil {
		logger = utils.NewLogger()
	}

	service := &JobIngestionService{
		companyModel: models.NewJobCompanyModel(db),
		postingModel: models.NewJobPostingModel(db),
		syncRunModel: models.NewJobSyncRunModel(db),
		providers:    map[string]ATSProvider{},
		logger:       logger,
		clock:        time.Now,
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	service.RegisterProvider("greenhouse", NewGreenhouseProvider(httpClient, logger))
	service.RegisterProvider("workday", NewWorkdayProvider(httpClient, logger))
	service.RegisterProvider("smartrecruiters", NewSmartRecruitersProvider(httpClient, logger))
	service.RegisterProvider("lever", NewLeverProvider(httpClient, logger))
	service.RegisterProvider("workable", NewWorkableProvider(httpClient, logger))
	service.RegisterProvider("ashby", NewAshbyProvider(httpClient, logger))
	service.RegisterProvider("bamboohr", NewBambooHRProvider(httpClient, logger))

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
	return result, nil
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

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
