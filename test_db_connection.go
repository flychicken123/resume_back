package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load environment variables
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
		log.Println("Continuing with default configuration...")
	}

	// Get database configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "resumeai")

	fmt.Printf("Testing database connection with:\n")
	fmt.Printf("Host: %s\n", dbHost)
	fmt.Printf("Port: %s\n", dbPort)
	fmt.Printf("User: %s\n", dbUser)
	fmt.Printf("Database: %s\n", dbName)
	fmt.Printf("Password: %s\n", maskPassword(dbPassword))

	// Build connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	fmt.Printf("\nConnection string: %s\n", maskConnectionString(psqlInfo))

	// Open database connection
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}

	fmt.Println("\n✅ Database connection successful!")

	// Test if users table exists
	var tableExists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')").Scan(&tableExists)
	if err != nil {
		log.Printf("Warning: Could not check if users table exists: %v", err)
	} else {
		if tableExists {
			fmt.Println("✅ Users table exists")
		} else {
			fmt.Println("❌ Users table does not exist")
		}
	}

	// Test creating a temporary table
	_, err = db.Exec("CREATE TEMP TABLE test_table (id int)")
	if err != nil {
		fmt.Printf("❌ Cannot create tables: %v\n", err)
		fmt.Println("This might be a permissions issue.")
	} else {
		fmt.Println("✅ Can create tables")
		// Clean up
		db.Exec("DROP TABLE test_table")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func maskPassword(password string) string {
	if len(password) > 2 {
		return password[:2] + "***"
	}
	return "***"
}

func maskConnectionString(connStr string) string {
	// Simple masking for security
	if len(connStr) > 20 {
		return connStr[:20] + "***"
	}
	return "***"
}
