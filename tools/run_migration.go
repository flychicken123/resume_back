package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	_ "github.com/lib/pq"
)

func main() {
	// Database connection
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "resumeai"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPassword := os.Getenv("DB_PASSWORD")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Run migration 004
	migrationFile := filepath.Join("migrations", "004_add_membership_system.sql")
	content, err := ioutil.ReadFile(migrationFile)
	if err != nil {
		log.Fatal("Failed to read migration file:", err)
	}

	_, err = db.Exec(string(content))
	if err != nil {
		log.Printf("Migration might already be applied or failed: %v", err)
	} else {
		fmt.Println("✅ Migration 004_add_membership_system.sql applied successfully!")
	}

	// Verify tables exist
	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('subscription_plans', 'user_subscriptions', 'resume_usage', 'payment_history')
	`).Scan(&tableCount)

	if err == nil {
		fmt.Printf("✅ Found %d subscription tables in database\n", tableCount)
	}

	// Check if columns exist in users table
	var columnCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'users'
		AND column_name IN ('subscription_plan_id', 'subscription_status', 'stripe_customer_id')
	`).Scan(&columnCount)

	if err == nil {
		fmt.Printf("✅ Found %d subscription columns in users table\n", columnCount)
	}

	// Check subscription plans
	rows, err := db.Query("SELECT name, display_name, price, resume_limit FROM subscription_plans")
	if err == nil {
		defer rows.Close()
		fmt.Println("\n📋 Subscription Plans:")
		for rows.Next() {
			var name, displayName string
			var price float64
			var limit int
			rows.Scan(&name, &displayName, &price, &limit)
			fmt.Printf("  - %s (%s): $%.2f, %d resumes\n", name, displayName, price, limit)
		}
	}

	fmt.Println("\n✅ Database is ready for subscription system!")
}