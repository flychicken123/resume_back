Write-Host "🚀 Starting AI Resume Backend for local development..." -ForegroundColor Green

$env:DB_HOST = "localhost"
$env:DB_PORT = "5432"
$env:DB_USER = "postgres"
$env:DB_PASSWORD = "admin"
$env:DB_NAME = "resumeai"
$env:DB_SSLMODE = "disable"
$env:ENVIRONMENT = "development"
$env:GEMINI_API_KEY = "AIzaSyD18Ge_xBz7dyvvOpE2yju2XJk60hgK9ww"
$env:AWS_REGION  = "us-east-2"
$env:AWS_S3_BUCKET  = "airesumestorage"
# AWS credentials should be set via environment variables or .env file
# $env:AWS_ACCESS_KEY_ID = "your-access-key"
# $env:AWS_SECRET_ACCESS_KEY = "your-secret-key"

Write-Host "✅ Environment variables set for local development" -ForegroundColor Green
Write-Host "   DB_HOST: $env:DB_HOST" -ForegroundColor Yellow
Write-Host "   DB_USER: $env:DB_USER" -ForegroundColor Yellow
Write-Host "   DB_NAME: $env:DB_NAME" -ForegroundColor Yellow
Write-Host "   GEMINI_API_KEY: Set" -ForegroundColor Yellow
Write-Host ""

Write-Host "Starting server..." -ForegroundColor Cyan
go run .\main.go
