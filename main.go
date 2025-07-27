package main

import (
	"log"
	"resumeai/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	r := gin.Default()
	r.Use(cors.Default())

	r.Static("/static", "./static")

	r.POST("/api/resume/generate", handlers.GenerateResume)
	r.POST("/api/resume/parse", handlers.ParseResume)
	r.Run(":8081")
}
