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

	fmt.Println("=== Updating Resume Limits for Testing ===\n")

	// Update limits temporarily
	fmt.Println("Changing limits to:")
	fmt.Println("- Free: 1 resume per week (was 1)")
	fmt.Println("- Premium: 2 resumes per month (was 30)")
	fmt.Println("- Ultimate: 3 resumes per month (was 200)")

	_, err = db.Exec(`
		UPDATE subscription_plans
		SET resume_limit = CASE
			WHEN display_name = 'Free' THEN 1
			WHEN display_name = 'Premium' THEN 2
			WHEN display_name = 'Ultimate' THEN 3
			ELSE resume_limit
		END
		WHERE display_name IN ('Free', 'Premium', 'Ultimate')
	`)

	if err != nil {
		log.Fatal("Failed to update limits:", err)
	}

	fmt.Println("\n✓ Limits updated successfully!")

	// Verify the changes
	fmt.Println("\nVerifying changes:")
	rows, err := db.Query(`
		SELECT display_name, resume_limit, resume_period
		FROM subscription_plans
		ORDER BY id
	`)
	if err != nil {
		log.Fatal("Failed to query plans:", err)
	}
	defer rows.Close()

	for rows.Next() {
		var planName string
		var limit int
		var period string
		err = rows.Scan(&planName, &limit, &period)
		if err != nil {
			continue
		}
		fmt.Printf("  %s: %d resumes per %s\n", planName, limit, period)
	}

	fmt.Println("\n⚠️  Remember to restore original limits after testing!")
	fmt.Println("Run: go run test/restore_limits.go")
}