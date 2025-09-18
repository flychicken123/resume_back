package main

import (
	"database/sql"
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

	fmt.Println("=== Testing Database Functions ===\n")

	// Test 1: Check if function exists
	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_proc
			WHERE proname = 'check_resume_limit'
		)
	`).Scan(&exists)

	if err != nil {
		log.Fatal("Error checking function existence:", err)
	}

	if !exists {
		fmt.Println("✗ Function check_resume_limit does not exist!")
		fmt.Println("Running migration to create it...")

		// Try to create the function
		_, err = db.Exec(`
			CREATE OR REPLACE FUNCTION check_resume_limit(p_user_id INTEGER)
			RETURNS TABLE(can_generate BOOLEAN, remaining_count INTEGER, reset_date TIMESTAMP)
			LANGUAGE plpgsql
			AS $$
			DECLARE
				v_plan_id INTEGER;
				v_resume_limit INTEGER;
				v_resume_period VARCHAR(20);
				v_usage_count INTEGER;
				v_period_start TIMESTAMP;
			BEGIN
				-- Get user's subscription plan (default to free if none)
				SELECT COALESCE(u.subscription_plan_id, 1) INTO v_plan_id
				FROM users u
				WHERE u.id = p_user_id;

				-- If user doesn't exist, allow generation (for new users)
				IF v_plan_id IS NULL THEN
					RETURN QUERY SELECT true::boolean, 1::integer, NULL::timestamp;
					RETURN;
				END IF;

				-- Get plan details
				SELECT resume_limit, resume_period INTO v_resume_limit, v_resume_period
				FROM subscription_plans
				WHERE id = v_plan_id;

				-- Calculate period start date based on plan period
				IF v_resume_period = 'weekly' THEN
					v_period_start := date_trunc('week', CURRENT_TIMESTAMP);
				ELSIF v_resume_period = 'monthly' THEN
					v_period_start := date_trunc('month', CURRENT_TIMESTAMP);
				ELSE
					v_period_start := CURRENT_TIMESTAMP - INTERVAL '30 days';
				END IF;

				-- Count resumes generated in current period
				SELECT COUNT(*) INTO v_usage_count
				FROM resume_usage
				WHERE user_id = p_user_id
				AND created_at >= v_period_start;

				-- Return result
				RETURN QUERY
				SELECT
					(v_usage_count < v_resume_limit)::boolean as can_generate,
					GREATEST(0, v_resume_limit - v_usage_count)::integer as remaining_count,
					CASE
						WHEN v_resume_period = 'weekly' THEN date_trunc('week', CURRENT_TIMESTAMP) + INTERVAL '7 days'
						WHEN v_resume_period = 'monthly' THEN date_trunc('month', CURRENT_TIMESTAMP) + INTERVAL '1 month'
						ELSE CURRENT_TIMESTAMP + INTERVAL '30 days'
					END as reset_date;
			END;
			$$;
		`)

		if err != nil {
			log.Fatal("Failed to create function:", err)
		}
		fmt.Println("✓ Function created successfully")
	} else {
		fmt.Println("✓ Function check_resume_limit exists")
	}

	// Test 2: Test the function with a test user
	fmt.Println("\nTest 2: Testing function with user ID 7")

	var canGenerate sql.NullBool
	var remaining sql.NullInt64
	var resetDate sql.NullTime

	err = db.QueryRow(`SELECT can_generate, remaining_count, reset_date FROM check_resume_limit($1)`, 7).Scan(&canGenerate, &remaining, &resetDate)

	if err != nil {
		fmt.Printf("✗ Error calling function: %v\n", err)

		// Try to debug - check if user exists
		var userExists bool
		err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, 7).Scan(&userExists)
		if err == nil {
			fmt.Printf("  User ID 7 exists: %v\n", userExists)
		}

		// Check subscription plans table
		var planCount int
		err = db.QueryRow(`SELECT COUNT(*) FROM subscription_plans`).Scan(&planCount)
		if err == nil {
			fmt.Printf("  Subscription plans count: %d\n", planCount)
		}

		// Check resume_usage table
		var tableExists bool
		err = db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'resume_usage'
			)
		`).Scan(&tableExists)
		if err == nil {
			fmt.Printf("  resume_usage table exists: %v\n", tableExists)
		}
	} else {
		fmt.Println("✓ Function executed successfully")
		if canGenerate.Valid {
			fmt.Printf("  Can generate: %v\n", canGenerate.Bool)
		} else {
			fmt.Printf("  Can generate: NULL\n")
		}
		if remaining.Valid {
			fmt.Printf("  Remaining: %d\n", remaining.Int64)
		} else {
			fmt.Printf("  Remaining: NULL\n")
		}
		if resetDate.Valid {
			fmt.Printf("  Reset date: %v\n", resetDate.Time)
		} else {
			fmt.Printf("  Reset date: NULL\n")
		}
	}
}