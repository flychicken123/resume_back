package controllers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"resumeai/models"
	"resumeai/services"
	"resumeai/utils"
)

type JobsController struct {
	companies  *models.JobCompanyModel
	postings   *models.JobPostingModel
	syncRuns   *models.JobSyncRunModel
	matches    *models.ResumeJobMatchModel
	dismissals *models.DismissedJobMatchModel
	matcher    services.ResumeJobMatcher
	ingest     *services.JobIngestionService
	importer   *services.CompanyImportService
}

func NewJobsController(companies *models.JobCompanyModel, postings *models.JobPostingModel, syncRuns *models.JobSyncRunModel, matches *models.ResumeJobMatchModel, dismissals *models.DismissedJobMatchModel, matcher services.ResumeJobMatcher, ingest *services.JobIngestionService) *JobsController {
	return &JobsController{
		companies:  companies,
		postings:   postings,
		syncRuns:   syncRuns,
		matches:    matches,
		dismissals: dismissals,
		matcher:    matcher,
		ingest:     ingest,
		importer:   services.NewCompanyImportService(companies),
	}
}

// GetJobByID returns a single job posting with company name.
func (jc *JobsController) GetJobByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	posting, companyName, err := jc.postings.GetByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load job"})
		return
	}

	response := gin.H{
		"id":              posting.ID,
		"company_id":      posting.CompanyID,
		"external_job_id": posting.ExternalJobID,
		"title":           posting.Title,
		"location":        posting.Location,
		"remote_type":     posting.RemoteType,
		"department":      posting.Department,
		"employment_type": posting.EmploymentType,
		"job_url":         posting.JobURL,
		"application_url": posting.ApplicationURL,
		"description":     posting.Description,
		"salary_min":      posting.SalaryMin,
		"salary_max":      posting.SalaryMax,
		"salary_currency": posting.SalaryCurrency,
		"posted_at":       posting.PostedAt,
		"first_seen_at":   posting.FirstSeenAt,
		"last_seen_at":    posting.LastSeenAt,
		"closed_at":       posting.ClosedAt,
		"is_active":       posting.IsActive,
	}
	if companyName != nil {
		response["company_name"] = *companyName
	}

	c.JSON(http.StatusOK, response)
}

func (jc *JobsController) ListCompanies(c *gin.Context) {
	page := 1
	pageSize := 25
	status := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "all")))

	if raw := c.Query("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}

	if pageSize > 100 {
		pageSize = 100
	}
	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * pageSize
	companies, total, err := jc.companies.ListWithPagination(status, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load companies"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if total == 0 {
		page = 1
	}
	if total > 0 && page > totalPages {
		page = totalPages
		offset = (page - 1) * pageSize
		companies, _, err = jc.companies.ListWithPagination(status, pageSize, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load companies"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"companies":   companies,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
	})
}

