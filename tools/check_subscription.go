package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

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

	// Check user's subscription
	email := "flychicken1991@gmail.com"

	var userID int
	var planID sql.NullInt64
	var status sql.NullString
	var stripeCustomerID sql.NullString

	err = db.QueryRow(`
		SELECT id, subscription_plan_id, subscription_status, stripe_customer_id
		FROM users
		WHERE email = $1
	`, email).Scan(&userID, &planID, &status, &stripeCustomerID)

	if err != nil {
		log.Fatal("User not found:", err)
	}

	fmt.Printf("👤 User: %s (ID: %d)\n", email, userID)

	if planID.Valid {
		fmt.Printf("📋 Plan ID: %d\n", planID.Int64)
	} else {
		fmt.Printf("📋 Plan ID: NULL (defaults to 1 - Free)\n")
	}

	if status.Valid {
		fmt.Printf("✨ Status: %s\n", status.String)
	} else {
		fmt.Printf("✨ Status: NULL (defaults to 'free')\n")
	}

	if stripeCustomerID.Valid {
		fmt.Printf("💳 Stripe Customer: %s\n", stripeCustomerID.String)
	} else {
		fmt.Printf("💳 Stripe Customer: Not set\n")
	}

	// Get the plan details
	if planID.Valid {
		var planName, displayName string
		var price float64
		var limit int

		err = db.QueryRow(`
			SELECT name, display_name, price, resume_limit
			FROM subscription_plans
			WHERE id = $1
		`, planID.Int64).Scan(&planName, &displayName, &price, &limit)

		if err == nil {
			fmt.Printf("\n🎯 Current Plan Details:\n")
			fmt.Printf("   Name: %s\n", displayName)
			fmt.Printf("   Price: $%.2f\n", price)
			fmt.Printf("   Resume Limit: %d per month\n", limit)
		}
	}

	// Check if there's a subscription record
	var subExists bool
	err = db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_subscriptions WHERE user_id = $1
		)
	`, userID).Scan(&subExists)

	if subExists {
		fmt.Println("\n✅ Has subscription record in user_subscriptions table")

		var subPlanID int
		var subStatus string
		err = db.QueryRow(`
			SELECT plan_id, status
			FROM user_subscriptions
			WHERE user_id = $1
		`, userID).Scan(&subPlanID, &subStatus)

		if err == nil {
			fmt.Printf("   Subscription Plan ID: %d\n", subPlanID)
			fmt.Printf("   Subscription Status: %s\n", subStatus)
		}
	} else {
		fmt.Println("\n❌ No subscription record in user_subscriptions table")
	}
}