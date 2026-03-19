package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(ctrl *JobEmbeddingController) *gin.Engine {
	r := gin.New()
	r.POST("/backfill", ctrl.StartBackfill)
	r.GET("/backfill/status", ctrl.GetStatus)
	return r
}

func TestStartBackfillReturns503WhenEmbeddingSvcNotConfigured(t *testing.T) {
	// nil embeddingSvc → Trigger returns ErrEmbeddingSvcNotConfigured
	backfillSvc := services.NewJobEmbeddingBackfillService(nil, nil, nil)
	ctrl := NewJobEmbeddingController(backfillSvc, context.Background())
	router := newTestRouter(ctrl)

	req := httptest.NewRequest(http.MethodPost, "/backfill", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestStartBackfillReturns400ForMalformedJSON(t *testing.T) {
	backfillSvc := services.NewJobEmbeddingBackfillService(nil, &services.EmbeddingService{}, nil)
	ctrl := NewJobEmbeddingController(backfillSvc, context.Background())
	router := newTestRouter(ctrl)

	body := strings.NewReader(`{"batch_size": "not-a-number"}`)
	req := httptest.NewRequest(http.MethodPost, "/backfill", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetStatusReturns200WithJSONBody(t *testing.T) {
	backfillSvc := services.NewJobEmbeddingBackfillService(nil, nil, nil)
	ctrl := NewJobEmbeddingController(backfillSvc, context.Background())
	router := newTestRouter(ctrl)

	req := httptest.NewRequest(http.MethodGet, "/backfill/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, ok := body["running"]; !ok {
		t.Errorf("response JSON missing 'running' field: %s", w.Body.String())
	}
}
