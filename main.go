package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"resumeai/config"
	"resumeai/controllers"
	"resumeai/database"
	"resumeai/handlers"
	"resumeai/models"
	"resumeai/services"
	"time"

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

	// Initialize models
	userModel := models.NewUserModel(db)
	resumeHistoryModel := models.NewResumeHistoryModel(db)
	resumeModel := models.NewResumeModel(db)

	// Initialize services
	jwtService := services.NewJWTService(config.GetAppConfig().JWTSecret)
	s3Service, err := services.NewS3Service()
	if err != nil {
		log.Fatal("Error initializing S3 service:", err)
	}
	resumeService := services.NewResumeService(resumeHistoryModel, s3Service)

	// Initialize controllers
	authController := controllers.NewAuthController(userModel, jwtService)
	resumeController := controllers.NewResumeController(resumeHistoryModel, resumeService)
	userController := controllers.NewUserController(userModel, resumeModel)

	r := gin.Default()

	// Allow larger multipart uploads (HTML file uploads)
	r.MaxMultipartMemory = 8 << 20 // 8 MiB (sufficient for typical resume files)

	// Add middleware to handle large request errors
	r.Use(func(c *gin.Context) {
		c.Next()

		// Check if we have a 413 error
		if c.Writer.Status() == http.StatusRequestEntityTooLarge {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":    "File too large. Please ensure your resume file is under 8MB.",
				"max_size": "8MB",
			})
		}
	})

	// CORS middleware - only for local development (when nginx is not present)
	r.Use(func(c *gin.Context) {
		// If we have X-Forwarded headers, we're behind nginx (production)
		// Let nginx handle CORS
		if c.GetHeader("X-Forwarded-For") != "" || c.GetHeader("X-Forwarded-Proto") != "" {
			if c.Request.Method == http.MethodOptions {
				c.Status(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		// Local development - handle CORS ourselves
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
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

		// Serve the file
		c.File(filepath)
	})

	// Add OPTIONS handler for download endpoint
	r.OPTIONS("/download/:filename", func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
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

		// Auth routes using new controllers
		api.POST("/auth/register", authController.Register)
		api.POST("/auth/login", authController.Login)
		api.POST("/auth/google", authController.GoogleLogin)
		api.POST("/auth/logout", handlers.LogoutUser())
	}

	// Public routes (no auth required) - keep using handlers for now
	public := r.Group("/api")
	{
		public.POST("/experience/optimize", handlers.OptimizeExperience)
		public.POST("/resume/generate", handlers.GenerateResume)
		public.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		public.POST("/resume/generate-pdf-file", handlers.GeneratePDFResumeFromHTMLFile)
		public.POST("/resume/parse", handlers.ParseResume)
		public.POST("/ai/education", handlers.OptimizeEducation)
		public.POST("/ai/summary", handlers.OptimizeSummary)
	}

	// Protected routes (require auth)
	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())

	{
		// User routes using new controllers
		protected.GET("/user/profile", userController.GetProfile)
		protected.PUT("/user/profile", userController.UpdateProfile)
		protected.POST("/user/change-password", userController.ChangePassword)
		protected.POST("/user/save", userController.SaveUserData)
		protected.GET("/user/load", userController.LoadUserData)

		// Resume History routes using new controllers
		protected.GET("/resume/history", resumeController.GetHistory)
		protected.DELETE("/resume/history/:id", resumeController.DeleteHistory)
		protected.GET("/resume/download/:filename", resumeController.DownloadResume)
	}

	log.Println("Server starting on port 8081")
	if err := r.Run(":8081"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
