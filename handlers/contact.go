package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/services"
)

type contactRequest struct {
	Name    string `json:"name" binding:"required"`
	Email   string `json:"email" binding:"required"`
	Message string `json:"message" binding:"required"`
}

func CreateContactRequest(db *sql.DB, emailSvc *services.EmailService) gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, _ := c.GetRawData()
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		var req contactRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			ctype := c.GetHeader("Content-Type")
			log.Printf("contact request bind failed: content_type=%q body_len=%d err=%v", ctype, len(bodyBytes), err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid contact request"})
			return
		}

		name := strings.TrimSpace(req.Name)
		email := strings.TrimSpace(req.Email)
		message := strings.TrimSpace(req.Message)

		if name == "" || email == "" || message == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Name, email, and message are required"})
			return
		}

		_, err := db.Exec(
			`INSERT INTO contact_requests (name, email, message) VALUES ($1, $2, $3)`,
			name, email, message,
		)
		if err != nil {
			log.Printf("failed to persist contact request: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not submit request"})
			return
		}

		if emailSvc != nil && emailSvc.Enabled() {
			subject := fmt.Sprintf("HiHired contact request from %s", name)
			body := fmt.Sprintf("Name: %s\nEmail: %s\n\nMessage:\n%s", name, email, message)
			if err := emailSvc.SendSupportEmail(subject, body); err != nil {
				log.Printf("failed to send support notification email: %v", err)
			}
		}

		c.Status(http.StatusNoContent)
	}
}
