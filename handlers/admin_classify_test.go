package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type stubClassifyStopper struct {
	backfillCancelResult bool
}

func (s *stubClassifyStopper) CancelBackfill() bool {
	return s.backfillCancelResult
}

func setupClassifyStopRouter(stopper ClassifyStopper) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/classify/stop", ClassifyStopHandler(stopper))
	return r
}

func TestAdminClassifyStop_ActiveBackfill_Returns200(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: true}
	r := setupClassifyStopRouter(stopper)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classify/stop", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, true, body["stopped"])
	assert.Equal(t, "backfill", body["scope"])
}

func TestAdminClassifyStop_NoActiveRun_Returns409(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: false}
	r := setupClassifyStopRouter(stopper)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classify/stop", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, false, body["stopped"])
	assert.Contains(t, body["reason"], "no active")
}
