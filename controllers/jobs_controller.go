package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/models"
	"resumeai/services"
	"resumeai/utils"
)

type JobsController struct {
	companies *models.JobCompanyModel
	postings  *models.JobPostingModel
	syncRuns  *models.JobSyncRunModel
	matches   *models.ResumeJobMatchModel
	matcher   services.ResumeJobMatcher
	ingest    *services.JobIngestionService
	importer  *services.CompanyImportService
}

func NewJobsController(companies *models.JobCompanyModel, postings *models.JobPostingModel, syncRuns *models.JobSyncRunModel, matches *models.ResumeJobMatchModel, matcher services.ResumeJobMatcher, ingest *services.JobIngestionService) *JobsController {
	return &JobsController{
		companies: companies,
		postings:  postings,
		syncRuns:  syncRuns,
		matches:   matches,
		matcher:   matcher,
		ingest:    ingest,
		importer:  services.NewCompanyImportService(companies),
	}
}

func (jc *JobsController) ListCompanies(c *gin.Context) {
	companies, err := jc.companies.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load companies"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"companies": companies})
}

type createCompanyRequest struct {
	Name                string  `json:"name" binding:"required"`
	WebsiteURL          string  `json:"website_url"`
	CareersURL          string  `json:"careers_url" binding:"required"`
	ATSProvider         string  `json:"ats_provider" binding:"required"`
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
	if name == "" || careers == "" || provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, careers_url, and ats_provider are required"})
		return
	}

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
	}

	matches, err := jc.matcher.MatchAndStore(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute job matches"})
		return
	}

	topMatches := matches
	if len(topMatches) > 3 {
		topMatches = topMatches[:3]
	}

	c.JSON(http.StatusOK, gin.H{
		"resumeHash": resumeHash,
		"count":      len(matches),
		"matches":    matches,
		"topMatches": topMatches,
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