func (jc *JobsController) ExportCompanyATS(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "json")))
	status := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "all")))

	companies, err := jc.companies.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load companies"})
		return
	}

	filtered := make([]*models.JobCompany, 0, len(companies))
	for _, company := range companies {
		if company == nil {
			continue
		}
		switch status {
		case "active":
			if !company.IsActive {
				continue
			}
		case "inactive":
			if company.IsActive {
				continue
			}
		}
		filtered = append(filtered, company)
	}

	if format == "csv" {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"id", "name", "ats_provider", "careers_url", "is_active"})
		for _, company := range filtered {
			_ = writer.Write([]string{
				strconv.Itoa(company.ID),
				company.Name,
				company.ATSProvider,
				company.CareersURL,
				strconv.FormatBool(company.IsActive),
			})
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build csv"})
			return
		}

		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename=job_companies_ats.csv")
		c.String(http.StatusOK, buf.String())
		return
	}

	out := make([]gin.H, 0, len(filtered))
	for _, company := range filtered {
		out = append(out, gin.H{
			"id":           company.ID,
			"name":         company.Name,
			"ats_provider": company.ATSProvider,
			"careers_url":  company.CareersURL,
			"is_active":    company.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{"companies": out, "count": len(out)})
}

type createCompanyRequest struct {
	Name                string  `json:"name" binding:"required"`
	WebsiteURL          string  `json:"website_url"`
	CareersURL          string  `json:"careers_url" binding:"required"`
	ATSProvider         string  `json:"ats_provider"`
	ExternalIdentifier  *string `json:"external_identifier"`
	SyncIntervalMinutes *int    `json:"sync_interval_minutes"`
	IsActive            *bool   `json:"is_active"`
}

func (jc *JobsController) CreateCompany(c *gin.Context) {
	var req createCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	name := strings.TrimSpace(req.Name)
	careers := strings.TrimSpace(req.CareersURL)
	provider := strings.TrimSpace(req.ATSProvider)
	if name == "" || careers == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and careers_url are required"})
		return
	}

	if provider == "" || strings.EqualFold(provider, "auto") {
		provider = services.DetectATSProvider(careers)
	}
	if provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unable to infer ats_provider; please provide it explicitly"})
		return
	}
	provider = strings.ToLower(provider)

	interval := 1440
	if req.SyncIntervalMinutes != nil && *req.SyncIntervalMinutes > 0 {
		interval = *req.SyncIntervalMinutes
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	company := &models.JobCompany{
		Name:                name,
		WebsiteURL:          strings.TrimSpace(req.WebsiteURL),
		CareersURL:          careers,
		ATSProvider:         provider,
		ExternalIdentifier:  trimPointer(req.ExternalIdentifier),
		IsActive:            isActive,
		SyncIntervalMinutes: interval,
	}

	if err := jc.companies.Create(company); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create company"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"company": company})
}

type updateCompanyStatusRequest struct {
	IsActive *bool `json:"is_active"`
}

func (jc *JobsController) UpdateCompanyStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company id"})
		return
	}

	var req updateCompanyStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.IsActive == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "is_active field is required"})
		return
	}

	if err := jc.companies.SetCompanyActive(id, *req.IsActive); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update company status"})
		return
	}

	company, err := jc.companies.GetByID(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"company": company})
}

func (jc *JobsController) ImportCompanies(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field 'file' is required"})
		return
	}

	if file.Size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file is empty"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer src.Close()

	summary, err := jc.importer.ImportCSV(src)
	if err != nil {
		if errors.Is(err, services.ErrCompanyImportInvalidFormat) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import companies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "company import completed",
		"summary": summary,
	})
}

func (jc *JobsController) TriggerSync(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company id"})
		return
	}

	result, err := jc.ingest.SyncCompany(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sync started", "result": result})
}

func (jc *JobsController) TriggerSyncAll(c *gin.Context) {
	result, err := jc.ingest.SyncAllCompaniesWithSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sync completed", "result": result})
}

