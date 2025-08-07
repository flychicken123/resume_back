# Local Test Setup for AI Resume Builder Backend (Windows)
Write-Host "🚀 Setting up local backend test..." -ForegroundColor Green

# Check if PostgreSQL is running
Write-Host "📋 Checking PostgreSQL status..." -ForegroundColor Yellow
try {
    $pgTest = & pg_isready -h localhost -p 5432 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ PostgreSQL is running" -ForegroundColor Green
    } else {
        Write-Host "❌ PostgreSQL is not running. Please start PostgreSQL first." -ForegroundColor Red
        Write-Host "   On Windows: Start PostgreSQL service from Services" -ForegroundColor Yellow
        Write-Host "   Or run: net start postgresql-x64-15" -ForegroundColor Yellow
        exit 1
    }
} catch {
    Write-Host "❌ PostgreSQL is not running. Please start PostgreSQL first." -ForegroundColor Red
    Write-Host "   On Windows: Start PostgreSQL service from Services" -ForegroundColor Yellow
    exit 1
}

# Create .env file for local testing
Write-Host "📝 Creating local .env file..." -ForegroundColor Yellow
$envContent = @"
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
"@

$envContent | Out-File -FilePath ".env" -Encoding UTF8
Write-Host "✅ Created .env file" -ForegroundColor Green

Write-Host ""
Write-Host "🔧 Next steps:" -ForegroundColor Cyan
Write-Host "1. Update the DB_PASSWORD in .env with your actual PostgreSQL password" -ForegroundColor White
Write-Host "2. Create the database: createdb resumeai" -ForegroundColor White
Write-Host "3. Run the setup script: psql -d resumeai -f setup_database.sql" -ForegroundColor White
Write-Host "4. Test the backend: go run main.go" -ForegroundColor White
Write-Host ""
Write-Host "📋 Quick commands:" -ForegroundColor Cyan
Write-Host "   createdb resumeai" -ForegroundColor White
Write-Host "   psql -d resumeai -f setup_database.sql" -ForegroundColor White
Write-Host "   go run main.go" -ForegroundColor White
Write-Host ""
Write-Host "🌐 Backend will be available at: http://localhost:8081" -ForegroundColor Green 