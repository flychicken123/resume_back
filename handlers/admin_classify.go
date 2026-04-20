package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ClassifyStopper is the minimal surface needed to halt classification work.
// The production adapter wraps both the backfill service and the ingestion
// classifier so a single admin call stops both paths.
type ClassifyStopper interface {
	CancelBackfill() bool
	PauseIngestionClassifier()
	IsIngestionClassifierRunning() bool
}

// ClassifyStopHandler returns an admin endpoint that halts in-flight
// classification runs (both the explicit backfill and the ingestion-driven path).
//
// Response:
//   - 200 { "stopped": true, "scope": "backfill_and_ingestion" } — at least one path was active and is now stopping.
//   - 409 { "stopped": false, "reason": "no active classification run" } — nothing was running.
func ClassifyStopHandler(stopper ClassifyStopper) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfillCancelled := stopper.CancelBackfill()
		ingestionWasRunning := stopper.IsIngestionClassifierRunning()
		if ingestionWasRunning {
			stopper.PauseIngestionClassifier()
		}

		if !backfillCancelled && !ingestionWasRunning {
			c.JSON(http.StatusConflict, gin.H{
				"stopped": false,
				"reason":  "no active classification run",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"stopped": true,
			"scope":   "backfill_and_ingestion",
		})
	}
}
