package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultTimeZone = "America/Los_Angeles"

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type AppConfig struct {
	Port                  string
	Database              DatabaseConfig
	JWTSecret             string
	Environment           string
	TimeZone              string
	GeminiRetryEnabled    bool
	ClassifyPerJobDelayMS int
	ClassifyBatchSize     int
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
		Port:                  getEnv("PORT", "8081"),
		Database:              GetDatabaseConfig(),
		JWTSecret:             getEnv("JWT_SECRET", "your-secret-key"),
		Environment:           getEnv("ENVIRONMENT", "development"),
		TimeZone:              getEnv("APP_TIMEZONE", defaultTimeZone),
		GeminiRetryEnabled:    getEnvBool("GEMINI_RETRY_ENABLED", true),
		ClassifyPerJobDelayMS: getEnvPositiveInt("CLASSIFY_PER_JOB_DELAY_MS", 500),
		ClassifyBatchSize:     getEnvPositiveInt("CLASSIFY_BATCH_SIZE", 5),
	}
}

func getEnvBool(key string, defaultValue bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "":
		return defaultValue
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultValue
	}
}

func getEnvPositiveInt(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultValue
	}
	return v
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
