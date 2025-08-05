# Quick Setup Guide

## Step 1: Test Database Connection

Run the database test script to check your connection:

```bash
go run test_db_connection.go
```

This will tell you if:
- Database connection works
- Users table exists
- You have permission to create tables

## Step 2: Create .env File

Create a `.env` file in the back directory:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_postgres_password
DB_NAME=resumeai
DB_SSLMODE=disable

# Application Configuration
PORT=8081
ENVIRONMENT=development

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRATION_HOURS=24
```

## Step 3: Create Database (if needed)

If the database doesn't exist, create it:

```bash
# Connect to PostgreSQL as superuser
psql -U postgres

# Create database
CREATE DATABASE resumeai;

# Exit
\q
```

## Step 4: Create Tables (if needed)

If tables don't exist, create them:

```bash
# Run the setup script
psql -U postgres -d resumeai -f setup_database.sql
```

## Step 5: Test Registration

Once everything is set up, test registration:

```bash
curl -X POST http://localhost:8081/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

## Common Issues

### 1. "Database connection failed"
- Check if PostgreSQL is running
- Verify database credentials in `.env`
- Make sure database exists

### 2. "Users table does not exist"
- Run the setup script: `psql -U postgres -d resumeai -f setup_database.sql`

### 3. "Cannot create tables"
- Grant permissions to your database user
- Or use postgres superuser temporarily

### 4. "Failed to create user account"
- Check the server logs for specific error messages
- Usually indicates database permission issues

## Quick Fix for Development

For quick testing, you can use the postgres superuser:

```bash
# In your .env file
DB_USER=postgres
DB_PASSWORD=your_postgres_password
```

**Note:** Only use this for development, never in production! 