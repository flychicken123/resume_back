package controllers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"resumeai/services"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type SubscriptionController struct {
	db            *sql.DB
	stripeService *services.StripeService
}

func NewSubscriptionController(db *sql.DB, stripeService *services.StripeService) *SubscriptionController {
	return &SubscriptionController{
		db:            db,
		stripeService: stripeService,
	}
}

// GetPlans returns all available subscription plans
func (sc *SubscriptionController) GetPlans(c *gin.Context) {
	rows, err := sc.db.Query(`
		SELECT id, name, display_name, price, resume_limit, resume_period, features
		FROM subscription_plans
		WHERE is_active = true
		ORDER BY price ASC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch plans"})
		return
	}
	defer rows.Close()

	var plans []map[string]interface{}
	for rows.Next() {
		var id int
		var name, displayName, resumePeriod string
		var price float64
		var resumeLimit int
		var features json.RawMessage

		err := rows.Scan(&id, &name, &displayName, &price, &resumeLimit, &resumePeriod, &features)
		if err != nil {
			continue
		}

		var featuresObj map[string]interface{}
		if err := json.Unmarshal(features, &featuresObj); err != nil {
			featuresObj = map[string]interface{}{}
		}

		plans = append(plans, map[string]interface{}{
			"id":            id,
			"name":          name,
			"display_name":  displayName,
			"price":         price,
			"resume_limit":  resumeLimit,
			"resume_period": resumePeriod,
			"features":      featuresObj,
		})
	}

	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

// GetCurrentSubscription returns user's current subscription
func (sc *SubscriptionController) GetCurrentSubscription(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var subscription struct {
		PlanName           string     `json:"plan_name"`
		DisplayName        string     `json:"display_name"`
		Price              float64    `json:"price"`
		Status             string     `json:"status"`
		ResumeLimit        int        `json:"resume_limit"`
		ResumePeriod       string     `json:"resume_period"`
		CancelAtPeriodEnd  bool       `json:"cancel_at_period_end"`
		CurrentPeriodStart *time.Time `json:"current_period_start"`
		CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	}

	var periodStart sql.NullTime
	var periodEnd sql.NullTime

	err := sc.db.QueryRow(`
		SELECT sp.name, sp.display_name, sp.price,
		       COALESCE(us.status, 'free'), sp.resume_limit, sp.resume_period,
		       COALESCE(us.cancel_at_period_end, false) AS cancel_at_period_end,
		       us.current_period_start,
		       us.current_period_end
		FROM users u
		LEFT JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
		LEFT JOIN user_subscriptions us ON us.user_id = u.id
		WHERE u.id = $1
	`, userID).Scan(&subscription.PlanName, &subscription.DisplayName,
		&subscription.Price, &subscription.Status, &subscription.ResumeLimit,
		&subscription.ResumePeriod, &subscription.CancelAtPeriodEnd,
		&periodStart, &periodEnd)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscription"})
		return
	}

	if periodStart.Valid {
		start := periodStart.Time
		subscription.CurrentPeriodStart = &start
	}
	if periodEnd.Valid {
		end := periodEnd.Time
		subscription.CurrentPeriodEnd = &end
	}

	// Get usage information (with fallback for errors)
	canGenerate := true
	remaining := subscription.ResumeLimit
	resetDate := time.Now().AddDate(0, 1, 0)

	// Try to get actual usage, but don't fail if it errors
	actualCanGenerate, actualRemaining, actualResetDate, err := sc.stripeService.CheckUsageLimit(userID)
	if err == nil {
		canGenerate = actualCanGenerate
		remaining = actualRemaining
		resetDate = actualResetDate
	} else {
		// Log the error but continue
		fmt.Printf("Warning: Failed to check usage limit: %v\n", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"subscription": subscription,
		"usage": gin.H{
			"can_generate": canGenerate,
			"remaining":    remaining,
			"limit":        subscription.ResumeLimit,
			"reset_date":   resetDate,
		},
	})
}

// CreateCheckoutSession creates a Stripe checkout session
func (sc *SubscriptionController) CreateCheckoutSession(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		PlanName string `json:"plan_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check if plan is valid and not free
	if req.PlanName == "free" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot checkout free plan"})
		return
	}

	// Create success and cancel URLs
	baseURL := c.Request.Header.Get("Origin")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	successURL := baseURL + "/subscription/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := baseURL + "/pricing"

	session, err := sc.stripeService.CreateCheckoutSession(userID, req.PlanName, successURL, cancelURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create checkout session", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"checkout_url": session.URL,
		"session_id":   session.ID,
	})
}

