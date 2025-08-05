package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load environment variables
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
		log.Println("Continuing with default configuration...")
	}

	// Get database configuration
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "resumeai")

	// Build connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Open database connection
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer db.Close()

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}

	fmt.Println("✅ Database connection successful!")

	// Create tables
	schema := `
	-- Users table
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		name VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Resume data table
	CREATE TABLE IF NOT EXISTS resumes (
		id SERIAL PRIMARY KEY,
		user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255),
		phone VARCHAR(100),
		summary TEXT,
		skills TEXT,
		selected_format VARCHAR(50) DEFAULT 'temp1',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Experience entries table
	CREATE TABLE IF NOT EXISTS experiences (
		id SERIAL PRIMARY KEY,
		resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
		job_title VARCHAR(255) NOT NULL,
		company VARCHAR(255) NOT NULL,
		city VARCHAR(100),
		state VARCHAR(100),
		start_date DATE,
		end_date DATE,
		currently_working BOOLEAN DEFAULT FALSE,
		description TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Education entries table
	CREATE TABLE IF NOT EXISTS education (
		id SERIAL PRIMARY KEY,
		resume_id INTEGER REFERENCES resumes(id) ON DELETE CASCADE,
		degree VARCHAR(255) NOT NULL,
		school VARCHAR(255) NOT NULL,
		field VARCHAR(255),
		graduation_year INTEGER,
		gpa VARCHAR(20),
		honors TEXT,
		location VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(schema)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}

	fmt.Println("✅ Tables created successfully!")

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)",
		"CREATE INDEX IF NOT EXISTS idx_resumes_user_id ON resumes(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_experiences_resume_id ON experiences(resume_id)",
		"CREATE INDEX IF NOT EXISTS idx_education_resume_id ON education(resume_id)",
	}

	for _, index := range indexes {
		_, err := db.Exec(index)
		if err != nil {
			log.Printf("Warning: Could not create index: %v", err)
		} else {
			fmt.Printf("✅ Created index\n")
		}
	}

	fmt.Println("✅ Database setup complete!")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
