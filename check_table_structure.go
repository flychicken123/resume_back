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

	// Check if users table exists
	var tableExists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')").Scan(&tableExists)
	if err != nil {
		fmt.Printf("Could not check if users table exists: %v\n", err)
		return
	}

	if !tableExists {
		fmt.Println("❌ Users table does not exist")
		return
	}

	fmt.Println("✅ Users table exists")

	// Get table structure
	rows, err := db.Query(`
		SELECT 
			column_name, 
			data_type, 
			is_nullable,
			column_default
		FROM information_schema.columns 
		WHERE table_name = 'users' 
		ORDER BY ordinal_position
	`)
	if err != nil {
		fmt.Printf("Could not get table structure: %v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n📋 Users table structure:")
	fmt.Printf("%-20s %-15s %-10s %-20s\n", "Column", "Type", "Nullable", "Default")
	fmt.Println("------------------------------------------------------------")

	for rows.Next() {
		var colName, dataType, isNullable, columnDefault sql.NullString
		rows.Scan(&colName, &dataType, &isNullable, &columnDefault)

		defaultValue := "NULL"
		if columnDefault.Valid {
			defaultValue = columnDefault.String
		}

		fmt.Printf("%-20s %-15s %-10s %-20s\n",
			colName.String, dataType.String, isNullable.String, defaultValue)
	}

	// Check for any existing data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		fmt.Printf("Could not count users: %v\n", err)
	} else {
		fmt.Printf("\n📊 Current user count: %d\n", count)
	}

	// Try to drop and recreate the table
	fmt.Println("\n🔄 Attempting to recreate users table...")

	// Drop the table if it exists
	_, err = db.Exec("DROP TABLE IF EXISTS users CASCADE")
	if err != nil {
		fmt.Printf("Could not drop users table: %v\n", err)
		return
	}
	fmt.Println("✅ Dropped existing users table")

	// Create the table with correct structure
	createTableSQL := `
	CREATE TABLE users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		name VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		fmt.Printf("Could not create users table: %v\n", err)
		return
	}
	fmt.Println("✅ Created users table with correct structure")

	// Test insert
	_, err = db.Exec("INSERT INTO users (email, password, name) VALUES ($1, $2, $3)",
		"test@example.com", "hashed_password", "Test User")
	if err != nil {
		fmt.Printf("Could not insert test user: %v\n", err)
	} else {
		fmt.Println("✅ Successfully inserted test user")
		// Clean up
		db.Exec("DELETE FROM users WHERE email = 'test@example.com'")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
