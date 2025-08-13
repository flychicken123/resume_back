package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"resumeai/models"
	"resumeai/services"
)

type AuthController struct {
	userModel  *models.UserModel
	jwtService *services.JWTService
}

func NewAuthController(userModel *models.UserModel, jwtService *services.JWTService) *AuthController {
	return &AuthController{
		userModel:  userModel,
		jwtService: jwtService,
	}
}

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Name     string `json:"name" binding:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
	Token   string `json:"token,omitempty"`
}

func (c *AuthController) Register(ctx *gin.Context) {
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, AuthResponse{
			Success: false,
			Message: "Invalid request data: " + err.Error(),
		})
		return
	}

	// Check if user already exists
	existingUser, err := c.userModel.GetByEmail(req.Email)
	if err == nil && existingUser != nil {
		ctx.JSON(http.StatusConflict, AuthResponse{
			Success: false,
			Message: "User with this email already exists",
		})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to hash password",
		})
		return
	}

	// Create user
	user, err := c.userModel.Create(req.Email, req.Name, string(hashedPassword))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to create user account",
		})
		return
	}

	// Generate JWT token
	token, err := c.jwtService.GenerateToken(user.ID, user.Email)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to generate authentication token",
		})
		return
	}

	ctx.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "Registration successful",
		User:    user.Email,
		Token:   token,
	})
}

func (c *AuthController) Login(ctx *gin.Context) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, AuthResponse{
			Success: false,
			Message: "Invalid request data: " + err.Error(),
		})
		return
	}

	// Get user by email
	user, err := c.userModel.GetByEmail(req.Email)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, AuthResponse{
			Success: false,
			Message: "Invalid email or password",
		})
		return
	}

	// Check password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, AuthResponse{
			Success: false,
			Message: "Invalid email or password",
		})
		return
	}

	// Generate JWT token
	token, err := c.jwtService.GenerateToken(user.ID, user.Email)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to generate authentication token",
		})
		return
	}

	ctx.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "Login successful",
		User:    user.Email,
		Token:   token,
	})
}

func (c *AuthController) GoogleLogin(ctx *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, AuthResponse{
			Success: false,
			Message: "Invalid request data: " + err.Error(),
		})
		return
	}

	// Check if user exists, create if not
	user, err := c.userModel.GetByEmail(req.Email)
	if err != nil {
		// User doesn't exist, create them
		user, err = c.userModel.Create(req.Email, req.Email, "google_oauth_user")
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to create user account",
			})
			return
		}
	}

	// Generate JWT token
	token, err := c.jwtService.GenerateToken(user.ID, user.Email)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, AuthResponse{
			Success: false,
			Message: "Failed to generate authentication token",
		})
		return
	}

	ctx.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "Google login successful",
		User:    user.Email,
		Token:   token,
	})
}
