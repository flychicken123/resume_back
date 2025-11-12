package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"resumeai/config"
	"resumeai/controllers"
	"resumeai/database"
	"resumeai/handlers"
	"resumeai/middleware"
	"resumeai/models"
	"resumeai/services"
	"resumeai/utils"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	appConfig := config.GetAppConfig()

	loc, err := time.LoadLocation(appConfig.TimeZone)
	if err != nil {
		log.Fatalf("invalid APP_TIMEZONE %q: %v", appConfig.TimeZone, err)
	}
	time.Local = loc

	logger := utils.NewLogger()

	dbConfig := appConfig.Database

	logger.Info("Starting application", map[string]interface{}{
		"db_host":   dbConfig.Host,
		"db_port":   dbConfig.Port,
		"db_name":   dbConfig.DBName,
		"time_zone": appConfig.TimeZone,
	})

	db, err := database.Connect(
		dbConfig.Host,
		fmt.Sprintf("%d", dbConfig.Port),
		dbConfig.User,
		dbConfig.Password,
		dbConfig.DBName,
		dbConfig.SSLMode,
		appConfig.TimeZone,
	)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}
	defer db.Close()

	log.Println("? Database connection successful!")

	// Initialize models
	userModel := models.NewUserModel(db)
	resumeHistoryModel := models.NewResumeHistoryModel(db)
	resumeModel := models.NewResumeModel(db)
	projectModel := models.NewProjectModel(db)
	if err := models.EnsureFeedbackSchema(db); err != nil {
		log.Fatal("Error ensuring feedback tables:", err)
	}
	feedbackModel := models.NewFeedbackModel(db)
	chatHistoryModel := models.NewChatHistoryModel(db)

	jobCompanyModel := models.NewJobCompanyModel(db)
	jobPostingModel := models.NewJobPostingModel(db)
	jobSyncModel := models.NewJobSyncRunModel(db)
	jobMatchModel := models.NewResumeJobMatchModel(db)

	// Initialize services
	jwtService := services.NewJWTService(appConfig.JWTSecret)
	s3Service, err := services.NewS3Service()
	if err != nil {
		log.Fatal("Error initializing S3 service:", err)
	}
	resumeService := services.NewResumeService(resumeHistoryModel, s3Service)
	stripeService := services.NewStripeService(db)
	emailService := services.NewEmailService()
	jobsService := services.NewJobIngestionService(db, logger)
	jobMatcherService := services.NewJobMatcherService(jobPostingModel, jobMatchModel, logger)
	handlers.SetResumeJobMatcherService(jobMatcherService)
	handlers.SetChatHistoryModel(chatHistoryModel)
	jobsController := controllers.NewJobsController(jobCompanyModel, jobPostingModel, jobSyncModel, jobMatchModel, jobMatcherService, jobsService)
	geoService := services.NewGeoService(time.Now().UTC())
	if copilotAgent, err := services.NewCopilotAgent(); err != nil {
		log.Printf("Warning: Copilot agent disabled: %v", err)
	} else {
		handlers.SetCopilotAgent(copilotAgent)
	}

	// Initialize Stripe products (only run this once or on startup)
	if err := stripeService.CreateOrUpdateStripeProducts(); err != nil {
		log.Printf("Warning: Could not initialize Stripe products: %v", err)
	}

	// Initialize controllers
	authController := controllers.NewAuthController(userModel, jwtService)
	resumeController := controllers.NewResumeController(resumeHistoryModel, resumeService, s3Service)
	userController := controllers.NewUserController(userModel, resumeModel)
	projectController := controllers.NewProjectController(projectModel)
	subscriptionController := controllers.NewSubscriptionController(db, stripeService)
	adminController := controllers.NewAdminController(db)
	geoController := controllers.NewGeoController(geoService)
	dataAnalysisController := controllers.NewDataAnalysisController(jobPostingModel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobsService.StartScheduler(ctx, 24*time.Hour)
	go func() {
		if err := jobsService.SyncAllCompanies(ctx); err != nil {
			logger.Warn("initial job sync failed", map[string]interface{}{"error": err.Error()})
		}
	}()

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Create rate limiters and caches
	rateLimiters := middleware.CreateRateLimiters()
	caches := middleware.CreateCaches()

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
		c.Status(http.StatusNoContent)
	})

	// API routes
	api := r.Group("/api")
	{
		// Health check endpoint for Docker Compose
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":    "healthy",
				"timestamp": time.Now().Unix(),
				"version":   "1.0.1",
				"uptime":    "ok",
			})
		})

		api.GET("/version", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"version":     "1.0.1",
				"build_time":  time.Now().Format("2006-01-02 15:04:05"),
				"pdf_margins": "zero_margins_v2",
			})
		})
		api.GET("/geo/answers", geoController.GetAnswers)

		// Auth routes using new controllers
		api.POST("/auth/register", authController.Register)
		api.POST("/auth/login", authController.Login)
		api.POST("/auth/google", authController.GoogleLogin)
		api.POST("/auth/logout", handlers.LogoutUser())
		api.GET("/jobs", jobsController.ListJobs)
	}

	// Public routes (no auth required) - keep using handlers for now
	public := r.Group("/api")
	// Add rate limiting and caching for AI endpoints
	public.Use(rateLimiters["ai"].Limit())
	public.Use(caches["ai"].Cache())
	{
		// Existing AI optimization endpoints
		public.POST("/experience/optimize", handlers.OptimizeExperience)
		public.POST("/ai/education", handlers.OptimizeEducation)
		public.POST("/ai/summary", handlers.OptimizeSummary)

		// New grammar improvement endpoints
		public.POST("/experience/improve-grammar", handlers.ImproveExperienceGrammar)
		public.POST("/summary/improve-grammar", handlers.ImproveSummaryGrammar)
		public.POST("/project/optimize", handlers.OptimizeProject)
		public.POST("/project/improve-grammar", handlers.ImproveProjectGrammar)

		// New final step AI endpoints
		public.POST("/resume/analyze-advice", handlers.AnalyzeResumeAdvice)
		public.POST("/cover-letter/generate", handlers.GenerateCoverLetter)

		// Resume parsing (no limit needed)
		public.POST("/resume/parse", handlers.ParseResume)

		// Job extraction endpoint
		public.POST("/job/extract", handlers.ImprovedExtractJobDescription)
		public.POST("/assistant/chat", handlers.ChatAssistant)
		public.POST("/analytics/exit", handlers.TrackExitEvent(db))
		public.POST("/feedback", handlers.SubmitFeedback(feedbackModel))
		public.POST("/feedback/follow-up", handlers.ScheduleFeedbackFollowUp(feedbackModel))
		public.POST("/contact", handlers.CreateContactRequest(db, emailService))
		public.GET("/analysis/job-count", dataAnalysisController.GetJobCount)

		// Job automation endpoints removed - feature in development
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
		protected.PUT("/resume/history/:id/rename", resumeController.RenameResume)
		protected.POST("/resume/history/upload", resumeController.UploadHistoryPDF)
		protected.GET("/resume/download/:filename", resumeController.DownloadResume)
		protected.POST("/resume/copilot", handlers.ResumeCopilot)

		// Project routes
		protected.GET("/projects/resume/:resumeId", projectController.GetProjectsByResumeID)
		protected.POST("/projects", projectController.CreateProject)
		protected.PUT("/projects/:id", projectController.UpdateProject)
		protected.DELETE("/projects/:id", projectController.DeleteProject)

		protected.POST("/jobs/matches", jobsController.ComputeMatches)
		protected.GET("/jobs/matches", jobsController.ListMatchedJobs)

		// Subscription routes
		protected.GET("/subscription/current", subscriptionController.GetCurrentSubscription)
		protected.GET("/subscription/usage", subscriptionController.GetUsageStats)
		protected.POST("/subscription/checkout", subscriptionController.CreateCheckoutSession)
		protected.POST("/subscription/cancel", subscriptionController.CancelSubscription)
		protected.POST("/subscription/change-plan", subscriptionController.ChangePlan)
		protected.POST("/subscription/portal", subscriptionController.CreateCustomerPortal)
		protected.GET("/subscription/check-limit", subscriptionController.CheckResumeLimit)
		protected.POST("/subscription/confirm", subscriptionController.ConfirmSuccess)

		admin := protected.Group("/admin")
		admin.Use(middleware.RequireAdmin())
		{
			admin.GET("/memberships/users", adminController.ListUsers)
			admin.PUT("/memberships/users/:id", adminController.UpdateUserMembership)
			admin.GET("/users/emails", adminController.ExportUserEmails)
			admin.GET("/jobs/companies", jobsController.ListCompanies)
			admin.POST("/jobs/companies", jobsController.CreateCompany)
			admin.POST("/jobs/companies/import", jobsController.ImportCompanies)
			admin.POST("/jobs/companies/sync-all", jobsController.TriggerSyncAll)
			admin.POST("/jobs/companies/:id/sync", jobsController.TriggerSync)
			admin.PATCH("/jobs/companies/:id/status", jobsController.UpdateCompanyStatus)
			admin.GET("/analytics/exit-summary", adminController.GetExitSummary)
		}
	}

	// Public subscription routes
	api.GET("/plans", subscriptionController.GetPlans)
	api.POST("/webhook/stripe", subscriptionController.HandleStripeWebhook)
	api.POST("/stripe/webhook", subscriptionController.HandleStripeWebhook)

	// Apply subscription limit middleware to resume generation endpoints
	resumeGenRoutes := r.Group("/api")
	resumeGenRoutes.Use(handlers.AuthMiddleware()) // Auth is required to track usage
	resumeGenRoutes.Use(middleware.CheckResumeLimitMiddleware(db))
	{
		resumeGenRoutes.POST("/resume/generate", handlers.GenerateResume)
		resumeGenRoutes.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		resumeGenRoutes.POST("/resume/generate-pdf-file", handlers.GeneratePDFResumeHandler(db, resumeHistoryModel, userModel))
	}

	log.Println("Server starting on port 8081")
	if err := r.Run(":8081"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
