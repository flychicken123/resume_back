package main

import (
	"fmt"
	"log"
	"resumeai/config"
	"resumeai/database"
	"resumeai/handlers"
	"strings"

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
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
	}))

	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		referer := c.GetHeader("Referer")

		fmt.Printf("Request: %s %s\n", c.Request.Method, c.Request.URL.Path)
		fmt.Printf("Origin: %s\n", origin)
		fmt.Printf("Referer: %s\n", referer)

		// Allow health checks and internal requests
		if origin == "" && (strings.Contains(c.Request.RemoteAddr, "127.0.0.1") || strings.Contains(c.Request.RemoteAddr, "::1")) {
			fmt.Printf("✅ Allowing internal request from: %s\n", c.Request.RemoteAddr)
			c.Next()
			return
		}

		allowedDomains := []string{"https://hihired.org", "https://www.hihired.org"}
		isAllowed := false

		for _, domain := range allowedDomains {
			if origin == domain || (referer != "" && referer[:len(domain)] == domain) {
				isAllowed = true
				break
			}
		}

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

	r.Static("/static", "./static")

	r.POST("/api/auth/register", handlers.RegisterUser(db))
	r.POST("/api/auth/login", handlers.LoginUser(db))
	r.POST("/api/auth/logout", handlers.LogoutUser())

	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())
	protected.Use(func(c *gin.Context) {
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
		protected.GET("/user/profile", handlers.GetUserProfile(db))
		protected.PUT("/user/profile", handlers.UpdateUserProfile(db))
		protected.POST("/user/change-password", handlers.ChangePassword(db))

		protected.POST("/resume/generate", handlers.GenerateResume)
		protected.POST("/resume/generate-pdf", handlers.GeneratePDFResume)
		protected.POST("/resume/parse", handlers.ParseResume)

		protected.POST("/user/save", handlers.SaveUserData(db))
		protected.GET("/user/load", handlers.LoadUserData(db))
		protected.POST("/experience/optimize", handlers.OptimizeExperience)
	}

	// Add OPTIONS handler outside protected group
	r.OPTIONS("/api/experience/optimize", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "https://www.hihired.org")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		c.Status(200)
	})

	log.Println("Server starting on port 8081")
	r.Run(":8081")
}
