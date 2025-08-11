package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"resumeai/config"
	"resumeai/database"
	"resumeai/handlers"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	dbConfig := config.GetDatabaseConfig()

	log.Printf("Database Config - Host: %s, Port: %d, User: %s, DB: %s, SSL: %s",
		dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.DBName, dbConfig.SSLMode)

	db, err := database.Connect(
		dbConfig.Host,
		fmt.Sprintf("%d", dbConfig.Port),
		dbConfig.User,
		dbConfig.Password,
		dbConfig.DBName,
		dbConfig.SSLMode,
	)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}
	defer db.Close()

	log.Println("✅ Database connection successful!")

	r := gin.Default()

	// Allow larger multipart uploads (HTML file uploads)
	r.MaxMultipartMemory = 32 << 20 // 32 MiB (increased from 16 MiB)

	// Add middleware to handle large request errors
	r.Use(func(c *gin.Context) {
		c.Next()

		// Check if we have a 413 error
		if c.Writer.Status() == http.StatusRequestEntityTooLarge {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":    "File too large. Please ensure your resume file is under 32MB.",
				"max_size": "32MB",
			})
		}
	})

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://hihired.org", "https://www.hihired.org", "http://localhost:3000", "http://127.0.0.1:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Forwarded-Host", "X-Forwarded-Port", "Content-Length"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
	}))

	// Ensure all OPTIONS preflight requests succeed with proper CORS headers
	r.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			origin := c.Request.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Forwarded-Host,X-Forwarded-Port,Content-Length")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Serve static files without /api prefix
	r.Static("/static", "./static")

	// Add a dedicated endpoint for PDF downloads with proper headers
	r.GET("/download/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filepath := "./static/" + filename

		log.Printf("Download request for file: %s", filename)
		log.Printf("Full filepath: %s", filepath)

		// Check if file exists
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			log.Printf("File not found: %s", filepath)
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		// Get file info for debugging
		if fileInfo, err := os.Stat(filepath); err == nil {
			log.Printf("File found: %s, size: %d bytes", filepath, fileInfo.Size())
		}

		// Set proper headers for file download
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		c.Header("Cache-Control", "no-cache")

		// Set CORS headers for cross-origin requests
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")

		// Serve the file
		c.File(filepath)
	})

	// Add OPTIONS handler for download endpoint
	r.OPTIONS("/download/:filename", func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		c.Status(http.StatusNoContent)
	})

	// API routes
	api := r.Group("/api")
	{
		api.GET("/version", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"version":     "1.0.1",
				"build_time":  time.Now().Format("2006-01-02 15:04:05"),
				"pdf_margins": "zero_margins_v2",
			})
		})

		api.POST("/auth/register", handlers.RegisterUser(db))
		api.POST("/auth/login", handlers.LoginUser(db))
		api.POST("/auth/logout", handlers.LogoutUser())
	}

	// Public routes (no auth required)
	public := r.Group("/api")
	{
		public.POST("/experience/optimize", handlers.OptimizeExperience)
		public.POST("/resume/generate", handlers.GenerateResume)
		public.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		public.POST("/resume/generate-pdf-file", handlers.GeneratePDFResumeFromHTMLFile)
		public.POST("/resume/parse", handlers.ParseResume)
		public.OPTIONS("/resume/parse", func(c *gin.Context) {
			origin := c.Request.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Forwarded-Host,X-Forwarded-Port,Content-Length")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
			c.Status(http.StatusNoContent)
		})
		public.POST("/ai/education", handlers.OptimizeEducation)
		public.POST("/ai/summary", handlers.OptimizeSummary)
	}

	// Protected routes (require auth)
	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())

	{
		protected.GET("/user/profile", handlers.GetUserProfile(db))
		protected.PUT("/user/profile", handlers.UpdateUserProfile(db))
		protected.POST("/user/change-password", handlers.ChangePassword(db))
		protected.POST("/user/save", handlers.SaveUserData(db))
		protected.GET("/user/load", handlers.LoadUserData(db))
	}

	log.Println("Server starting on port 8081")
	if err := r.Run(":8081"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
