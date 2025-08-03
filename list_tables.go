package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load environment variables
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Printf("Warning: Error loading .env file: %v\n", err)
	}

	// Get database configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "resumeai")

	// Build connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Open database connection
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		return
	}

	fmt.Println("✅ Database connection successful!")

	// List all tables
	rows, err := db.Query(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		ORDER BY table_name
	`)
	if err != nil {
		fmt.Printf("Could not list tables: %v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n📋 All tables in database:")
	fmt.Println("---------------------------")

	tableCount := 0
	for rows.Next() {
		var tableName string
		rows.Scan(&tableName)
		fmt.Printf("  - %s\n", tableName)
		tableCount++
	}

	if tableCount == 0 {
		fmt.Println("  (no tables found)")
	} else {
		fmt.Printf("\nTotal tables: %d\n", tableCount)
	}

	// Check for any table that might be related to users
	fmt.Println("\n🔍 Checking for user-related tables...")
	userTables, err := db.Query(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_name ILIKE '%user%'
		ORDER BY table_name
	`)
	if err != nil {
		fmt.Printf("Could not search for user tables: %v\n", err)
		return
	}
	defer userTables.Close()

	userTableCount := 0
	for userTables.Next() {
		var tableName string
		userTables.Scan(&tableName)
		fmt.Printf("  - %s\n", tableName)
		userTableCount++
	}

	if userTableCount == 0 {
		fmt.Println("  (no user-related tables found)")
	} else {
		fmt.Printf("\nUser-related tables: %d\n", userTableCount)
	}

	// If we found any tables, show their structure
	if tableCount > 0 {
		fmt.Println("\n📊 Table structures:")
		fmt.Println("====================")

		allTables, err := db.Query(`
			SELECT table_name 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			ORDER BY table_name
		`)
		if err != nil {
			return
		}
		defer allTables.Close()

		for allTables.Next() {
			var tableName string
			allTables.Scan(&tableName)

			fmt.Printf("\nTable: %s\n", tableName)
			fmt.Println("Columns:")

			columns, err := db.Query(`
				SELECT column_name, data_type, is_nullable
				FROM information_schema.columns 
				WHERE table_schema = 'public' 
				AND table_name = $1
				ORDER BY ordinal_position
			`, tableName)
			if err != nil {
				continue
			}

			for columns.Next() {
				var colName, dataType, isNullable string
				columns.Scan(&colName, &dataType, &isNullable)
				fmt.Printf("  - %s (%s, nullable: %s)\n", colName, dataType, isNullable)
			}
			columns.Close()
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
