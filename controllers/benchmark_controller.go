package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/models"
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

// GetResults returns results for a specific benchmark type or run, with summary and insights.
func (bc *BenchmarkController) GetResults(c *gin.Context) {
	runID := c.Query("run_id")
	benchmarkType := c.Query("type")

	var results []models.AiBenchmarkResult
	var err error

	if runID != "" {
		results, err = bc.svc.GetResultsByRunID(runID)
	} else if benchmarkType != "" {
		results, err = bc.svc.GetLatestResultsByType(benchmarkType)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide type or run_id query parameter"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load results"})
		return
	}

	// Build failures list
	var failures []models.AiBenchmarkResult
	for _, r := range results {
		if (r.IsCorrect != nil && !*r.IsCorrect) || (r.Score != nil && *r.Score < 3.0) {
			failures = append(failures, r)
		}
	}

	// Build per-field summary
	summary := map[string]interface{}{}
	fieldCorrect := map[string]int{}
	fieldTotal := map[string]int{}
	fieldScoreSum := map[string]float64{}
	fieldScoreCount := map[string]int{}
	for _, r := range results {
		if r.IsCorrect != nil {
			fieldTotal[r.FieldName]++
			if *r.IsCorrect {
				fieldCorrect[r.FieldName]++
			}
		}
		if r.Score != nil {
			fieldScoreSum[r.FieldName] += *r.Score
			fieldScoreCount[r.FieldName]++
		}
	}
	for field, total := range fieldTotal {
		summary[field+"_accuracy"] = float64(fieldCorrect[field]) / float64(total)
	}
	for field, count := range fieldScoreCount {
		if _, hasAccuracy := fieldTotal[field]; !hasAccuracy {
			summary[field+"_avg"] = fieldScoreSum[field] / float64(count)
		}
	}

	insights := services.GenerateInsights(results)

	detectedRunID := ""
	detectedType := ""
	if len(results) > 0 {
		detectedRunID = results[0].RunID
		detectedType = results[0].BenchmarkType
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id":         detectedRunID,
		"benchmark_type": detectedType,
		"sample_size":    len(results),
		"summary":        summary,
		"details":        results,
		"failures":       failures,
		"insights":       insights,
	})
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
