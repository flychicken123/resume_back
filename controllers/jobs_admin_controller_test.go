package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Test router setup for admin job endpoints
// ---------------------------------------------------------------------------

func setupAdminJobsTestRouter(ctrl *JobsController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("is_admin", true)
		c.Next()
	})
	r.GET("/api/admin/jobs/postings", ctrl.AdminListPostings)
	r.GET("/api/admin/jobs/postings/:id", ctrl.AdminGetPosting)
	r.PUT("/api/admin/jobs/postings/:id", ctrl.AdminUpdatePosting)
	r.DELETE("/api/admin/jobs/postings/:id", ctrl.AdminDeletePosting)
	r.POST("/api/admin/jobs/postings/bulk-update", ctrl.AdminBulkUpdatePostings)
	r.GET("/api/admin/jobs/stats", ctrl.AdminGetJobStats)
	r.GET("/api/admin/jobs/sync-runs", ctrl.AdminListSyncRuns)
	return r
}

func adminGet(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest("GET", path, nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func adminPutJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return putJSON(t, router, path, body)
}

func adminDeleteReq(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	return deleteRequest(t, router, path)
}

func adminPostJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, router, path, body)
}

// ---------------------------------------------------------------------------
// AdminGetPosting — input validation tests (no DB)
// ---------------------------------------------------------------------------

func TestAdminGetPosting_InvalidID_String(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminGet(t, router, "/api/admin/jobs/postings/abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "invalid job id", resp["error"])
}

func TestAdminGetPosting_InvalidID_Zero(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminGet(t, router, "/api/admin/jobs/postings/0")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminGetPosting_InvalidID_Negative(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminGet(t, router, "/api/admin/jobs/postings/-1")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// AdminUpdatePosting — input validation tests (no DB)
// ---------------------------------------------------------------------------

func TestAdminUpdatePosting_InvalidID(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminPutJSON(t, router, "/api/admin/jobs/postings/abc", map[string]any{"title": "X"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = adminPutJSON(t, router, "/api/admin/jobs/postings/0", map[string]any{"title": "X"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminUpdatePosting_InvalidPayload(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	req, _ := http.NewRequest("PUT", "/api/admin/jobs/postings/1", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// AdminDeletePosting — input validation tests (no DB)
// ---------------------------------------------------------------------------

func TestAdminDeletePosting_InvalidID(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminDeleteReq(t, router, "/api/admin/jobs/postings/abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = adminDeleteReq(t, router, "/api/admin/jobs/postings/0")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = adminDeleteReq(t, router, "/api/admin/jobs/postings/-5")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// AdminBulkUpdatePostings — input validation tests (no DB)
// ---------------------------------------------------------------------------

func TestAdminBulkUpdate_EmptyIDs(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	active := true
	w := adminPostJSON(t, router, "/api/admin/jobs/postings/bulk-update", map[string]any{
		"ids":       []int64{},
		"is_active": active,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ids required", resp["error"])
}

func TestAdminBulkUpdate_MissingIsActive(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	w := adminPostJSON(t, router, "/api/admin/jobs/postings/bulk-update", map[string]any{
		"ids": []int64{1, 2, 3},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "is_active required", resp["error"])
}

func TestAdminBulkUpdate_InvalidPayload(t *testing.T) {
	ctrl := &JobsController{}
	router := setupAdminJobsTestRouter(ctrl)

	req, _ := http.NewRequest("POST", "/api/admin/jobs/postings/bulk-update", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

