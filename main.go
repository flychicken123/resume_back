package main

import (
	"fmt"
	"log"
	"resumeai/config"
	"resumeai/database"
	"resumeai/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Environment variables are passed by Docker Compose
	// No need to load .env file since variables are set by container environment

	// Initialize database
	dbConfig := config.GetDatabaseConfig()

	// Debug: Print database configuration
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

	// Add security middleware to only allow requests from our domain
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		referer := c.GetHeader("Referer")

		// Log request for debugging
		fmt.Printf("Request: %s %s\n", c.Request.Method, c.Request.URL.Path)
		fmt.Printf("Origin: %s\n", origin)
		fmt.Printf("Referer: %s\n", referer)

		// Only allow requests from our domain
		allowedDomains := []string{"https://hihired.org", "https://www.hihired.org"}
		isAllowed := false

		for _, domain := range allowedDomains {
			if origin == domain || (referer != "" && referer[:len(domain)] == domain) {
				isAllowed = true
				break
			}
		}

		// For local development, allow localhost requests
		if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
			isAllowed = true
		}

		if !isAllowed {
			fmt.Printf("❌ Blocked request from unauthorized origin: %s\n", origin)
			c.JSON(403, gin.H{"error": "Unauthorized origin"})
			c.Abort()
			return
		}

		c.Next()
	})

	// Configure CORS to only allow requests from our own frontend
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://hihired.org", "https://www.hihired.org"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Forwarded-Host", "X-Forwarded-Port"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60, // 12 hours
	}))

	r.Static("/static", "./static")

	// Public endpoints
	r.POST("/api/auth/register", handlers.RegisterUser(db))
	r.POST("/api/auth/login", handlers.LoginUser(db))
	r.POST("/api/auth/logout", handlers.LogoutUser())

	// Protected endpoints (require authentication and API key)
	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())
	protected.Use(func(c *gin.Context) {
		// Check for custom API key header
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "hihired-secure-api-2024" {
			fmt.Printf("❌ Invalid API key from: %s\n", c.GetHeader("Origin"))
			c.JSON(401, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}
		c.Next()
	})
	{
		// User profile management
		protected.GET("/user/profile", handlers.GetUserProfile(db))
		protected.PUT("/user/profile", handlers.UpdateUserProfile(db))
		protected.POST("/user/change-password", handlers.ChangePassword(db))

		// Resume generation endpoints
		protected.POST("/resume/generate", handlers.GenerateResume)
		protected.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		protected.POST("/resume/parse", handlers.ParseResume)
		protected.POST("/experience/optimize", handlers.OptimizeExperience)

		// User data management
		protected.POST("/user/save", handlers.SaveUserData(db))
		protected.GET("/user/load", handlers.LoadUserData(db))
	}

	log.Println("Server starting on port 8081")
	r.Run(":8081")
}
