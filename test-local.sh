#!/bin/bash

# Local Test Setup for AI Resume Builder Backend
echo "🚀 Setting up local backend test..."

# Check if PostgreSQL is running
echo "📋 Checking PostgreSQL status..."
if ! pg_isready -h localhost -p 5432 > /dev/null 2>&1; then
    echo "❌ PostgreSQL is not running. Please start PostgreSQL first."
    echo "   On Windows: Start PostgreSQL service"
    echo "   On Mac: brew services start postgresql"
    echo "   On Linux: sudo systemctl start postgresql"
    exit 1
fi

echo "✅ PostgreSQL is running"

# Create .env file for local testing
echo "📝 Creating local .env file..."
cat > .env << EOF
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password_here
DB_NAME=resumeai
DB_SSLMODE=disable

# Application Configuration
PORT=8081
ENVIRONMENT=development

# JWT Configuration
JWT_SECRET=local-test-secret-key-change-in-production
JWT_EXPIRATION_HOURS=24

# GCP Configuration (optional for local testing)
GCP_PROJECT_ID=your_gcp_project_id
GCP_PRIVATE_KEY_ID=your_private_key_id
GCP_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nYour private key here\n-----END PRIVATE KEY-----\n"
GCP_CLIENT_EMAIL=your_service_account_email
GCP_CLIENT_ID=your_client_id
GCP_CLIENT_X509_CERT_URL=your_cert_url
EOF

echo "✅ Created .env file"
echo ""
echo "🔧 Next steps:"
echo "1. Update the DB_PASSWORD in .env with your actual PostgreSQL password"
echo "2. Create the database: createdb resumeai"
echo "3. Run the setup script: psql -d resumeai -f setup_database.sql"
echo "4. Test the backend: go run main.go"
echo ""
echo "📋 Quick commands:"
echo "   createdb resumeai"
echo "   psql -d resumeai -f setup_database.sql"
echo "   go run main.go"
echo ""
echo "🌐 Backend will be available at: http://localhost:8081" 