package config

import (
	"fmt"
	"os"
	"strconv"
)

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type AppConfig struct {
	Port        string
	Database    DatabaseConfig
	JWTSecret   string
	Environment string
}

func GetDatabaseConfig() DatabaseConfig {
	port, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	password := getEnv("DB_PASSWORD", "")

	if password == "" {
		fmt.Println("⚠️  Warning: DB_PASSWORD environment variable is not set.")
		fmt.Println("   Please run the development script: .\\dev-run.ps1")
		fmt.Println("   Or set environment variables manually:")
		fmt.Println("   $env:DB_PASSWORD='your_password'; go run .\\main.go")
	}

	return DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     port,
		User:     getEnv("DB_USER", "postgres"),
		Password: password,
		DBName:   getEnv("DB_NAME", ""),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
}

func GetAppConfig() AppConfig {
	return AppConfig{
		Port:        getEnv("PORT", "8081"),
		Database:    GetDatabaseConfig(),
		JWTSecret:   getEnv("JWT_SECRET", "your-secret-key"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