func (jc *JobsController) ComputeMatches(c *gin.Context) {
	if jc.matcher == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "job matching not configured"})
		return
	}

	rawUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	userID, ok := normalizeUserID(rawUserID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user context"})
		return
	}

	var req struct {
		Position          string   `json:"position"`
		Name              string   `json:"name"`
		Email             string   `json:"email"`
		Summary           string   `json:"summary"`
		Experience        string   `json:"experience"`
		Education         string   `json:"education"`
		JobDescription    string   `json:"jobDescription"`
		Location          string   `json:"location"`
		Skills            []string `json:"skills"`
		HtmlContent       string   `json:"htmlContent"`
		CandidateJobLimit int      `json:"candidateJobLimit"`
		MaxResults        int      `json:"maxResults"`
		QuickMode         bool     `json:"quickMode"` // If true, skip AI filter for instant results
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	hashInput := services.ResumeHashInput{
		Position:       req.Position,
		Name:           req.Name,
		Email:          req.Email,
		Summary:        req.Summary,
		Experience:     req.Experience,
		Education:      req.Education,
		JobDescription: req.JobDescription,
		Location:       req.Location,
		Skills:         req.Skills,
		HtmlContent:    req.HtmlContent,
	}

	resumeHash := services.DeriveResumeHash(hashInput)

	input := services.ResumeJobMatchInput{
		UserID:            userID,
		ResumeHash:        resumeHash,
		Position:          req.Position,
		Summary:           req.Summary,
		Experience:        req.Experience,
		Education:         req.Education,
		JobDescription:    req.JobDescription,
		PreferredLocation: req.Location,
		Skills:            req.Skills,
		CandidateJobLimit: req.CandidateJobLimit,
		MaxResults:        req.MaxResults,
		QuickMode:         req.QuickMode,
	}

	matches, err := jc.matcher.MatchAndStore(c.Request.Context(), input)

	// If quick mode was used, start background job to run full matching with AI filter
	aiFilterPending := false
	if req.QuickMode {
		aiFilterPending = true
		// Run full matching in background
		go func() {
			bgInput := input
			bgInput.QuickMode = false // Run with AI filter
			_, bgErr := jc.matcher.MatchAndStore(context.Background(), bgInput)
			if bgErr != nil {
				// Log error but don't fail - quick results are already returned
				// The background job will update the DB with refined results
				_ = bgErr // Silently handle - results will be updated on next poll
			}
		}()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute job matches", "details": err.Error()})
		return
	}

	filteredMatches := make([]*models.ResumeJobMatchRecord, 0, len(matches))
	for _, match := range matches {
		if utils.JobLooksLikeInternRole(match.JobTitle, match.JobDescription) {
			continue
		}
		filteredMatches = append(filteredMatches, match)
	}
	matches = filteredMatches

	topMatches := matches
	if len(topMatches) > 3 {
		topMatches = topMatches[:3]
	}

	c.JSON(http.StatusOK, gin.H{
		"resumeHash":      resumeHash,
		"count":           len(matches),
		"matches":         matches,
		"topMatches":      topMatches,
		"aiFilterPending": aiFilterPending,
	})
}

func (jc *JobsController) ListMatchedJobs(c *gin.Context) {
	if jc.matches == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "job matching not configured"})
		return
	}

	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	userID, ok := normalizeUserID(userIDVal)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user context"})
		return
	}

	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	resumeHash := strings.TrimSpace(c.Query("resume_hash"))
	var (
		matches       []*models.ResumeJobMatchRecord
		effectiveHash string
		err           error
	)

	if resumeHash != "" {
		matches, err = jc.matches.ListByUserAndResume(userID, resumeHash, limit)
		effectiveHash = resumeHash
	} else {
		matches, effectiveHash, err = jc.matches.ListMostRecentByUser(userID, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load matched jobs"})
		return
	}

	filteredMatches := make([]*models.ResumeJobMatchRecord, 0, len(matches))
	for _, match := range matches {
		if utils.JobLooksLikeInternRole(match.JobTitle, match.JobDescription) {
			continue
		}
		if utils.IsSupportedJobLocation(match.JobLocation, match.JobRemoteType) {
			filteredMatches = append(filteredMatches, match)
		}
	}

	matches = filteredMatches

	var topMatches []*models.ResumeJobMatchRecord
	if len(matches) > 0 {
		topLimit := 3
		if len(matches) < topLimit {
			topLimit = len(matches)
		}
		topMatches = matches[:topLimit]
	}

	c.JSON(http.StatusOK, gin.H{
		"resumeHash": effectiveHash,
		"count":      len(matches),
		"matches":    matches,
		"topMatches": topMatches,
	})
}

func (jc *JobsController) ListJobs(c *gin.Context) {
	var companyID *int
	if raw := c.Query("company_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid company_id"})
			return
		}
		companyID = &id
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if l, err := strconv.Atoi(raw); err == nil && l > 0 {
			limit = l
		}
	}

	jobs, err := jc.postings.ListActive(companyID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load job postings"})
		return
	}

	filtered := make([]*models.JobPosting, 0, len(jobs))
	for _, job := range jobs {
		if utils.IsSupportedJobLocation(job.Location, job.RemoteType) {
			filtered = append(filtered, job)
		}
	}

	c.JSON(http.StatusOK, gin.H{"jobs": filtered, "count": len(filtered)})
}

// ---------------------------------------------------------------------------
// Admin job posting management endpoints
// ---------------------------------------------------------------------------

