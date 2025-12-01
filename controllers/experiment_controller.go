package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"resumeai/models"
	"resumeai/services"
)

var allowedExperimentStatuses = map[string]bool{
	"draft":     true,
	"running":   true,
	"paused":    true,
	"completed": true,
}

type ExperimentController struct {
	service *services.ExperimentService
}

func NewExperimentController(service *services.ExperimentService) *ExperimentController {
	return &ExperimentController{service: service}
}

type variantRequest struct {
	Key         string `json:"key" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Weight      int    `json:"weight"`
	IsControl   bool   `json:"is_control"`
}

type experimentUpsertRequest struct {
	Key         string           `json:"key" binding:"required"`
	Name        string           `json:"name" binding:"required"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	StartAt     string           `json:"start_at"`
	EndAt       string           `json:"end_at"`
	Variants    []variantRequest `json:"variants" binding:"required"`
}

func (ec *ExperimentController) CreateOrUpdateExperiment(c *gin.Context) {
	var req experimentUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	if len(req.Variants) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one variant is required"})
		return
	}

	// Allow updating existing experiments by posting the same key
	status := normalizeStatus(req.Status)
	if !allowedExperimentStatuses[status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of draft, running, paused, or completed"})
		return
	}

	startAt, err := parseOptionalTime(req.StartAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_at timestamp; expected RFC3339"})
		return
	}
	endAt, err := parseOptionalTime(req.EndAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_at timestamp; expected RFC3339"})
		return
	}

	variantKeys := make(map[string]struct{})
	var variants []models.ExperimentVariant
	for _, variant := range req.Variants {
		key := strings.TrimSpace(variant.Key)
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "variant key cannot be empty"})
			return
		}
		if _, exists := variantKeys[key]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "variant keys must be unique"})
			return
		}
		variantKeys[key] = struct{}{}

		if variant.Weight < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("variant %s has negative weight", key)})
			return
		}

		weight := variant.Weight
		if weight == 0 {
			weight = 50
		}

		variants = append(variants, models.ExperimentVariant{
			VariantKey:  key,
			Name:        strings.TrimSpace(variant.Name),
			Description: strings.TrimSpace(variant.Description),
			Weight:      weight,
			IsControl:   variant.IsControl,
		})
	}

	if !atLeastOneControl(variants) {
		variants[0].IsControl = true
	}

	experiment := models.Experiment{
		Key:         strings.TrimSpace(req.Key),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Status:      status,
		StartAt:     startAt,
		EndAt:       endAt,
	}

	result, err := ec.service.UpsertExperiment(c.Request.Context(), experiment, variants)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save experiment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"experiment": result})
}

func (ec *ExperimentController) ListExperiments(c *gin.Context) {
	experiments, err := ec.service.ListExperiments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list experiments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"experiments": experiments})
}

func (ec *ExperimentController) GetExperimentMetrics(c *gin.Context) {
	key := c.Param("key")
	experiment, err := ec.service.GetExperimentWithMetrics(c.Request.Context(), key)
	if errors.Is(err, models.ErrExperimentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"experiment": experiment})
}

func (ec *ExperimentController) DeleteExperiment(c *gin.Context) {
	key := c.Param("key")
	if strings.TrimSpace(key) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "experiment key is required"})
		return
	}

	err := ec.service.DeleteExperiment(c.Request.Context(), key)
	switch {
	case errors.Is(err, models.ErrExperimentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete experiment"})
	default:
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

type assignmentRequest struct {
	ExperimentKey string `json:"experiment_key" binding:"required"`
	UserID        string `json:"user_id"`
	AnonymousID   string `json:"anonymous_id"`
	RequestPath   string `json:"request_path"`
	ForceReassign bool   `json:"force_reassign"`
}

func (ec *ExperimentController) AssignVariant(c *gin.Context) {
	var req assignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	expKey := strings.TrimSpace(req.ExperimentKey)
	if expKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "experiment_key is required"})
		return
	}

	userIdentifier := ec.resolveUserIdentifier(c, req.UserID, req.AnonymousID)

	result, err := ec.service.AssignVariant(c.Request.Context(), expKey, userIdentifier, req.RequestPath, req.ForceReassign)
	if errors.Is(err, models.ErrExperimentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign variant"})
		return
	}

	ec.persistUserIdentifier(c, userIdentifier)
	c.Header("X-Experiment-Key", expKey)
	c.Header("X-Experiment-Variant", result.Variant.VariantKey)
	c.Header("X-Experiment-User", userIdentifier)

	c.JSON(http.StatusOK, gin.H{
		"experiment": result.Experiment,
		"variant":    result.Variant,
		"assignment": result.Assignment,
		"user_id":    userIdentifier,
	})
}

type trackEventRequest struct {
	ExperimentKey string                 `json:"experiment_key" binding:"required"`
	EventName     string                 `json:"event_name" binding:"required"`
	VariantKey    string                 `json:"variant_key"`
	UserID        string                 `json:"user_id"`
	AnonymousID   string                 `json:"anonymous_id"`
	RequestPath   string                 `json:"request_path"`
	Metadata      map[string]interface{} `json:"metadata"`
}

func (ec *ExperimentController) TrackEvent(c *gin.Context) {
	var req trackEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	expKey := strings.TrimSpace(req.ExperimentKey)
	if expKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "experiment_key is required"})
		return
	}

	userIdentifier := ec.resolveUserIdentifier(c, req.UserID, req.AnonymousID)
	event, variant, err := ec.service.TrackEvent(c.Request.Context(), services.TrackEventRequest{
		ExperimentKey: expKey,
		EventName:     req.EventName,
		VariantKey:    req.VariantKey,
		UserID:        userIdentifier,
		RequestPath:   req.RequestPath,
		Metadata:      req.Metadata,
	})

	switch {
	case errors.Is(err, models.ErrExperimentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record event"})
		return
	}

	ec.persistUserIdentifier(c, userIdentifier)
	c.Header("X-Experiment-Key", expKey)
	c.Header("X-Experiment-Variant", variant.VariantKey)
	c.Header("X-Experiment-User", userIdentifier)

	c.JSON(http.StatusOK, gin.H{
		"event":      event,
		"variant":    variant,
		"user_id":    userIdentifier,
		"experiment": req.ExperimentKey,
	})
}

func (ec *ExperimentController) resolveUserIdentifier(c *gin.Context, explicitUser string, anonymousID string) string {
	if val, ok := c.Get("user_id"); ok {
		return fmt.Sprintf("user:%v", val)
	}

	trimmed := strings.TrimSpace(explicitUser)
	if trimmed != "" {
		return trimmed
	}

	anon := strings.TrimSpace(anonymousID)
	if anon != "" {
		return anon
	}

	header := strings.TrimSpace(c.GetHeader("X-Experiment-User"))
	if header != "" {
		return header
	}

	if cookie, err := c.Cookie("ab_user_id"); err == nil && strings.TrimSpace(cookie) != "" {
		return cookie
	}

	return fmt.Sprintf("anon:%s", uuid.NewString())
}

func (ec *ExperimentController) persistUserIdentifier(c *gin.Context, id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	c.SetCookie("ab_user_id", id, 60*60*24*60, "/", "", false, true)
}

func parseOptionalTime(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func normalizeStatus(status string) string {
	trimmed := strings.ToLower(strings.TrimSpace(status))
	if trimmed == "" {
		return "draft"
	}
	return trimmed
}

func atLeastOneControl(variants []models.ExperimentVariant) bool {
	for _, v := range variants {
		if v.IsControl {
			return true
		}
	}
	return false
}
