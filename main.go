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

	ctx, cancel := context.WithCancel(context.Background())
	embeddingSvc, embErr := services.NewEmbeddingService(os.Getenv("GEMINI_API_KEY"))
	if embErr != nil {
		logger.Warn("embedding service unavailable", map[string]interface{}{"error": embErr.Error()})
		embeddingSvc = nil
	}

	dbConfig := appConfig.Database

	fmt.Println("[BUILD] version=2026-02-14-v2 temperature=0 parse")

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
	defer func() {
		cancel()
		if embeddingSvc != nil {
			embeddingSvc.Close()
		}
	}()

	log.Println("Database connection successful!")

	// Run database migrations on startup
	// migrationsPath := "./database/migrations"
	// if err := database.RunMigrations(db, migrationsPath); err != nil {
	// 	log.Printf("Warning: Database migration error: %v", err)
	// 	// Don't fatal - allow app to start even if migrations have issues
	// 	// This prevents deployment failures when migrations were already applied
	// }

	// Initialize models
	userModel := models.NewUserModel(db)
	resumeHistoryModel := models.NewResumeHistoryModel(db)
	resumeModel := models.NewResumeModel(db)
	handlers.SetResumeProfileStore(resumeModel)
	projectModel := models.NewProjectModel(db)
	if err := models.EnsureFeedbackSchema(db); err != nil {
		log.Fatal("Error ensuring feedback tables:", err)
	}
	feedbackModel := models.NewFeedbackModel(db)
	chatHistoryModel := models.NewChatHistoryModel(db)

	if err := models.EnsureExperimentSchema(db); err != nil {
		log.Fatal("Error ensuring experiment tables:", err)
	}
	jobCompanyModel := models.NewJobCompanyModel(db)
	jobPostingModel := models.NewJobPostingModel(db)
	jobSyncModel := models.NewJobSyncRunModel(db)
	jobMatchModel := models.NewResumeJobMatchModel(db)
	jobAppModel := models.NewJobApplicationModel(db)
	experimentModel := models.NewExperimentModel(db)

	// Initialize services
	jwtService := services.NewJWTService(appConfig.JWTSecret)
	s3Service, err := services.NewS3Service()
	if err != nil {
		log.Fatal("Error initializing S3 service:", err)
	}
	resumeService := services.NewResumeService(resumeHistoryModel, s3Service)
	stripeService := services.NewStripeService(db)
	adService := services.NewAdService(db)
	emailService := services.NewEmailService(logger)
	jobsService := services.NewJobIngestionService(db, logger, embeddingSvc, ctx)
	experimentService := services.NewExperimentService(experimentModel)

	// Initialize the LangChain-backed copilot agent once and share it between
	// the job matcher (for re-ranking) and the copilot handlers.
	var copilotAgent *services.CopilotAgent
	var experienceAgent *services.ExperienceAgent
	if agent, err := services.NewCopilotAgent(); err != nil {
		log.Printf("Warning: Copilot agent disabled: %v", err)
	} else {
		copilotAgent = agent
		handlers.SetCopilotAgent(agent)
	}

	jobMatcherService := services.NewJobMatcherService(jobPostingModel, jobMatchModel, logger, copilotAgent, embeddingSvc, ctx)
	handlers.SetResumeJobMatcherService(jobMatcherService)
	handlers.SetChatHistoryModel(chatHistoryModel)
	handlers.SetChatModels(userModel, jobAppModel)
	handlers.SetJobMatchModel(jobMatchModel)

	// RAG knowledge base
	knowledgeModel := models.NewKnowledgeBaseModel(db)
	knowledgeSvc := services.NewKnowledgeService(knowledgeModel, embeddingSvc, logger)
	knowledgeCtrl := controllers.NewKnowledgeController(knowledgeSvc)
	handlers.SetKnowledgeService(knowledgeSvc)
	chatProfileModel := models.NewUserChatProfileModel(db)
	handlers.SetChatProfileModel(chatProfileModel)
	handlers.SetChatJobMatchModel(jobMatchModel)
	handlers.SetChatResumeHistoryModel(resumeHistoryModel)
	handlers.SetChatDB(db)
	services.SetToolRegistry(&services.ToolRegistry{
		JobPostingModel:  jobPostingModel,
		JobAppModel:      jobAppModel,
		JobMatchModel:    jobMatchModel,
		ChatProfileModel: chatProfileModel,
		DB:               db,
	})
	go knowledgeSvc.SeedIfEmpty(ctx)
	dismissedJobMatchModel := models.NewDismissedJobMatchModel(db)
	resumeSkillCacheModel := models.NewResumeSkillCacheModel(db)
	resumeEmbeddingCacheModel := models.NewResumeEmbeddingCacheModel(db)
	services.SetSkillInferenceDBCache(resumeSkillCacheModel)
	services.SetJobMatcherEmbeddingDBCache(resumeEmbeddingCacheModel)
	jobsController := controllers.NewJobsController(jobCompanyModel, jobPostingModel, jobSyncModel, jobMatchModel, dismissedJobMatchModel, jobMatcherService, jobsService)
	jobAppController := controllers.NewJobApplicationController(jobAppModel, jobPostingModel, jobMatchModel)
	geoService := services.NewGeoService(time.Now().UTC())
	if jobIntentAgent, err := services.NewJobIntentAgent(); err != nil {
		log.Printf("Warning: Job intent agent disabled: %v", err)
	} else {
		handlers.SetJobIntentAgent(jobIntentAgent)
	}
	if personalAgent, err := services.NewPersonalInfoAgent(); err != nil {
		log.Printf("Warning: Personal info agent disabled: %v", err)
	} else {
		handlers.SetPersonalInfoAgent(personalAgent)
	}
	if agent, err := services.NewExperienceAgent(); err != nil {
		log.Printf("Warning: Experience agent disabled: %v", err)
	} else {
		experienceAgent = agent
		handlers.SetExperienceAgent(experienceAgent)
	}
	if templatePreferenceAgent, err := services.NewTemplatePreferenceAgent(); err != nil {
		log.Printf("Warning: Template preference agent disabled: %v", err)
	} else {
		handlers.SetTemplatePreferenceAgent(templatePreferenceAgent)
	}
	if projectsAgent, err := services.NewProjectsAgent(); err != nil {
		log.Printf("Warning: Projects agent disabled: %v", err)
	} else {
		handlers.SetProjectsAgent(projectsAgent)
	}
	if educationAgent, err := services.NewEducationAgent(); err != nil {
		log.Printf("Warning: Education agent disabled: %v", err)
	} else {
		handlers.SetEducationAgent(educationAgent)
	}
	if jobDescriptionAgent, err := services.NewJobDescriptionAgent(); err != nil {
		log.Printf("Warning: Job description agent disabled: %v", err)
	} else {
		handlers.SetJobDescriptionAgent(jobDescriptionAgent)
	}
	if skillsAgent, err := services.NewSkillsAgent(); err != nil {
		log.Printf("Warning: Skills agent disabled: %v", err)
	} else {
		handlers.SetSkillsAgent(skillsAgent)
	}
	if voiceAgent, err := services.NewVoiceAgent(); err != nil {
		log.Printf("Warning: Voice agent disabled: %v", err)
	} else {
		handlers.SetVoiceAgent(voiceAgent)
	}
	if resumeModifyAgent, err := services.NewResumeModifyAgent(); err != nil {
		log.Printf("Warning: Resume modify agent disabled: %v", err)
	} else {
		handlers.SetResumeModifyAgent(resumeModifyAgent)
	}
	if polishAgent, err := services.NewPolishAgent(); err != nil {
		log.Printf("Warning: Polish agent disabled: %v", err)
	} else {
		handlers.SetPolishAgent(polishAgent)
	}
	if impactKeywordsAgent, err := services.NewImpactKeywordsAgent(); err != nil {
		log.Printf("Warning: Impact keywords agent disabled: %v", err)
	} else {
		handlers.SetImpactKeywordsAgent(impactKeywordsAgent)
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
	experimentController := controllers.NewExperimentController(experimentService)
	adsController := controllers.NewAdsController(db, adService, stripeService)

	autofillController := controllers.NewAutofillController(userModel, resumeModel)

	jobsService.StartScheduler(ctx, 24*time.Hour)
	jobMatchNotifier := services.NewJobMatchNotifier(resumeModel, jobMatcherService, emailService, logger)
	resumeBackfill := services.NewResumeProfileBackfillService(db, resumeModel, s3Service, logger)
	jobEmbeddingBackfill := services.NewJobEmbeddingBackfillService(jobPostingModel, embeddingSvc, logger)
	jobEmbeddingCtrl := controllers.NewJobEmbeddingController(jobEmbeddingBackfill, ctx)
	jobClassifyBackfill := services.NewJobClassifyBackfillService(jobPostingModel, logger)
	jobClassifyCtrl := controllers.NewJobClassifyController(jobClassifyBackfill, ctx)
	benchmarkModel := models.NewAiBenchmarkModel(db)
	benchmarkSvc := services.NewBenchmarkService(jobPostingModel, jobMatchModel, benchmarkModel, logger)
	benchmarkCtrl := controllers.NewBenchmarkController(benchmarkSvc, ctx)
	benchmarkSvc.StartScheduler(ctx)
	jobMatchNotifier.Start(ctx, 100*time.Hour)

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
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, Content-Length, X-Experiment-User")
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
		internal := api.Group("/internal")
		internal.Use(middleware.RequireInternalToken("JOB_MATCH_NOTIFIER_TRIGGER_TOKEN"))
		{
			internal.POST("/job-match-notifier/run", func(c *gin.Context) {
				var req struct {
					UserID        int    `json:"user_id"`
					EmailOverride string `json:"email_override"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.UserID <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
					return
				}

				result, err := jobMatchNotifier.RunOnceForUser(c.Request.Context(), req.UserID, req.EmailOverride)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"result": result})
			})

			internal.POST("/job-match-notifier/run-all", func(c *gin.Context) {
				var req struct {
					Confirm       string `json:"confirm"`
					Limit         int    `json:"limit"`
					EmailOverride string `json:"email_override"`
					SendEmail     bool   `json:"send_email"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.Confirm != "RUN_ALL_USERS" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "confirm must be RUN_ALL_USERS"})
					return
				}

				status, err := jobMatchNotifier.TriggerRunAll(req.Limit, req.EmailOverride, req.SendEmail)
				if err != nil {
					c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusAccepted, gin.H{"status": status})
			})

			internal.GET("/job-match-notifier/run-all/status", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": jobMatchNotifier.RunAllStatus()})
			})

			internal.POST("/resumes/backfill-from-history", func(c *gin.Context) {
				var req services.ResumeProfileBackfillRequest
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.Confirm != "BACKFILL_RESUME_PROFILES" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "confirm must be BACKFILL_RESUME_PROFILES"})
					return
				}
				status, err := resumeBackfill.Trigger(req.Limit, req.DryRun)
				if err != nil {
					c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusAccepted, gin.H{"status": status})
			})

			internal.GET("/resumes/backfill-from-history/status", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": resumeBackfill.Status()})
			})
		}

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
		api.POST("/experiments/assign", experimentController.AssignVariant)
		api.POST("/experiments/event", experimentController.TrackEvent)
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
		public.POST("/skills/auto-generate", handlers.AutoGenerateSkills)
		public.POST("/skills/categorize", handlers.CategorizeSkills)

		// Resume parsing (no limit needed)
		public.POST("/resume/parse", handlers.ParseResume)

		// Job extraction endpoint
		public.POST("/job/extract", handlers.ImprovedExtractJobDescription)
		public.POST("/assistant/chat", handlers.ChatAssistant)
		public.POST("/assistant/personal-info", handlers.ParsePersonalInfo)
		public.POST("/assistant/job-intent", handlers.ParseJobIntent)
		public.POST("/assistant/experience", handlers.ParseExperience)
		public.POST("/assistant/projects", handlers.ParseProjects)
		public.POST("/assistant/education", handlers.ParseEducation)
		public.POST("/assistant/job-description", handlers.ParseJobDescription)
		public.POST("/assistant/skills", handlers.ParseSkillsAI)
		public.POST("/assistant/skills/generate", handlers.GenerateSkillsAI)
		public.POST("/assistant/voice/transcribe", handlers.TranscribeAudio)
		public.POST("/assistant/resume/modify", handlers.AnalyzeResumeModification)
		public.POST("/assistant/resume/polish", handlers.PolishResume)
		public.POST("/template/preference", handlers.InferTemplatePreference)
		public.POST("/impact-keywords/extract", handlers.ExtractImpactKeywords)
		public.POST("/analytics/exit", handlers.TrackExitEvent(db))
		public.POST("/feedback", handlers.SubmitFeedback(feedbackModel))
		public.POST("/feedback/follow-up", handlers.ScheduleFeedbackFollowUp(feedbackModel))
		public.POST("/contact", handlers.CreateContactRequest(db, emailService))
		public.GET("/email/unsubscribe", handlers.HandleUnsubscribeGET(userModel))
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
		protected.GET("/user/job-preferences", userController.GetJobPreferences)
		protected.PUT("/user/job-preferences", userController.SaveJobPreferences)

		// Job profile routes (backed by job_preferences, used by website JobProfileSetup)
		protected.GET("/job/profile", userController.GetJobProfile)
		protected.POST("/job/profile", userController.SaveJobProfile)
		protected.GET("/user/followup-reminders", userController.GetFollowupReminders)
		protected.PUT("/user/followup-reminders", userController.UpdateFollowupReminders)

		// Resume History routes using new controllers
		protected.GET("/resume/history", resumeController.GetHistory)
		protected.DELETE("/resume/history/:id", resumeController.DeleteHistory)
		protected.PUT("/resume/history/:id/rename", resumeController.RenameResume)
		protected.POST("/resume/history/upload", resumeController.UploadHistoryPDF)
		protected.GET("/resume/download/:filename", resumeController.DownloadResume)
		protected.GET("/resume/latest-pdf-url", resumeController.GetLatestPDFURL)
		protected.POST("/resume/copilot", handlers.ResumeCopilot)
		protected.POST("/jobs/explain-fit", handlers.ExplainJobFit)

		// Project routes
		protected.GET("/projects/resume/:resumeId", projectController.GetProjectsByResumeID)
		protected.POST("/projects", projectController.CreateProject)
		protected.PUT("/projects/:id", projectController.UpdateProject)
		protected.DELETE("/projects/:id", projectController.DeleteProject)

		protected.POST("/jobs/matches", jobsController.ComputeMatches)
		protected.GET("/jobs/matches", jobsController.ListMatchedJobs)
		protected.POST("/jobs/matches/dismiss", jobsController.DismissMatch)
		protected.POST("/jobs/matches/undismiss", jobsController.UndismissMatch)
		protected.GET("/jobs/:id", jobsController.GetJobByID)

		// Job application tracking routes
		protected.POST("/job/apply", jobAppController.SubmitApplication)
		protected.GET("/job/applications", jobAppController.ListApplications)
		protected.GET("/job/applications/stats", jobAppController.GetStats)
		protected.GET("/job/applications/:id", jobAppController.GetApplication)
		protected.PUT("/job/applications/:id/status", jobAppController.UpdateStatus)
		protected.DELETE("/job/applications/:id", jobAppController.DeleteApplication)
		protected.PUT("/job/applications/:id/reminders", jobAppController.UpdateReminders)
		protected.PUT("/job/applications/:id/notes", jobAppController.UpdateNotes)

		// Subscription routes
		protected.GET("/subscription/current", subscriptionController.GetCurrentSubscription)
		protected.GET("/subscription/usage", subscriptionController.GetUsageStats)
		protected.POST("/subscription/checkout", subscriptionController.CreateCheckoutSession)
		protected.POST("/subscription/cancel", subscriptionController.CancelSubscription)
		protected.POST("/subscription/change-plan", subscriptionController.ChangePlan)
		protected.POST("/subscription/portal", subscriptionController.CreateCustomerPortal)
		protected.GET("/subscription/check-limit", subscriptionController.CheckResumeLimit)
		protected.POST("/subscription/confirm", subscriptionController.ConfirmSuccess)

		// Ads rewards routes
		protected.GET("/ads/status", adsController.GetAdStatus)
		protected.POST("/ads/watch-complete", adsController.AdWatchComplete)

		admin := protected.Group("/admin")
		admin.Use(middleware.RequireAdmin())
		{
			admin.GET("/memberships/users", adminController.ListUsers)
			admin.PUT("/memberships/users/:id", adminController.UpdateUserMembership)
			admin.GET("/users/emails", adminController.ExportUserEmails)
			admin.POST("/job-match-notifier/run", func(c *gin.Context) {
				var req struct {
					UserID        int    `json:"user_id"`
					EmailOverride string `json:"email_override"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.UserID <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
					return
				}

				result, err := jobMatchNotifier.RunOnceForUser(c.Request.Context(), req.UserID, req.EmailOverride)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"result": result})
			})

			admin.POST("/job-match-notifier/run-all", func(c *gin.Context) {
				var req struct {
					Confirm       string `json:"confirm"`
					Limit         int    `json:"limit"`
					EmailOverride string `json:"email_override"`
					SendEmail     bool   `json:"send_email"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.Confirm != "RUN_ALL_USERS" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "confirm must be RUN_ALL_USERS"})
					return
				}

				status, err := jobMatchNotifier.TriggerRunAll(req.Limit, req.EmailOverride, req.SendEmail)
				if err != nil {
					c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusAccepted, gin.H{"status": status})
			})

			admin.GET("/job-match-notifier/run-all/status", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": jobMatchNotifier.RunAllStatus()})
			})

			admin.POST("/resumes/backfill-from-history", func(c *gin.Context) {
				var req services.ResumeProfileBackfillRequest
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
					return
				}
				if req.Confirm != "BACKFILL_RESUME_PROFILES" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "confirm must be BACKFILL_RESUME_PROFILES"})
					return
				}
				status, err := resumeBackfill.Trigger(req.Limit, req.DryRun)
				if err != nil {
					c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusAccepted, gin.H{"status": status})
			})

			admin.GET("/resumes/backfill-from-history/status", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": resumeBackfill.Status()})
			})
			admin.GET("/jobs/companies", jobsController.ListCompanies)
			admin.GET("/jobs/companies/ats", jobsController.ExportCompanyATS)
			admin.POST("/jobs/companies", jobsController.CreateCompany)
			admin.POST("/jobs/companies/import", jobsController.ImportCompanies)
			admin.POST("/jobs/companies/sync-all", jobsController.TriggerSyncAll)
			admin.POST("/jobs/companies/:id/sync", jobsController.TriggerSync)
			admin.PATCH("/jobs/companies/:id/status", jobsController.UpdateCompanyStatus)

			// Admin job postings management
			admin.GET("/jobs/postings", jobsController.AdminListPostings)
			admin.GET("/jobs/postings/:id", jobsController.AdminGetPosting)
			admin.PUT("/jobs/postings/:id", jobsController.AdminUpdatePosting)
			admin.DELETE("/jobs/postings/:id", jobsController.AdminDeletePosting)
			admin.POST("/jobs/postings/bulk-update", jobsController.AdminBulkUpdatePostings)
			admin.GET("/jobs/stats", jobsController.AdminGetJobStats)
			admin.GET("/jobs/sync-runs", jobsController.AdminListSyncRuns)

			admin.GET("/analytics/exit-summary", adminController.GetExitSummary)
			admin.GET("/experiments", experimentController.ListExperiments)
			admin.POST("/experiments", experimentController.CreateOrUpdateExperiment)
			admin.GET("/experiments/:key/metrics", experimentController.GetExperimentMetrics)
			admin.DELETE("/experiments/:key", experimentController.DeleteExperiment)
			admin.POST("/jobs/embeddings/backfill", jobEmbeddingCtrl.StartBackfill)
			admin.GET("/jobs/embeddings/backfill/status", jobEmbeddingCtrl.GetStatus)
			admin.DELETE("/jobs/embeddings/backfill", jobEmbeddingCtrl.StopBackfill)

			admin.POST("/jobs/classify/backfill", jobClassifyCtrl.StartBackfill)
			admin.GET("/jobs/classify/backfill/status", jobClassifyCtrl.GetStatus)
			admin.DELETE("/jobs/classify/backfill", jobClassifyCtrl.StopBackfill)

			admin.POST("/benchmark/run", benchmarkCtrl.RunBenchmark)
			admin.GET("/benchmark/status", benchmarkCtrl.GetStatus)
			admin.GET("/benchmark/results", benchmarkCtrl.GetResults)
			admin.GET("/benchmark/history", benchmarkCtrl.GetHistory)
			admin.GET("/benchmark/summary", benchmarkCtrl.GetSummary)

			// Knowledge base management
			admin.GET("/knowledge", knowledgeCtrl.List)
			admin.POST("/knowledge", knowledgeCtrl.Create)
			admin.PUT("/knowledge/:id", knowledgeCtrl.Update)
			admin.DELETE("/knowledge/:id", knowledgeCtrl.Delete)
			admin.POST("/knowledge/backfill", knowledgeCtrl.BackfillEmbeddings)
		}
	}

	// Public subscription routes
	api.GET("/plans", subscriptionController.GetPlans)
	api.POST("/webhook/stripe", subscriptionController.HandleStripeWebhook)
	api.POST("/stripe/webhook", subscriptionController.HandleStripeWebhook)

	// Autofill session routes
	api.GET("/autofill/session/:id", autofillController.GetSession)
	api.OPTIONS("/autofill/session/:id", autofillController.GetSessionOptions)
	protected.POST("/autofill/session", autofillController.CreateSession)

	// Apply subscription limit middleware to resume generation endpoints
	resumeGenRoutes := r.Group("/api")
	resumeGenRoutes.Use(handlers.AuthMiddleware()) // Auth is required to track usage
	resumeGenRoutes.Use(middleware.CheckResumeLimitMiddleware(db))
	{
		resumeGenRoutes.POST("/resume/generate", handlers.GenerateResume)
		resumeGenRoutes.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		resumeGenRoutes.POST("/resume/generate-pdf-file", handlers.GeneratePDFResumeHandler(db, resumeHistoryModel, userModel))
		resumeGenRoutes.POST("/resume/tailor", handlers.TailorResume(resumeModel, resumeHistoryModel, projectModel))
	}

	log.Println("Server starting on port 8081")
	if err := r.Run(":8081"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
