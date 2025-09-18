package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	// Database connection parameters
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		log.Fatal("DB_PASSWORD environment variable is required")
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "resumeai"
	}

	// Connect to database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Failed to ping database:", err)
	}
	fmt.Println("✅ Connected to database")

	// Read migration file
	migrationFile := "migrations/004_add_membership_system.sql"
	sqlContent, err := ioutil.ReadFile(migrationFile)
	if err != nil {
		log.Fatal("Failed to read migration file:", err)
	}

	// Execute migration
	fmt.Println("📝 Running membership system migration...")
	_, err = db.Exec(string(sqlContent))
	if err != nil {
		log.Fatal("Failed to execute migration:", err)
	}

	fmt.Println("✅ Migration completed successfully!")
	fmt.Println("📊 Subscription plans created:")

	// Show created plans
	rows, err := db.Query("SELECT name, display_name, price, resume_limit, resume_period FROM subscription_plans ORDER BY price")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, displayName, period string
			var price float64
			var limit int
			rows.Scan(&name, &displayName, &price, &limit, &period)
			fmt.Printf("  - %s (%s): $%.2f - %d resumes per %s\n", displayName, name, price, limit, period)
		}
	}
}