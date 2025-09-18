package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

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

	// Get user email
	email := "flychicken1991@gmail.com" // Your email
	planName := "premium" // or "ultimate"

	// Get user ID
	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID)
	if err != nil {
		log.Fatal("User not found:", err)
	}

	// Get plan ID
	var planID int
	err = db.QueryRow("SELECT id FROM subscription_plans WHERE name = $1", planName).Scan(&planID)
	if err != nil {
		log.Fatal("Plan not found:", err)
	}

	// Update user's subscription
	_, err = db.Exec(`
		UPDATE users
		SET subscription_plan_id = $1,
		    subscription_status = 'active'
		WHERE id = $2
	`, planID, userID)
	if err != nil {
		log.Fatal("Failed to update user:", err)
	}

	// Create or update subscription record
	_, err = db.Exec(`
		INSERT INTO user_subscriptions (
			user_id, plan_id, status,
			current_period_start, current_period_end
		) VALUES ($1, $2, 'active', $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			plan_id = $2,
			status = 'active',
			current_period_start = $3,
			current_period_end = $4,
			updated_at = CURRENT_TIMESTAMP
	`, userID, planID, time.Now(), time.Now().AddDate(0, 1, 0))
	if err != nil {
		log.Fatal("Failed to create subscription:", err)
	}

	fmt.Printf("✅ Successfully upgraded %s to %s plan!\n", email, planName)

	// Verify the upgrade
	var currentPlan string
	var status string
	err = db.QueryRow(`
		SELECT sp.name, u.subscription_status
		FROM users u
		JOIN subscription_plans sp ON sp.id = u.subscription_plan_id
		WHERE u.id = $1
	`, userID).Scan(&currentPlan, &status)

	if err == nil {
		fmt.Printf("📋 Current subscription: %s (status: %s)\n", currentPlan, status)
	}
}