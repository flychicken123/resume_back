package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	fmt.Println("🔍 AI Resume Builder - Database Diagnostic")
	fmt.Println("==========================================")

	// Load environment variables
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Printf("⚠️  Warning: Error loading .env file: %v\n", err)
		fmt.Println("Continuing with default configuration...")
	}

	// Get database configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "resumeai")

	fmt.Printf("\n📊 Database Configuration:\n")
	fmt.Printf("Host: %s\n", dbHost)
	fmt.Printf("Port: %s\n", dbPort)
	fmt.Printf("User: %s\n", dbUser)
	fmt.Printf("Database: %s\n", dbName)
	fmt.Printf("Password: %s\n", maskPassword(dbPassword))

	// Build connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Open database connection
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		fmt.Printf("❌ Error opening database: %v\n", err)
		return
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		fmt.Printf("❌ Error connecting to database: %v\n", err)
		return
	}

	fmt.Println("✅ Database connection successful!")

	// Check if users table exists
	var tableExists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')").Scan(&tableExists)
	if err != nil {
		fmt.Printf("⚠️  Could not check if users table exists: %v\n", err)
	} else {
		if tableExists {
			fmt.Println("✅ Users table exists")

			// Check table structure
			rows, err := db.Query("SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name = 'users' ORDER BY ordinal_position")
			if err != nil {
				fmt.Printf("⚠️  Could not check table structure: %v\n", err)
			} else {
				defer rows.Close()
				fmt.Println("📋 Users table structure:")
				for rows.Next() {
					var colName, dataType, isNullable string
					rows.Scan(&colName, &dataType, &isNullable)
					fmt.Printf("  - %s (%s, nullable: %s)\n", colName, dataType, isNullable)
				}
			}
		} else {
			fmt.Println("❌ Users table does not exist")
		}
	}

	// Test CREATE TABLE permission
	fmt.Println("\n🔧 Testing permissions...")
	_, err = db.Exec("CREATE TEMP TABLE test_permissions (id int)")
	if err != nil {
		fmt.Printf("❌ Cannot create tables: %v\n", err)
		fmt.Println("This indicates a permissions issue.")
	} else {
		fmt.Println("✅ Can create tables")
		// Clean up
		db.Exec("DROP TABLE test_permissions")
	}

	// Test INSERT permission
	fmt.Println("\n📝 Testing insert permissions...")
	_, err = db.Exec("INSERT INTO users (email, password, name) VALUES ($1, $2, $3)",
		"test@diagnostic.com", "hashed_password", "Test User")
	if err != nil {
		fmt.Printf("❌ Cannot insert into users table: %v\n", err)
	} else {
		fmt.Println("✅ Can insert into users table")
		// Clean up
		db.Exec("DELETE FROM users WHERE email = 'test@diagnostic.com'")
	}

	// Check current user and permissions
	fmt.Println("\n👤 Current user information:")
	var currentUser string
	err = db.QueryRow("SELECT current_user").Scan(&currentUser)
	if err == nil {
		fmt.Printf("Current user: %s\n", currentUser)
	}

	// Check if we're in the right database
	var currentDB string
	err = db.QueryRow("SELECT current_database()").Scan(&currentDB)
	if err == nil {
		fmt.Printf("Current database: %s\n", currentDB)
	}

	fmt.Println("\n🎯 Recommendations:")
	if !tableExists {
		fmt.Println("1. Create the users table using the setup script")
		fmt.Println("2. Or run: go run create_tables.go")
	} else {
		fmt.Println("1. Tables exist, check application logs for specific errors")
		fmt.Println("2. Make sure the application is running on port 8081")
		fmt.Println("3. Test with: go run test_api.go")
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
