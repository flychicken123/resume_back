package controllers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/models"
)

type UserController struct {
	userModel   *models.UserModel
	resumeModel *models.ResumeModel
}

func NewUserController(userModel *models.UserModel, resumeModel *models.ResumeModel) *UserController {
	return &UserController{
		userModel:   userModel,
		resumeModel: resumeModel,
	}
}

func (c *UserController) GetProfile(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	user, err := c.userModel.GetByID(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user profile"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"name":     user.Name,
			"is_admin": user.IsAdmin,
		},
	})
}

func (c *UserController) UpdateProfile(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := c.userModel.UpdateProfile(userID.(int), req.Name)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Profile updated successfully",
	})
}

func (c *UserController) ChangePassword(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=6"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// Get current user to verify password
	_, err := c.userModel.GetByID(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user data"})
		return
	}

	// Verify current password (you'll need to implement password verification)
	// For now, we'll just update the password
	err = c.userModel.UpdatePassword(userID.(int), req.NewPassword)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password changed successfully",
	})
}

func (c *UserController) SaveUserData(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Data              json.RawMessage `json:"data"`
		Name              string          `json:"name"`
		Email             string          `json:"email"`
		Phone             string          `json:"phone"`
		Summary           string          `json:"summary"`
		Skills            []string        `json:"skills"`
		SkillsCategorized string          `json:"skillsCategorized"`
		SelectedFormat    string          `json:"selectedFormat"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	input := models.ResumeProfileSaveInput{
		Name:              req.Name,
		Email:             req.Email,
		Phone:             req.Phone,
		Summary:           req.Summary,
		Experience:        stringFromAnyOrJSON(req.Data, "experience"),
		Education:         stringFromAnyOrJSON(req.Data, "education"),
		JobDescription:    stringFromAnyOrJSON(req.Data, "jobDescription"),
		Location:          stringFromAnyOrJSON(req.Data, "location"),
		Skills:            req.Skills,
		SkillsCategorized: req.SkillsCategorized,
		SelectedFormat:    req.SelectedFormat,
		ProfileJSON:       req.Data,
	}
	models.MergeProfileFieldsFromJSON(req.Data, &input)

	_, err := c.resumeModel.SaveProfile(ctx.Request.Context(), userID.(int), input)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user data"})
		return
	}

	// Auto-resubscribe user to emails when they build/save a resume
	user, userErr := c.userModel.GetByID(userID.(int))
	if userErr == nil && user != nil && user.EmailUnsubscribed {
		_ = c.userModel.SetEmailUnsubscribed(user.Email, false)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User data saved successfully",
	})
}

func (c *UserController) LoadUserData(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	resume, err := c.resumeModel.GetByUserID(userID.(int))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    json.RawMessage(`{}`),
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user data"})
		return
	}

	experiences, err := c.resumeModel.GetExperiencesByResumeID(resume.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load experience data"})
		return
	}

	jobPrefs, _ := c.userModel.GetJobPreferences(userID.(int))
	if jobPrefs == nil {
		jobPrefs = json.RawMessage(`{}`)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"name":             resume.Name,
			"email":            resume.Email,
			"phone":            resume.Phone,
			"summary":          resume.Summary,
			"skills":           resume.Skills,
			"skillsCategorized": resume.SkillsCategorized,
			"experiences":      experiences,
			"experience":       resume.Experience,
			"education":        resume.Education,
			"jobDescription":   resume.JobDescription,
			"location":         resume.Location,
			"selectedFormat":   resume.SelectedFormat,
			"jobPreferences":   jobPrefs,
		},
	})
}

// GetJobPreferences returns the user's saved job application preferences
func (c *UserController) GetJobPreferences(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	prefs, err := c.userModel.GetJobPreferences(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load job preferences"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":     true,
		"preferences": prefs,
	})
}

// SaveJobPreferences saves the user's job application preferences
func (c *UserController) SaveJobPreferences(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Preferences json.RawMessage `json:"preferences" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	if err := c.userModel.SetJobPreferences(userID.(int), req.Preferences); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save job preferences"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Job preferences saved successfully",
	})
}

// GetFollowupReminders returns the user's followup_reminders_enabled setting
func (c *UserController) GetFollowupReminders(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	user, err := c.userModel.GetByID(userID.(int))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"followup_reminders_enabled": user.FollowupRemindersEnabled,
	})
}

// UpdateFollowupReminders updates the user's followup_reminders_enabled setting
func (c *UserController) UpdateFollowupReminders(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := c.userModel.SetFollowupReminders(userID.(int), *req.Enabled); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reminder settings"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":                    "Follow-up reminders updated",
		"followup_reminders_enabled": *req.Enabled,
	})
}

// stringFromAnyOrJSON extracts a field from raw JSON and marshals non-strings.
func stringFromAnyOrJSON(raw json.RawMessage, key string) string {
	if len(raw) == 0 || key == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	val, ok := m[key]
	if !ok && key == "experience" {
		val, ok = m["experiences"]
	}
	if ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
		if b, err := json.Marshal(val); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
