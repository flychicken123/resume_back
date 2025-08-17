# PowerShell script to run tests on Windows

Write-Host "🧪 Running Backend Tests..." -ForegroundColor Green
Write-Host "================================" -ForegroundColor Green

# Install test dependencies if needed
Write-Host "📦 Installing test dependencies..." -ForegroundColor Cyan
go get -u github.com/stretchr/testify/assert

# Run all tests with coverage
Write-Host "`n📊 Running tests with coverage..." -ForegroundColor Cyan
go test -v -cover ./...

# Run tests with detailed coverage report
Write-Host "`n📈 Generating coverage report..." -ForegroundColor Cyan
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

Write-Host "`n✅ Tests completed!" -ForegroundColor Green
Write-Host "📄 Coverage report saved to coverage.html" -ForegroundColor Yellow

# Run specific middleware tests
Write-Host "`n🔧 Running Middleware Tests..." -ForegroundColor Cyan
Write-Host "--------------------------------" -ForegroundColor Cyan
go test -v ./middleware

# Show coverage percentage
Write-Host "`n📊 Coverage Summary:" -ForegroundColor Cyan
go test -cover ./... | Select-String "coverage"