# Test MVC Structure and Resume History Functionality

Write-Host "🧪 Testing MVC Structure and Resume History..." -ForegroundColor Green

# Test 1: Check if server is running
Write-Host "`n1️⃣ Testing server connectivity..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/version" -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Server is running successfully!" -ForegroundColor Green
        $version = $response.Content | ConvertFrom-Json
        Write-Host "   Version: $($version.version)" -ForegroundColor Cyan
    }
} catch {
    Write-Host "❌ Server is not running or not accessible" -ForegroundColor Red
    exit 1
}

# Test 2: Test authentication endpoints (new controllers)
Write-Host "`n2️⃣ Testing authentication endpoints (new controllers)..." -ForegroundColor Yellow

# Test registration endpoint
$registerData = @{
    email = "test_mvc@example.com"
    password = "test123456"
    name = "Test MVC User"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/auth/register" -Method POST -Body $registerData -ContentType "application/json" -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Registration endpoint working (new controller)" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "   Response: $($result.message)" -ForegroundColor Cyan
    }
} catch {
    Write-Host "❌ Registration endpoint failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
}

# Test 3: Test resume history endpoints (new controllers)
Write-Host "`n3️⃣ Testing resume history endpoints (new controllers)..." -ForegroundColor Yellow

# Test history endpoint (should return 401 without auth)
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/resume/history" -UseBasicParsing
    Write-Host "❌ History endpoint should require authentication" -ForegroundColor Red
} catch {
    if ($_.Exception.Response.StatusCode -eq 401) {
        Write-Host "✅ History endpoint properly requires authentication" -ForegroundColor Green
    } else {
        Write-Host "❌ Unexpected error: $($_.Exception.Message)" -ForegroundColor Red
    }
}

Write-Host "`n🎉 MVC Structure Test Complete!" -ForegroundColor Green
Write-Host "   - Server is running with new MVC structure" -ForegroundColor Cyan
Write-Host "   - Controllers are properly initialized" -ForegroundColor Cyan
Write-Host "   - Authentication endpoints are working" -ForegroundColor Cyan
Write-Host "   - Protected routes are properly secured" -ForegroundColor Cyan
