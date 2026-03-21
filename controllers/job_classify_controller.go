package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

// JobClassifyController handles admin endpoints for job classify backfill.
type JobClassifyController struct {
	svc       *services.JobClassifyBackfillService
	serverCtx context.Context
}

// NewJobClassifyController creates a new JobClassifyController.
func NewJobClassifyController(svc *services.JobClassifyBackfillService, serverCtx context.Context) *JobClassifyController {
	return &JobClassifyController{svc: svc, serverCtx: serverCtx}
}

// StartBackfill triggers the classify backfill.
func (c *JobClassifyController) StartBackfill(gc *gin.Context) {
	var req struct {
		BatchSize int `json:"batch_size"`
		SinceDays int `json:"since_days"`
	}
	_ = gc.ShouldBindJSON(&req)
	if req.BatchSize <= 0 {
		req.BatchSize = 20
	}

	if err := c.svc.Trigger(c.serverCtx, req.BatchSize, req.SinceDays); err != nil {
		if err == services.ErrClassifyAlreadyRunning {
			gc.JSON(http.StatusConflict, gin.H{"error": "backfill already running"})
			return
		}
		gc.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	gc.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

// GetStatus returns the current backfill status.
func (c *JobClassifyController) GetStatus(gc *gin.Context) {
	gc.JSON(http.StatusOK, c.svc.Status())
}

// StopBackfill stops the running backfill.
func (c *JobClassifyController) StopBackfill(gc *gin.Context) {
	c.svc.Stop()
	gc.JSON(http.StatusOK, gin.H{"status": "stopping"})
}