func (jc *JobsController) AdminListPostings(c *gin.Context) {
	params := models.JobPostingFilterParams{
		Page:     1,
		PageSize: 25,
	}

	if raw := c.Query("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.Page = v
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.PageSize = v
		}
	}
	params.Search = strings.TrimSpace(c.Query("search"))
	if raw := c.Query("company_id"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.CompanyID = &v
		}
	}
	if raw := c.Query("is_active"); raw != "" {
		val := strings.ToLower(raw) == "true"
		params.IsActive = &val
	}
	params.RemoteType = strings.TrimSpace(c.Query("remote_type"))
	params.EmploymentType = strings.TrimSpace(c.Query("employment_type"))
	if raw := c.Query("seniority_level"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			params.SeniorityLevel = &v
		}
	}
	if raw := c.Query("date_from"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			params.DateFrom = &t
		}
	}
	if raw := c.Query("date_to"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			params.DateTo = &end
		}
	}
	params.SortBy = strings.TrimSpace(c.Query("sort_by"))
	params.SortDir = strings.TrimSpace(c.Query("sort_dir"))

	postings, total, err := jc.postings.ListWithFilters(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load postings"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(params.PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"postings":    postings,
		"page":        params.Page,
		"page_size":   params.PageSize,
		"total":       total,
		"total_pages": totalPages,
	})
}

func (jc *JobsController) AdminGetPosting(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	posting, err := jc.postings.GetByIDFull(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load job"})
		return
	}

	c.JSON(http.StatusOK, posting)
}

func (jc *JobsController) AdminUpdatePosting(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	if err := jc.postings.UpdatePosting(id, body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		if err.Error() == "no valid fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update posting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "posting updated"})
}

func (jc *JobsController) AdminDeletePosting(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	if err := jc.postings.DeletePosting(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete posting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "posting deleted"})
}

func (jc *JobsController) AdminBulkUpdatePostings(c *gin.Context) {
	var req struct {
		IDs      []int64 `json:"ids"`
		IsActive *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids required"})
		return
	}
	if req.IsActive == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "is_active required"})
		return
	}

	updated, err := jc.postings.BulkUpdateActive(req.IDs, *req.IsActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update postings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("%d postings updated", updated),
		"updated": updated,
	})
}

func (jc *JobsController) AdminGetJobStats(c *gin.Context) {
	stats, err := jc.postings.GetStatistics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load statistics"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (jc *JobsController) AdminListSyncRuns(c *gin.Context) {
	params := models.SyncRunFilterParams{
		Page:     1,
		PageSize: 25,
	}

	if raw := c.Query("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.Page = v
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.PageSize = v
		}
	}
	if raw := c.Query("company_id"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			params.CompanyID = &v
		}
	}
	params.Status = strings.TrimSpace(c.Query("status"))
	if raw := c.Query("date_from"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			params.DateFrom = &t
		}
	}
	if raw := c.Query("date_to"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			params.DateTo = &end
		}
	}

	runs, total, err := jc.syncRuns.ListWithFilters(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sync runs"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(params.PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"sync_runs":   runs,
		"page":        params.Page,
		"page_size":   params.PageSize,
		"total":       total,
		"total_pages": totalPages,
	})
}

func normalizeUserID(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func trimPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// DismissMatch records a user's dismissal of a job match.
func (jc *JobsController) DismissMatch(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userID, ok := normalizeUserID(userIDVal)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user context"})
		return
	}

	var req struct {
		JobPostingID int64  `json:"job_posting_id"`
		Reason       string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.JobPostingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job_posting_id"})
		return
	}
	if !models.IsValidDismissReason(req.Reason) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dismiss reason"})
		return
	}

	if err := jc.dismissals.Dismiss(userID, req.JobPostingID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to dismiss job match"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "job_posting_id": req.JobPostingID})
}

// UndismissMatch removes a user's dismissal of a job match.
func (jc *JobsController) UndismissMatch(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userID, ok := normalizeUserID(userIDVal)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user context"})
		return
	}

	var req struct {
		JobPostingID int64 `json:"job_posting_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.JobPostingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job_posting_id"})
		return
	}

	if err := jc.dismissals.Undismiss(userID, req.JobPostingID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to undismiss job match"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
