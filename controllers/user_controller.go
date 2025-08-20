package controllers

import (
	"encoding/json"
	"net/http"

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
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

// SaveResumeContactInfo creates a new resume entry with the extracted contact info from parsed resume
func (c *UserController) SaveResumeContactInfo(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Phone      string `json:"phone"`
		ResumeName string `json:"resume_name"` // Optional resume name
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Create a new resume entry with contact info
	resume, err := c.resumeModel.CreateWithContactInfo(userID.(int), req.ResumeName, req.Name, req.Email, req.Phone)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save contact info"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Contact info saved successfully",
		"resume_id": resume.ID,
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
		Summary json.RawMessage `json:"summary"`
		Skills  json.RawMessage `json:"skills"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	err := c.resumeModel.Save(userID.(int), "User Resume", req.Summary, req.Skills)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user data"})
		return
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

	resume, err := c.resumeModel.GetLatestByUserID(userID.(int))
	if err != nil {
		// If no resume found, return empty data
		ctx.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"summary": json.RawMessage("{}"),
				"skills":  json.RawMessage("{}"),
			},
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary": resume.Summary,
			"skills":  resume.Skills,
		},
	})
}
