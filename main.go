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

	// CORS is already handled by the cors.New() middleware above
	// This custom middleware was causing issues and is redundant

	r.Static("/static", "./static")

	// Add OPTIONS handler at the very beginning, before any middleware
	r.OPTIONS("/api/experience/optimize", func(c *gin.Context) {
		fmt.Printf("🔧 Handling OPTIONS request for: %s\n", c.Request.URL.Path)
		c.Header("Access-Control-Allow-Origin", "https://www.hihired.org")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		fmt.Printf("✅ Set CORS headers for OPTIONS request\n")
		c.Status(200)
	})

	r.POST("/api/auth/register", handlers.RegisterUser(db))
	r.POST("/api/auth/login", handlers.LoginUser(db))
	r.POST("/api/auth/logout", handlers.LogoutUser())

	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())

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

	log.Println("Server starting on port 8081")
	r.Run(":8081")
}
