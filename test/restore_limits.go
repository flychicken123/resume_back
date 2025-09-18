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

	fmt.Println("=== Restoring Original Resume Limits ===\n")

	// Restore original limits
	fmt.Println("Restoring limits to:")
	fmt.Println("- Free: 1 resume per week")
	fmt.Println("- Premium: 30 resumes per month")
	fmt.Println("- Ultimate: 200 resumes per month")

	_, err = db.Exec(`
		UPDATE subscription_plans
		SET resume_limit = CASE
			WHEN display_name = 'Free' THEN 1
			WHEN display_name = 'Premium' THEN 30
			WHEN display_name = 'Ultimate' THEN 200
			ELSE resume_limit
		END
		WHERE display_name IN ('Free', 'Premium', 'Ultimate')
	`)

	if err != nil {
		log.Fatal("Failed to restore limits:", err)
	}

	fmt.Println("\n✓ Limits restored successfully!")

	// Verify the changes
	fmt.Println("\nCurrent limits:")
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
}