// HandleStripeWebhook handles Stripe webhook events

// ChangePlan updates the user's active subscription to a different paid plan
func (sc *SubscriptionController) ChangePlan(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		PlanName string `json:"plan_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	planName := strings.TrimSpace(strings.ToLower(req.PlanName))
	if planName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plan name is required"})
		return
	}
	if planName == "free" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Use the cancel endpoint to switch to the free plan"})
		return
	}

	updatedSub, err := sc.stripeService.ChangeSubscriptionPlan(userID, planName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Subscription updated",
		"subscription": gin.H{
			"plan_name":            planName,
			"status":               updatedSub.Status,
			"current_period_start": time.Unix(updatedSub.CurrentPeriodStart, 0),
			"current_period_end":   time.Unix(updatedSub.CurrentPeriodEnd, 0),
		},
	})
}

func (sc *SubscriptionController) HandleStripeWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing Stripe signature"})
		return
	}

	err = sc.stripeService.HandleWebhook(payload, signature)
	if err != nil {
		fmt.Printf("Webhook error: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

// CancelSubscription cancels the user's subscription
func (sc *SubscriptionController) CancelSubscription(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	sub, err := sc.stripeService.CancelUserSubscription(userID)
	if err != nil {
		if errors.Is(err, services.ErrNoActiveSubscription) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No active subscription to cancel"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":              "Subscription will be canceled at the end of the current period",
		"status":               sub.Status,
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
		"current_period_end":   time.Unix(sub.CurrentPeriodEnd, 0),
	})
}

// GetUsageStats returns usage statistics for the user
func (sc *SubscriptionController) GetUsageStats(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get current usage
	canGenerate, remaining, resetDate, err := sc.stripeService.CheckUsageLimit(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch usage stats"})
		return
	}

	// Get historical usage for the current period
	var totalUsed int
	err = sc.db.QueryRow(`
		SELECT COALESCE(resume_count, 0)
		FROM resume_usage
		WHERE user_id = $1
		AND period_start <= CURRENT_TIMESTAMP
		AND period_end > CURRENT_TIMESTAMP
	`, userID).Scan(&totalUsed)

	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch usage history"})
		return
	}

	// Get plan details
	var planLimit int
	var planPeriod string
	err = sc.db.QueryRow(`
		SELECT sp.resume_limit, sp.resume_period
		FROM users u
		JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
		WHERE u.id = $1
	`, userID).Scan(&planLimit, &planPeriod)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch plan details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"usage": gin.H{
			"can_generate": canGenerate,
			"remaining":    remaining,
			"total_used":   totalUsed,
			"limit":        planLimit,
			"period":       planPeriod,
			"reset_date":   resetDate,
		},
	})
}

// CreateCustomerPortal creates a Stripe customer portal session
func (sc *SubscriptionController) CreateCustomerPortal(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	baseURL := c.Request.Header.Get("Origin")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	returnURL := baseURL + "/account"

	portalURL, err := sc.stripeService.CreateCustomerPortalSession(userID, returnURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create portal session", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"portal_url": portalURL})
}

// CheckResumeLimit checks if user can generate a resume
func (sc *SubscriptionController) CheckResumeLimit(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		// Try to get from auth context
		userID := c.GetInt("user_id")
		if userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}
		userIDStr = strconv.Itoa(userID)
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	canGenerate, remaining, resetDate, err := sc.stripeService.CheckUsageLimit(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check limit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"can_generate": canGenerate,
		"remaining":    remaining,
		"reset_date":   resetDate,
	})
}

// ConfirmSuccess finalizes a subscription after returning from Stripe Checkout (fallback when webhooks are not available)
func (sc *SubscriptionController) ConfirmSuccess(c *gin.Context) {
	userID := c.GetInt("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			sessionID = req.SessionID
		}
	}
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	if err := sc.stripeService.ConfirmCheckoutSession(userID, sessionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
