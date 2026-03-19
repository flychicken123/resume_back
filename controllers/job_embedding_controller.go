package controllers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

// JobEmbeddingController handles admin endpoints for the job embedding backfill.
type JobEmbeddingController struct {
	svc       *services.JobEmbeddingBackfillService
	serverCtx context.Context
}

// NewJobEmbeddingController constructs a JobEmbeddingController.
// serverCtx is the server root context used to control the backfill goroutine lifetime.
func NewJobEmbeddingController(svc *services.JobEmbeddingBackfillService, serverCtx context.Context) *JobEmbeddingController {
	return &JobEmbeddingController{svc: svc, serverCtx: serverCtx}
}

// StartBackfill handles POST /api/admin/jobs/embeddings/backfill.
// Accepts an optional JSON body: {"batch_size": 500, "since_days": 30}.
// since_days=0 (default) embeds all active jobs with no embedding.
func (c *JobEmbeddingController) StartBackfill(ctx *gin.Context) {
	var req struct {
		BatchSize int `json:"batch_size"`
		SinceDays int `json:"since_days"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}

	err := c.svc.Trigger(c.serverCtx, batchSize, req.SinceDays)
	if errors.Is(err, services.ErrAlreadyRunning) {
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, services.ErrEmbeddingSvcNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

// GetStatus handles GET /api/admin/jobs/embeddings/backfill/status.
func (c *JobEmbeddingController) GetStatus(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.svc.Status())
}

// StopBackfill handles DELETE /api/admin/jobs/embeddings/backfill.
func (c *JobEmbeddingController) StopBackfill(ctx *gin.Context) {
	c.svc.Stop()
	ctx.JSON(http.StatusOK, gin.H{"status": "stopping"})
}
