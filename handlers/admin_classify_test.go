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
	backfillCancelResult  bool
	ingestionRunning      bool
	ingestionPauseCalled  bool
}

func (s *stubClassifyStopper) CancelBackfill() bool {
	return s.backfillCancelResult
}
func (s *stubClassifyStopper) PauseIngestionClassifier() {
	s.ingestionPauseCalled = true
	s.ingestionRunning = false
}
func (s *stubClassifyStopper) IsIngestionClassifierRunning() bool {
	return s.ingestionRunning
}

func setupClassifyStopRouter(stopper ClassifyStopper) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/classify/stop", ClassifyStopHandler(stopper))
	return r
}

func TestAdminClassifyStop_ActiveBackfill_Returns200(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: true, ingestionRunning: true}
	r := setupClassifyStopRouter(stopper)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classify/stop", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, true, body["stopped"])
	assert.Equal(t, "backfill_and_ingestion", body["scope"])
	assert.True(t, stopper.ingestionPauseCalled)
}

func TestAdminClassifyStop_OnlyBackfillActive_Returns200(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: true, ingestionRunning: false}
	r := setupClassifyStopRouter(stopper)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classify/stop", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, stopper.ingestionPauseCalled)
}

func TestAdminClassifyStop_OnlyIngestionActive_Returns200(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: false, ingestionRunning: true}
	r := setupClassifyStopRouter(stopper)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/classify/stop", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, stopper.ingestionPauseCalled)
}

func TestAdminClassifyStop_NoActiveRun_Returns409(t *testing.T) {
	stopper := &stubClassifyStopper{backfillCancelResult: false, ingestionRunning: false}
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
