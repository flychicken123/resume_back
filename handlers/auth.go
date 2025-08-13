package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	Token string `json:"token" binding:"required"`
	Email string `json:"email" binding:"required,email"`
}

type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
	Token   string `json:"token,omitempty"`
}

type UserProfile struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Claims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// GenerateJWT creates a JWT token for the user
func GenerateJWT(userID int, email string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-secret-key"
	}

	expirationHours, _ := strconv.Atoi(os.Getenv("JWT_EXPIRATION_HOURS"))
	if expirationHours == 0 {
		expirationHours = 24
	}

	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expirationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateJWT validates and extracts user information from JWT token
func ValidateJWT(tokenString string) (*Claims, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "your-secret-key"
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}

// AuthMiddleware validates JWT token and sets user context
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("AuthMiddleware called for path: %s\n", c.Request.URL.Path)

		tokenString := c.GetHeader("Authorization")
		fmt.Printf("Authorization header: %s\n", tokenString)

		if tokenString == "" {
			fmt.Println("No Authorization header found")
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "Authorization header required",
			})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		fmt.Printf("Token (first 20 chars): %s...\n", tokenString[:min(20, len(tokenString))])

		claims, err := ValidateJWT(tokenString)
		if err != nil {
			fmt.Printf("Token validation failed: %v\n", err)
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "Invalid or expired token",
			})
			c.Abort()
			return
		}

		fmt.Printf("Token validated successfully for user: %s\n", claims.Email)

		// Set user information in context
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Next()
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func RegisterUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, AuthResponse{
				Success: false,
				Message: "Invalid request data: " + err.Error(),
			})
			return
		}

		// Check if user already exists
		var existingUser string
		err := db.QueryRow("SELECT email FROM users WHERE email = $1", req.Email).Scan(&existingUser)
		if err == nil {
			c.JSON(http.StatusConflict, AuthResponse{
				Success: false,
				Message: "User with this email already exists",
			})
			return
		}

		// Hash the password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to hash password",
			})
			return
		}

		// Insert new user
		var userID int
		err = db.QueryRow("INSERT INTO users (email, password, name, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
			req.Email, string(hashedPassword), req.Name, time.Now()).Scan(&userID)
		if err != nil {
			log.Printf("Database error during user creation: %v", err)
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to create user account: " + err.Error(),
			})
			return
		}

		// Generate JWT token
		token, err := GenerateJWT(userID, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to generate authentication token",
			})
			return
		}

		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "User registered successfully",
			User:    req.Email,
			Token:   token,
		})
	}
}

func LoginUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, AuthResponse{
				Success: false,
				Message: "Invalid request data: " + err.Error(),
			})
			return
		}

		// Get user from database
		var hashedPassword string
		var userID int
		var name string
		err := db.QueryRow("SELECT id, password, name FROM users WHERE email = $1", req.Email).Scan(&userID, &hashedPassword, &name)
		if err != nil {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "Invalid email or password",
			})
			return
		}

		// Compare password
		err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password))
		if err != nil {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "Invalid email or password",
			})
			return
		}

		// Generate JWT token
		token, err := GenerateJWT(userID, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to generate authentication token",
			})
			return
		}

		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "Login successful",
			User:    req.Email,
			Token:   token,
		})
	}
}

// GoogleLogin handles Google OAuth login
func GoogleLogin(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req GoogleLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, AuthResponse{
				Success: false,
				Message: "Invalid request data: " + err.Error(),
			})
			return
		}

		// For now, we'll just create or get the user based on email
		// In a production app, you'd want to verify the Google token
		var userID int
		var name string
		err := db.QueryRow("SELECT id, name FROM users WHERE email = $1", req.Email).Scan(&userID, &name)
		if err != nil {
			// User doesn't exist, create them
			err = db.QueryRow("INSERT INTO users (email, name, password) VALUES ($1, $2, $3) RETURNING id, name",
				req.Email, req.Email, "google_oauth_user").Scan(&userID, &name)
			if err != nil {
				c.JSON(http.StatusInternalServerError, AuthResponse{
					Success: false,
					Message: "Failed to create user account",
				})
				return
			}
		}

		// Generate JWT token
		token, err := GenerateJWT(userID, req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to generate authentication token",
			})
			return
		}

		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "Google login successful",
			User:    req.Email,
			Token:   token,
		})
	}
}

// GetUserProfile returns the current user's profile
func GetUserProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "User not authenticated",
			})
			return
		}

		var profile UserProfile
		err := db.QueryRow("SELECT id, email, name FROM users WHERE id = $1", userID).Scan(&profile.ID, &profile.Email, &profile.Name)
		if err != nil {
			c.JSON(http.StatusNotFound, AuthResponse{
				Success: false,
				Message: "User not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"profile": profile,
		})
	}
}

// UpdateUserProfile updates the user's profile information
func UpdateUserProfile(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "User not authenticated",
			})
			return
		}

		var req struct {
			Name string `json:"name" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, AuthResponse{
				Success: false,
				Message: "Invalid request data: " + err.Error(),
			})
			return
		}

		_, err := db.Exec("UPDATE users SET name = $1, updated_at = $2 WHERE id = $3", req.Name, time.Now(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to update profile",
			})
			return
		}

		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "Profile updated successfully",
		})
	}
}

// ChangePassword allows users to change their password
func ChangePassword(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "User not authenticated",
			})
			return
		}

		var req struct {
			CurrentPassword string `json:"current_password" binding:"required"`
			NewPassword     string `json:"new_password" binding:"required,min=6"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, AuthResponse{
				Success: false,
				Message: "Invalid request data: " + err.Error(),
			})
			return
		}

		// Get current password hash
		var hashedPassword string
		err := db.QueryRow("SELECT password FROM users WHERE id = $1", userID).Scan(&hashedPassword)
		if err != nil {
			c.JSON(http.StatusNotFound, AuthResponse{
				Success: false,
				Message: "User not found",
			})
			return
		}

		// Verify current password
		err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.CurrentPassword))
		if err != nil {
			c.JSON(http.StatusUnauthorized, AuthResponse{
				Success: false,
				Message: "Current password is incorrect",
			})
			return
		}

		// Hash new password
		newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to hash password",
			})
			return
		}

		// Update password
		_, err = db.Exec("UPDATE users SET password = $1, updated_at = $2 WHERE id = $3", string(newHashedPassword), time.Now(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, AuthResponse{
				Success: false,
				Message: "Failed to update password",
			})
			return
		}

		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "Password changed successfully",
		})
	}
}

// LogoutUser handles user logout (client-side token removal)
func LogoutUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "Logged out successfully",
		})
	}
}
