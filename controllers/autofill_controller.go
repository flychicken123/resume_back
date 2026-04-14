package controllers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resumeai/models"
)

type autofillSession struct {
	Payload   gin.H     `json:"payload"`
	CreatedAt time.Time `json:"-"`
}

type AutofillController struct {
	userModel   *models.UserModel
	resumeModel *models.ResumeModel
	sessions    sync.Map
}

func NewAutofillController(userModel *models.UserModel, resumeModel *models.ResumeModel) *AutofillController {
	ac := &AutofillController{
		userModel:   userModel,
		resumeModel: resumeModel,
	}
	go ac.cleanupLoop()
	return ac
}

// cleanupLoop removes expired sessions every 5 minutes.
func (ac *AutofillController) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		ac.sessions.Range(func(key, value any) bool {
			if s, ok := value.(*autofillSession); ok && now.Sub(s.CreatedAt) > 30*time.Minute {
				ac.sessions.Delete(key)
			}
			return true
		})
	}
}

// CreateSession creates an autofill session with the user's resume payload.
// POST /api/autofill/session  (requires JWT)
func (ac *AutofillController) CreateSession(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		JobURL string `json:"job_url" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "job_url is required"})
		return
	}

	// Load resume data (same approach as user_controller.go LoadUserData)
	resume, err := ac.resumeModel.GetByUserID(userID.(int))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "No resume data found. Please build your resume first."})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load resume data"})
		return
	}

	experiences, err := ac.resumeModel.GetExperiencesByResumeID(resume.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load experience data"})
		return
	}

	jobPrefs, _ := ac.userModel.GetJobPreferences(userID.(int))
	if jobPrefs == nil {
		jobPrefs = json.RawMessage(`{}`)
	}

	// Safely convert json.RawMessage fields to plain values to avoid
	// MarshalJSON errors when DB contains non-JSON strings.
	safeRaw := func(r json.RawMessage) interface{} {
		if len(r) == 0 {
			return nil
		}
		// If it's a valid JSON value (object/array/string/number), pass as-is.
		var v interface{}
		if err := json.Unmarshal(r, &v); err == nil {
			return v
		}
		// Otherwise treat it as a plain string.
		return string(r)
	}

	// Fall back to first non-remote experience city when resume.Location is empty
	location := resume.Location
	if location == "" || location == `""` {
		for _, exp := range experiences {
			city := strings.TrimSpace(exp.City)
			state := strings.TrimSpace(exp.State)
			if city != "" && !strings.EqualFold(city, "remote") {
				parts := []string{city}
				if state != "" {
					parts = append(parts, state)
				}
				location = strings.Join(parts, ", ")
				break
			}
		}
	}

	payload := gin.H{
		"name":              resume.Name,
		"email":             resume.Email,
		"phone":             resume.Phone,
		"summary":           safeRaw(resume.Summary),
		"skills":            safeRaw(resume.Skills),
		"skillsCategorized": resume.SkillsCategorized,
		"experiences":       experiences,
		"experience":        resume.Experience,
		"education":         resume.Education,
		"jobDescription":    resume.JobDescription,
		"location":          location,
		"selectedFormat":    resume.SelectedFormat,
		"jobPreferences":    jobPrefs,
	}

	sessionID := uuid.New().String()
	ac.sessions.Store(sessionID, &autofillSession{
		Payload:   payload,
		CreatedAt: time.Now(),
	})

	// Build the target URL with __hh query param
	separator := "?"
	if len(req.JobURL) > 0 {
		for _, ch := range req.JobURL {
			if ch == '?' {
				separator = "&"
				break
			}
		}
	}
	targetURL := req.JobURL + separator + "__hh=" + sessionID

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"session_id": sessionID,
			"url":        targetURL,
		},
	})
}

// GetSession returns the resume payload for a session.
// GET /api/autofill/session/:id  (no auth)
func (ac *AutofillController) GetSession(ctx *gin.Context) {
	// CORS for extension
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	ctx.Header("Access-Control-Allow-Headers", "Content-Type")

	id := ctx.Param("id")
	val, ok := ac.sessions.Load(id)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Session not found or expired"})
		return
	}

	session := val.(*autofillSession)
	if time.Since(session.CreatedAt) > 30*time.Minute {
		ac.sessions.Delete(id)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Session expired"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    session.Payload,
	})
}

// GetSessionOptions handles OPTIONS preflight for GetSession.
func (ac *AutofillController) GetSessionOptions(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", "*")
	ctx.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	ctx.Header("Access-Control-Allow-Headers", "Content-Type")
	ctx.Status(http.StatusNoContent)
}
