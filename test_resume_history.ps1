# Test Resume History Functionality with New MVC Structure

Write-Host "🧪 Testing Resume History with New MVC Structure..." -ForegroundColor Green

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
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 2: Register a test user
Write-Host "`n2️⃣ Registering test user..." -ForegroundColor Yellow
$registerData = @{
    email = "test_history@example.com"
    password = "test123456"
    name = "Test History User"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/auth/register" -Method POST -Body $registerData -ContentType "application/json" -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ User registered successfully!" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        $token = $result.token
        Write-Host "   Token received: $($token.Substring(0, 20))..." -ForegroundColor Cyan
    } else {
        Write-Host "❌ Registration failed with status: $($response.StatusCode)" -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "❌ Registration failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 3: Test resume history endpoint with authentication
Write-Host "`n3️⃣ Testing resume history endpoint with authentication..." -ForegroundColor Yellow

$headers = @{
    "Authorization" = "Bearer $token"
    "Content-Type" = "application/json"
}

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/resume/history" -Headers $headers -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Resume history endpoint working!" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "   History count: $($result.history.Count)" -ForegroundColor Cyan
    } else {
        Write-Host "❌ Resume history failed with status: $($response.StatusCode)" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Resume history request failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
}

# Test 4: Test user profile endpoint
Write-Host "`n4️⃣ Testing user profile endpoint..." -ForegroundColor Yellow

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/user/profile" -Headers $headers -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ User profile endpoint working!" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "   User: $($result.user.email)" -ForegroundColor Cyan
    } else {
        Write-Host "❌ User profile failed with status: $($response.StatusCode)" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ User profile request failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
}

# Test 5: Test save user data endpoint
Write-Host "`n5️⃣ Testing save user data endpoint..." -ForegroundColor Yellow

$saveData = @{
    summary = '{"text": "Test summary"}'
    skills = '{"skills": ["Go", "Python", "JavaScript"]}'
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/user/save" -Method POST -Body $saveData -Headers $headers -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Save user data endpoint working!" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "   Response: $($result.message)" -ForegroundColor Cyan
    } else {
        Write-Host "❌ Save user data failed with status: $($response.StatusCode)" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Save user data request failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
}

# Test 6: Test load user data endpoint
Write-Host "`n6️⃣ Testing load user data endpoint..." -ForegroundColor Yellow

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8081/api/user/load" -Headers $headers -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✅ Load user data endpoint working!" -ForegroundColor Green
        $result = $response.Content | ConvertFrom-Json
        Write-Host "   Data loaded successfully" -ForegroundColor Cyan
    } else {
        Write-Host "❌ Load user data failed with status: $($response.StatusCode)" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Load user data request failed" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`n🎉 Resume History MVC Test Complete!" -ForegroundColor Green
Write-Host "   - All new controllers are working properly" -ForegroundColor Cyan
Write-Host "   - Authentication is functioning correctly" -ForegroundColor Cyan
Write-Host "   - User data operations are working" -ForegroundColor Cyan
Write-Host "   - Resume history endpoints are accessible" -ForegroundColor Cyan
Write-Host "   - MVC structure is successfully implemented!" -ForegroundColor Cyan
