Write-Host "🚀 Starting AI Resume Backend for local development..." -ForegroundColor Green

$env:DB_HOST = "localhost"
$env:DB_PORT = "5432"
$env:DB_USER = "postgres"
$env:DB_PASSWORD = "admin"
$env:DB_NAME = "resumeai"
$env:DB_SSLMODE = "disable"
$env:ENVIRONMENT = "development"
# Replace with your actual Gemini API key from https://makersuite.google.com/app/apikey
$env:GEMINI_API_KEY = "YOUR_GEMINI_API_KEY_HERE"

Write-Host "✅ Environment variables set for local development" -ForegroundColor Green
Write-Host "   DB_HOST: $env:DB_HOST" -ForegroundColor Yellow
Write-Host "   DB_USER: $env:DB_USER" -ForegroundColor Yellow
Write-Host "   DB_NAME: $env:DB_NAME" -ForegroundColor Yellow
Write-Host "   GEMINI_API_KEY: Set" -ForegroundColor Yellow
Write-Host ""

Write-Host "Starting server..." -ForegroundColor Cyan
go run .\main.go
