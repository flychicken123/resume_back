package controllers

import (
	"database/sql"
	"net/http"
	"resumeai/services"

	"github.com/gin-gonic/gin"
)

// AdsController handles ad-related API endpoints
type AdsController struct {
	db            *sql.DB
	adService     *services.AdService
	stripeService *services.StripeService
}

// NewAdsController creates a new AdsController instance
func NewAdsController(db *sql.DB, adService *services.AdService, stripeService *services.StripeService) *AdsController {
	return &AdsController{
		db:            db,
		adService:     adService,
		stripeService: stripeService,
	}
}

// GetAdStatus returns the current ad watch status for the user
// GET /api/ads/status
func (ac *AdsController) GetAdStatus(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	status := ac.adService.GetAdStatus(userID)

	c.JSON(http.StatusOK, status)
}

// AdWatchComplete handles the ad watch completion and grants credit
// POST /api/ads/watch-complete
func (ac *AdsController) AdWatchComplete(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Check cooldown first
	canWatch, cooldownRemaining := ac.adService.CanWatchAd(userID)
	if !canWatch {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":                      "Please wait before watching another ad",
			"cooldown_remaining_seconds": cooldownRemaining,
		})
		return
	}

	// Grant credit (decrement usage)
	err := ac.adService.GrantResumeCredit(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to grant credit: " + err.Error()})
		return
	}

	// Get updated usage stats using the stripe service
	canGenerate, remaining, resetDate, err := ac.stripeService.CheckUsageLimit(userID)
	if err != nil {
		// Still return success even if we can't get updated stats
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "You earned 1 free resume!",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "You earned 1 free resume!",
		"usage": gin.H{
			"can_generate": canGenerate,
			"remaining":    remaining,
			"reset_date":   resetDate,
		},
	})
}
