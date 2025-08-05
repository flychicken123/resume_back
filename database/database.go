package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func Connect(host, port, user, password, dbname string) (*sql.DB, error) {
	// Build connection string
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	// Open database connection
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	return db, nil
}

// InitializeDatabase creates tables if they don't exist
func InitializeDatabase(db *sql.DB) error {
	// Check if tables already exist
	var tableExists bool
	err := db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')").Scan(&tableExists)
	if err != nil {
		log.Printf("Warning: Could not check if tables exist: %v", err)
		// Continue anyway, try to create tables
	}

	if tableExists {
		log.Println("Database tables already exist, skipping initialization")
		return nil
	}

	log.Println("Creating database tables...")

	// Create tables one by one to handle errors better
	tables := []struct {
		name   string
		schema string
	}{
		{
			name: "users",
			schema: `
			CREATE TABLE IF NOT EXISTS users (
				id SERIAL PRIMARY KEY,
				email VARCHAR(255) UNIQUE NOT NULL,
				password VARCHAR(255) NOT NULL,
				name VARCHAR(255),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "resumes",
			schema: `
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
			)`,
		},
		{
			name: "experiences",
			schema: `
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
			)`,
		},
		{
			name: "education",
			schema: `
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
			)`,
		},
	}

	for _, table := range tables {
		log.Printf("Creating table: %s", table.name)
		_, err := db.Exec(table.schema)
		if err != nil {
			log.Printf("Error creating table %s: %v", table.name, err)
			return fmt.Errorf("error creating table %s: %v", table.name, err)
		}
		log.Printf("✅ Created table: %s", table.name)
	}

	// Create indexes separately to handle permission issues
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
			// Continue without failing, indexes are not critical
		} else {
			log.Printf("✅ Created index")
		}
	}

	log.Println("Database tables initialized successfully")
	return nil
}

// CheckDatabasePermissions checks if the current user has necessary permissions
func CheckDatabasePermissions(db *sql.DB) error {
	// Test basic permissions
	_, err := db.Exec("SELECT 1")
	if err != nil {
		return fmt.Errorf("database connection test failed: %v", err)
	}

	// Test CREATE TABLE permission
	_, err = db.Exec("CREATE TEMP TABLE test_permissions (id int)")
	if err != nil {
		return fmt.Errorf("user lacks CREATE TABLE permission: %v", err)
	}

	// Clean up test table
	_, err = db.Exec("DROP TABLE test_permissions")
	if err != nil {
		log.Printf("Warning: Could not drop test table: %v", err)
	}

	return nil
}
