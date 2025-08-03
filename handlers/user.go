package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserData struct {
	ResumeData map[string]interface{} `json:"resumeData"`
	UserID     int                    `json:"user_id"`
}

type SaveUserDataRequest struct {
	ResumeData map[string]interface{} `json:"resumeData" binding:"required"`
}

func SaveUserData(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "User not authenticated",
			})
			return
		}

		var req SaveUserDataRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid request data: " + err.Error(),
			})
			return
		}

		// Convert resume data to JSON string
		resumeDataJSON, err := json.Marshal(req.ResumeData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to serialize resume data",
			})
			return
		}

		// Check if user already has a resume record
		var existingResumeID int
		err = db.QueryRow("SELECT id FROM resumes WHERE user_id = $1", userID).Scan(&existingResumeID)

		if err == sql.ErrNoRows {
			// Create new resume record
			_, err = db.Exec("INSERT INTO resumes (user_id, name, summary, skills, created_at) VALUES ($1, $2, $3, $4, NOW())",
				userID, "My Resume", string(resumeDataJSON), "")
		} else if err == nil {
			// Update existing resume record
			_, err = db.Exec("UPDATE resumes SET summary = $1, updated_at = NOW() WHERE user_id = $2",
				string(resumeDataJSON), userID)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save user data",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "User data saved successfully",
		})
	}
}

func LoadUserData(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "User not authenticated",
			})
			return
		}

		var resumeDataJSON string
		err := db.QueryRow("SELECT summary FROM resumes WHERE user_id = $1", userID).Scan(&resumeDataJSON)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    map[string]interface{}{},
			})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to load user data",
			})
			return
		}

		// Parse JSON string back to map
		var resumeData map[string]interface{}
		err = json.Unmarshal([]byte(resumeDataJSON), &resumeData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to parse user data",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    resumeData,
		})
	}
}
