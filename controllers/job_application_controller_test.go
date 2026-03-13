package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"resumeai/models"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupJobAppTestRouter(ctrl *JobApplicationController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Inject a fake user_id for all requests
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Next()
	})

	r.POST("/api/job/apply", ctrl.SubmitApplication)
	r.GET("/api/job/applications", ctrl.ListApplications)
	r.GET("/api/job/applications/stats", ctrl.GetStats)
	r.GET("/api/job/applications/:id", ctrl.GetApplication)
	r.PUT("/api/job/applications/:id/status", ctrl.UpdateStatus)
	r.DELETE("/api/job/applications/:id", ctrl.DeleteApplication)
	return r
}

func postJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	assert.NoError(t, err)
	req, err := http.NewRequest("POST", path, bytes.NewBuffer(b))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func putJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	assert.NoError(t, err)
	req, err := http.NewRequest("PUT", path, bytes.NewBuffer(b))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func getRequest(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest("GET", path, nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func deleteRequest(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest("DELETE", path, nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// State machine unit tests (no DB needed)
// ---------------------------------------------------------------------------

func TestSubmitApplication_MissingTitle(t *testing.T) {
	ctrl := NewJobApplicationController(nil, nil, nil)
	router := setupJobAppTestRouter(ctrl)

	w := postJSON(t, router, "/api/job/apply", map[string]any{
		"company_name": "Acme Corp",
	})

	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	var resp map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "missing_job_profile_info", resp["error"])
}

func TestSubmitApplication_EmptyTitle(t *testing.T) {
	ctrl := NewJobApplicationController(nil, nil, nil)
	router := setupJobAppTestRouter(ctrl)

	w := postJSON(t, router, "/api/job/apply", map[string]any{
		"job_title":    "   ",
		"company_name": "Acme Corp",
	})

	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
}

func TestUpdateStatus_InvalidPayload(t *testing.T) {
	ctrl := NewJobApplicationController(nil, nil, nil)
	router := setupJobAppTestRouter(ctrl)

	w := putJSON(t, router, "/api/job/applications/1/status", map[string]any{})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetApplication_InvalidID(t *testing.T) {
	ctrl := NewJobApplicationController(nil, nil, nil)
	router := setupJobAppTestRouter(ctrl)

	w := getRequest(t, router, "/api/job/applications/abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = getRequest(t, router, "/api/job/applications/0")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = getRequest(t, router, "/api/job/applications/-1")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteApplication_InvalidID(t *testing.T) {
	ctrl := NewJobApplicationController(nil, nil, nil)
	router := setupJobAppTestRouter(ctrl)

	w := deleteRequest(t, router, "/api/job/applications/abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// InvalidTransitionError formatting
// ---------------------------------------------------------------------------

func TestInvalidTransitionError_Message(t *testing.T) {
	err := &models.InvalidTransitionError{
		CurrentStatus:   "rejected",
		RequestedStatus: "applied",
		Allowed:         []string{},
	}
	assert.Contains(t, err.Error(), "rejected")
	assert.Contains(t, err.Error(), "applied")
}
