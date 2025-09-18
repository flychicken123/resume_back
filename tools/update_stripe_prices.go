package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	// Database connection
	dbHost := "localhost"
	dbPort := "5432"
	dbUser := "postgres"
	dbPassword := "admin"
	dbName := "resumeai"

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Update Premium plan
	_, err = db.Exec(`UPDATE subscription_plans SET stripe_price_id = $1, updated_at = CURRENT_TIMESTAMP WHERE name = 'premium'`,
		"price_1S8CjPBdXAROpZNpXqSoy8vU")
	if err != nil {
		log.Fatal("Failed to update premium plan:", err)
	}
	fmt.Println("✅ Updated Premium plan with Stripe price ID")

	// Update Ultimate plan
	_, err = db.Exec(`UPDATE subscription_plans SET stripe_price_id = $1, updated_at = CURRENT_TIMESTAMP WHERE name = 'ultimate'`,
		"price_1S8CjQBdXAROpZNpXsJQBbG8")
	if err != nil {
		log.Fatal("Failed to update ultimate plan:", err)
	}
	fmt.Println("✅ Updated Ultimate plan with Stripe price ID")

	// Verify the updates
	rows, err := db.Query(`SELECT name, stripe_price_id, price FROM subscription_plans WHERE stripe_price_id IS NOT NULL`)
	if err != nil {
		log.Fatal("Failed to query plans:", err)
	}
	defer rows.Close()

	fmt.Println("\n📊 Current Stripe Price IDs:")
	for rows.Next() {
		var name, priceID string
		var price float64
		rows.Scan(&name, &priceID, &price)
		fmt.Printf("  %s: %s ($%.2f)\n", name, priceID, price)
	}

	fmt.Println("\n✅ Database updated successfully!")
	fmt.Println("Your Stripe subscription system is now fully configured!")
}