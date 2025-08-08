package main

import (
	"fmt"
	"log"
	"net/http"
	"resumeai/config"
	"resumeai/database"
	"resumeai/handlers"

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

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://hihired.org", "https://www.hihired.org", "http://localhost:3000", "http://127.0.0.1:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-Forwarded-Host", "X-Forwarded-Port"},
		ExposeHeaders:    []string{"Content-Length"},
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
			c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Forwarded-Host,X-Forwarded-Port")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// CORS is already handled by the cors.New() middleware above
	// This custom middleware was causing issues and is redundant

	r.Static("/static", "./static")

	// Generic OPTIONS handler to ensure preflight succeeds for all routes (including /api/*)
	r.OPTIONS("/*path", func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Requested-With,X-Forwarded-Host,X-Forwarded-Port")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		c.Status(http.StatusNoContent)
	})

	r.POST("/api/auth/register", handlers.RegisterUser(db))
	r.POST("/api/auth/login", handlers.LoginUser(db))
	r.POST("/api/auth/logout", handlers.LogoutUser())

	// Public routes (no auth required)
	public := r.Group("/api")
	{
		public.POST("/experience/optimize", handlers.OptimizeExperience)
		public.POST("/resume/generate", handlers.GenerateResume)
		public.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		public.POST("/resume/parse", handlers.ParseResume)
		public.POST("/ai/education", handlers.OptimizeEducation)
		public.POST("/ai/summary", handlers.OptimizeSummary)
	}

	// Serve static files without /api prefix
	r.Static("/static", "./static")

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
	r.Run(":8081")
}
