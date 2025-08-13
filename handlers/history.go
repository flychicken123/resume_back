package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ResumeHistory represents a resume history entry
type ResumeHistory struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	ResumeName  string    `json:"resume_name"`
	S3Path      string    `json:"s3_path"`
	GeneratedAt time.Time `json:"generated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// AddResumeToHistory adds a new resume to the user's history
func AddResumeToHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var request struct {
			ResumeName string `json:"resume_name" binding:"required"`
			S3Path     string `json:"s3_path" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
			return
		}

		// Insert new resume history entry
		query := `
			INSERT INTO resume_history (user_id, resume_name, s3_path, generated_at)
			VALUES ($1, $2, $3, $4)
			RETURNING id, user_id, resume_name, s3_path, generated_at, created_at
		`

		var history ResumeHistory
		err := db.QueryRow(
			query,
			userID,
			request.ResumeName,
			request.S3Path,
			time.Now(),
		).Scan(
			&history.ID,
			&history.UserID,
			&history.ResumeName,
			&history.S3Path,
			&history.GeneratedAt,
			&history.CreatedAt,
		)

		if err != nil {
			log.Printf("Error adding resume to history: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add resume to history"})
			return
		}

		// Keep only the last 3 resumes for this user
		go cleanupOldResumes(db, userID.(int))

		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"message": "Resume added to history",
			"history": history,
		})
	}
}

// GetResumeHistory retrieves the last 3 resumes for a user
func GetResumeHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Query the last 3 resumes for this user
		query := `
			SELECT id, user_id, resume_name, s3_path, generated_at, created_at
			FROM resume_history
			WHERE user_id = $1
			ORDER BY generated_at DESC
			LIMIT 3
		`

		rows, err := db.Query(query, userID)
		if err != nil {
			log.Printf("Error querying resume history: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve resume history"})
			return
		}
		defer rows.Close()

		var history []ResumeHistory
		for rows.Next() {
			var h ResumeHistory
			err := rows.Scan(
				&h.ID,
				&h.UserID,
				&h.ResumeName,
				&h.S3Path,
				&h.GeneratedAt,
				&h.CreatedAt,
			)
			if err != nil {
				log.Printf("Error scanning resume history row: %v", err)
				continue
			}
			history = append(history, h)
		}

		if err = rows.Err(); err != nil {
			log.Printf("Error iterating resume history rows: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve resume history"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"history": history,
		})
	}
}

// DeleteResumeFromHistory deletes a specific resume from history
func DeleteResumeFromHistory(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context (set by auth middleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Get history ID from URL parameter
		historyIDStr := c.Param("id")
		historyID, err := strconv.Atoi(historyIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid history ID"})
			return
		}

		// Delete the resume history entry (only if it belongs to the user)
		query := `
			DELETE FROM resume_history
			WHERE id = $1 AND user_id = $2
		`

		result, err := db.Exec(query, historyID, userID)
		if err != nil {
			log.Printf("Error deleting resume from history: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resume from history"})
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Error getting rows affected: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete resume from history"})
			return
		}

		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Resume not found or not authorized to delete"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Resume deleted from history",
		})
	}
}

// cleanupOldResumes removes old resumes beyond the limit of 3
func cleanupOldResumes(db *sql.DB, userID int) {
	query := `
		DELETE FROM resume_history
		WHERE user_id = $1
		AND id NOT IN (
			SELECT id FROM resume_history
			WHERE user_id = $1
			ORDER BY generated_at DESC
			LIMIT 3
		)
	`

	_, err := db.Exec(query, userID)
	if err != nil {
		log.Printf("Error cleaning up old resumes: %v", err)
	}
}
