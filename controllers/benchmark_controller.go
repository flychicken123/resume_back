package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

// BenchmarkController handles admin endpoints for AI benchmarking.
type BenchmarkController struct {
	svc       *services.BenchmarkService
	serverCtx context.Context
}

// NewBenchmarkController creates a new BenchmarkController.
func NewBenchmarkController(svc *services.BenchmarkService, serverCtx context.Context) *BenchmarkController {
	return &BenchmarkController{svc: svc, serverCtx: serverCtx}
}

// RunBenchmark starts a benchmark run in the background.
func (bc *BenchmarkController) RunBenchmark(c *gin.Context) {
	var req struct {
		Type       string `json:"type"`
		SampleSize int    `json:"sample_size"`
		UserID     int    `json:"user_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Type == "" {
		req.Type = "all"
	}
	if req.SampleSize <= 0 {
		req.SampleSize = 20
	}

	runID, err := bc.svc.RunBenchmark(bc.serverCtx, req.Type, req.SampleSize, req.UserID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "started", "run_id": runID})
}

// GetStatus returns the current benchmark run status.
func (bc *BenchmarkController) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, bc.svc.Status())
}

// GetResults returns results for a specific benchmark type or run.
func (bc *BenchmarkController) GetResults(c *gin.Context) {
	runID := c.Query("run_id")
	benchmarkType := c.Query("type")

	if runID != "" {
		results, err := bc.svc.GetResultsByRunID(runID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load results"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
		return
	}

	if benchmarkType != "" {
		results, err := bc.svc.GetLatestResultsByType(benchmarkType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load results"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "provide type or run_id query parameter"})
}

// GetHistory returns a list of past benchmark runs.
func (bc *BenchmarkController) GetHistory(c *gin.Context) {
	history, err := bc.svc.GetHistory()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": history})
}

// GetSummary returns the latest score for each benchmark type.
func (bc *BenchmarkController) GetSummary(c *gin.Context) {
	summary, err := bc.svc.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load summary"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"systems": summary})
}
