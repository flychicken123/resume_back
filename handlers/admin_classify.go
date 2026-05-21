package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ClassifyStopper is the minimal surface needed to halt explicit classification work.
type ClassifyStopper interface {
	CancelBackfill() bool
}

// ClassifyStopHandler returns an admin endpoint that halts an in-flight explicit
// classify backfill run.
//
// Response:
//   - 200 { "stopped": true, "scope": "backfill" } — a backfill was active and is now stopping.
//   - 409 { "stopped": false, "reason": "no active classification run" } — nothing was running.
func ClassifyStopHandler(stopper ClassifyStopper) gin.HandlerFunc {
	return func(c *gin.Context) {
		backfillCancelled := stopper.CancelBackfill()

		if !backfillCancelled {
			c.JSON(http.StatusConflict, gin.H{
				"stopped": false,
				"reason":  "no active classification run",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"stopped": true,
			"scope":   "backfill",
		})
	}
}
