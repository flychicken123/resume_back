package controllers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/models"
)

// JobApplicationController handles job application tracking endpoints.
type JobApplicationController struct {
	applications *models.JobApplicationModel
	postings     *models.JobPostingModel
	matches      *models.ResumeJobMatchModel
}

// NewJobApplicationController constructs a JobApplicationController.
func NewJobApplicationController(
	applications *models.JobApplicationModel,
	postings *models.JobPostingModel,
	matches *models.ResumeJobMatchModel,
) *JobApplicationController {
	return &JobApplicationController{
		applications: applications,
		postings:     postings,
		matches:      matches,
	}
}

// ---------------------------------------------------------------------------
// POST /api/job/apply
// ---------------------------------------------------------------------------

type submitApplicationRequest struct {
	JobPostingID     *int64 `json:"job_posting_id"`
	ResumeJobMatchID *int64 `json:"resume_job_match_id"`
	JobTitle         string `json:"job_title"`
	CompanyName      string `json:"company_name"`
	JobURL           string `json:"job_url"`
	JobLocation      string `json:"job_location"`
	ResumeHash       string `json:"resume_hash"`
	CoverLetterUsed  bool   `json:"cover_letter_used"`
	Notes            string `json:"notes"`
}

func (jac *JobApplicationController) SubmitApplication(c *gin.Context) {
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

	var req submitApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	jobTitle := strings.TrimSpace(req.JobTitle)
	if jobTitle == "" {
		c.JSON(http.StatusPreconditionFailed, gin.H{
			"error":          "missing_job_profile_info",
			"missing_fields": []string{"job_title"},
		})
		return
	}

	companyName := strings.TrimSpace(req.CompanyName)
	jobURL := strings.TrimSpace(req.JobURL)
	jobLocation := strings.TrimSpace(req.JobLocation)

	// If job_posting_id provided, try to fill denormalized fields from the posting
	if req.JobPostingID != nil && jac.postings != nil {
		posting, cn, err := jac.postings.GetByID(*req.JobPostingID)
		if err == nil && posting != nil {
			if jobTitle == "" {
				jobTitle = posting.Title
			}
			if companyName == "" && cn != nil {
				companyName = *cn
			}
			if jobURL == "" {
				jobURL = posting.JobURL
			}
			if jobLocation == "" {
				jobLocation = posting.Location
			}
		}
	}

	app := &models.JobApplication{
		UserID:           userID,
		JobPostingID:     req.JobPostingID,
		ResumeJobMatchID: req.ResumeJobMatchID,
		JobTitle:         jobTitle,
		CompanyName:      companyName,
		JobURL:           jobURL,
		JobLocation:      jobLocation,
		Status:           "applied",
		ResumeHash:       strings.TrimSpace(req.ResumeHash),
		CoverLetterUsed:  req.CoverLetterUsed,
		Notes:            strings.TrimSpace(req.Notes),
	}

	if err := jac.applications.Create(app); err != nil {
		if strings.Contains(err.Error(), "ux_job_applications_user_job") ||
			strings.Contains(err.Error(), "duplicate key") {
			c.JSON(http.StatusConflict, gin.H{"error": "application already exists for this job posting"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create application"})
		return
	}

	c.JSON(http.StatusCreated, app)
}

// ---------------------------------------------------------------------------
// GET /api/job/applications
// ---------------------------------------------------------------------------

func (jac *JobApplicationController) ListApplications(c *gin.Context) {
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

	limit := 20
	offset := 0
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	statusFilter := strings.TrimSpace(c.Query("status"))

	apps, total, err := jac.applications.ListByUser(userID, limit, offset, statusFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch applications"})
		return
	}
	if apps == nil {
		apps = []*models.JobApplication{}
	}

	c.JSON(http.StatusOK, gin.H{
		"applications": apps,
		"total":        total,
	})
}

// ---------------------------------------------------------------------------
// GET /api/job/applications/stats
// ---------------------------------------------------------------------------

func (jac *JobApplicationController) GetStats(c *gin.Context) {
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

	byStatus, total, thisWeek, thisMonth, err := jac.applications.GetStatsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch stats"})
		return
	}

	// response_rate = applications that moved past "applied" / total
	responseRate := 0.0
	if total > 0 {
		pastApplied := total - byStatus["applied"]
		responseRate = float64(pastApplied) / float64(total)
	}

	c.JSON(http.StatusOK, gin.H{
		"total":         total,
		"by_status":     byStatus,
		"this_week":     thisWeek,
		"this_month":    thisMonth,
		"response_rate": responseRate,
	})
}

// ---------------------------------------------------------------------------
// GET /api/job/applications/:id
// ---------------------------------------------------------------------------

func (jac *JobApplicationController) GetApplication(c *gin.Context) {
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

	appID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || appID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application id"})
		return
	}

	app, err := jac.applications.GetByID(userID, appID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch application"})
		return
	}

	history, err := jac.applications.GetStatusHistory(appID)
	if err != nil {
		history = []*models.JobApplicationStatusHistory{}
	}
	if history == nil {
		history = []*models.JobApplicationStatusHistory{}
	}

	c.JSON(http.StatusOK, gin.H{
		"application":    app,
		"status_history": history,
	})
}

// ---------------------------------------------------------------------------
// PUT /api/job/applications/:id/status
// ---------------------------------------------------------------------------

type updateStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Note   string `json:"note"`
}

func (jac *JobApplicationController) UpdateStatus(c *gin.Context) {
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

	appID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || appID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application id"})
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}

	newStatus := strings.TrimSpace(strings.ToLower(req.Status))
	if err := jac.applications.UpdateStatus(userID, appID, newStatus, strings.TrimSpace(req.Note)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		var transErr *models.InvalidTransitionError
		if errors.As(err, &transErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":               "invalid status transition",
				"current_status":      transErr.CurrentStatus,
				"requested_status":    transErr.RequestedStatus,
				"allowed_transitions": transErr.Allowed,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	// Fetch updated application to return
	app, err := jac.applications.GetByID(userID, appID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "status updated"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "status updated",
		"application": app,
	})
}

// ---------------------------------------------------------------------------
// DELETE /api/job/applications/:id
// ---------------------------------------------------------------------------

func (jac *JobApplicationController) DeleteApplication(c *gin.Context) {
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

	appID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || appID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application id"})
		return
	}

	if err := jac.applications.Delete(userID, appID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "application deleted"})
}
