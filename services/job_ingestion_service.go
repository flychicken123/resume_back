package services

import (
	"context"
	"database/sql"
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
		return nil, fmt.Errorf("no ATS provider registered for %s", company.ATSProvider)
	}

	run, err := s.syncRunModel.Start(company.ID)
	if err != nil {
		return nil, fmt.Errorf("starting sync run: %w", err)
	}

	result := &JobSyncResult{}
	finish := func(status string, finishErr error) {
		msg := ""
		if finishErr != nil {
			msg = finishErr.Error()
			s.logger.Error("job sync run failed", finishErr, map[string]interface{}{"company_id": company.ID, "provider": company.ATSProvider})
		}
		metrics := map[string]int{
			"found":   result.JobsFound,
			"created": result.JobsCreated,
			"updated": result.JobsUpdated,
			"closed":  result.JobsClosed,
		}
		_ = s.syncRunModel.Finish(run.ID, status, optionalString(msg), metrics)
		_ = s.companyModel.UpdateSyncMetadata(company.ID, s.clock(), status)
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
			result.Status = "failed"
			result.Error = syncErr.Error()
			batch.Failed++
			batch.CompanyResults = append(batch.CompanyResults, result)
			s.logger.Error("company sync failed", syncErr, map[string]interface{}{"company_id": company.ID})
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
		interval = 24 * time.Hour
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
		if company.SyncIntervalMinutes <= 0 {
			company.SyncIntervalMinutes = 180
		}

		if company.LastSyncedAt != nil {
			next := company.LastSyncedAt.Add(time.Duration(company.SyncIntervalMinutes) * time.Minute)
			if now.Before(next) {
				continue
			}
		}

		if _, err := s.SyncCompany(ctx, company.ID); err != nil {
			s.logger.Error("scheduled company sync failed", err, map[string]interface{}{"company_id": company.ID})
		}
	}

	return nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}
