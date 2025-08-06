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

	// Add CORS debugging middleware
	r.Use(func(c *gin.Context) {
		fmt.Printf("Request: %s %s\n", c.Request.Method, c.Request.URL.Path)
		fmt.Printf("Origin: %s\n", c.GetHeader("Origin"))
		fmt.Printf("Access-Control-Request-Method: %s\n", c.GetHeader("Access-Control-Request-Method"))
		fmt.Printf("Access-Control-Request-Headers: %s\n", c.GetHeader("Access-Control-Request-Headers"))
		c.Next()
	})

	// Configure CORS to allow authentication headers
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://3.142.69.155:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60, // 12 hours
	}))

	r.Static("/static", "./static")

	// Public endpoints
	r.POST("/api/auth/register", handlers.RegisterUser(db))
	r.POST("/api/auth/login", handlers.LoginUser(db))
	r.POST("/api/auth/logout", handlers.LogoutUser())

	// Protected endpoints (require authentication)
	protected := r.Group("/api")
	protected.Use(handlers.AuthMiddleware())
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
