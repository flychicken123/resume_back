package main

import (
	"fmt"
	"log"
	"resumeai/database"

	_ "github.com/lib/pq"
)

func main() {
	// Connect to database
	db, err := database.Connect("localhost", "5432", "postgres", "admin", "resumeai", "disable")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	fmt.Println("=== Checking Current Plans ===\n")

	// First check what's actually in the database
	rows, err := db.Query(`
		SELECT id, display_name, resume_limit, resume_period, price, stripe_price_id
		FROM subscription_plans
		ORDER BY id
	`)
	if err != nil {
		log.Fatal("Failed to query plans:", err)
	}
	defer rows.Close()

	fmt.Println("Current plans in database:")
	for rows.Next() {
		var id int
		var displayName string
		var limit int
		var period string
		var price float64
		var stripePriceID *string
		err = rows.Scan(&id, &displayName, &limit, &period, &price, &stripePriceID)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}
		fmt.Printf("  ID=%d, Name='%s', Limit=%d per %s, Price=$%.2f\n",
			id, displayName, limit, period, price)
	}

	// Now update with exact names
	fmt.Println("\nUpdating limits to test values...")
	result, err := db.Exec(`
		UPDATE subscription_plans
		SET resume_limit = CASE
			WHEN id = 1 THEN 1  -- Free
			WHEN id = 2 THEN 2  -- Premium
			WHEN id = 3 THEN 3  -- Ultimate
			ELSE resume_limit
		END
		WHERE id IN (1, 2, 3)
	`)

	if err != nil {
		log.Fatal("Failed to update limits:", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Updated %d rows\n", rowsAffected)

	// Verify the changes
	fmt.Println("\nAfter update:")
	rows2, err := db.Query(`
		SELECT id, display_name, resume_limit, resume_period
		FROM subscription_plans
		ORDER BY id
	`)
	if err != nil {
		log.Fatal("Failed to query plans:", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var id int
		var displayName string
		var limit int
		var period string
		err = rows2.Scan(&id, &displayName, &limit, &period)
		if err != nil {
			continue
		}
		fmt.Printf("  %s: %d resumes per %s\n", displayName, limit, period)
	}
}