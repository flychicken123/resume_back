# Zero-downtime deployment script for Windows PowerShell

Write-Host "🚀 Starting zero-downtime backend deployment..." -ForegroundColor Green

# Check if docker-compose is available
if (-not (Get-Command docker-compose -ErrorAction SilentlyContinue)) {
    Write-Host "❌ docker-compose not found. Please install Docker Compose." -ForegroundColor Red
    exit 1
}

# Check if .env file exists
if (-not (Test-Path .env)) {
    Write-Host "⚠️  .env file not found. Creating from template..." -ForegroundColor Yellow
    if (Test-Path .env.example) {
        Copy-Item .env.example .env
        Write-Host "📝 Please edit .env file with your actual values and run again." -ForegroundColor Yellow
        exit 1
    } else {
        Write-Host "❌ .env.example not found. Please create .env file manually." -ForegroundColor Red
        exit 1
    }
}

# Ensure network exists
Write-Host "🔗 Setting up Docker network..." -ForegroundColor Cyan
docker network create ai-resume-network 2>$null

# Build the new image
Write-Host "🔨 Building new backend image..." -ForegroundColor Cyan
docker-compose -f docker-compose.backend.yml build --no-cache backend

# Check if there's an existing service running
$existingService = docker-compose -f docker-compose.backend.yml ps backend | Select-String "Up"

if ($existingService) {
    Write-Host "📊 Existing service detected, performing zero-downtime deployment..." -ForegroundColor Yellow
    
    # Scale up to 2 instances (new + old)
    docker-compose -f docker-compose.backend.yml up -d --scale backend=2 --no-recreate
    
    # Wait for new instance to be healthy
    Write-Host "⏳ Waiting for new instance to be healthy..." -ForegroundColor Yellow
    Start-Sleep 30
    
    # Check health of new instance
    try {
        $healthCheck = Invoke-WebRequest -Uri "http://localhost:8081/api/health" -Method GET -TimeoutSec 10
        if ($healthCheck.StatusCode -eq 200) {
            Write-Host "✅ New instance is healthy, scaling down to 1 instance" -ForegroundColor Green
            docker-compose -f docker-compose.backend.yml up -d --scale backend=1 --no-recreate
            Write-Host "🎉 Zero-downtime deployment completed successfully!" -ForegroundColor Green
        } else {
            throw "Health check failed"
        }
    } catch {
        Write-Host "❌ New instance failed health check, rolling back..." -ForegroundColor Red
        docker-compose -f docker-compose.backend.yml up -d --scale backend=1 --no-recreate
        Write-Host "🔄 Rollback completed - service restored to previous version" -ForegroundColor Yellow
        exit 1
    }
} else {
    Write-Host "🆕 No existing service, performing fresh deployment..." -ForegroundColor Cyan
    docker-compose -f docker-compose.backend.yml up -d
}

# Wait for service to be ready
Write-Host "⏳ Waiting for service to be ready..." -ForegroundColor Yellow
Start-Sleep 10

# Verify deployment
Write-Host "✅ Verifying deployment..." -ForegroundColor Cyan
try {
    $healthCheck = Invoke-WebRequest -Uri "http://localhost:8081/api/health" -Method GET -TimeoutSec 10
    if ($healthCheck.StatusCode -eq 200) {
        Write-Host "🎉 Deployment successful!" -ForegroundColor Green
        Write-Host "💚 Health check: PASSED" -ForegroundColor Green
        
        # Clean up old images
        Write-Host "🧹 Cleaning up old images..." -ForegroundColor Cyan
        docker image prune -f
        
        Write-Host "✨ Zero-downtime deployment completed successfully!" -ForegroundColor Green
        Write-Host "📊 Service status:" -ForegroundColor Cyan
        docker-compose -f docker-compose.backend.yml ps
    } else {
        throw "Health check failed"
    }
} catch {
    Write-Host "❌ Health check failed. Please check the logs:" -ForegroundColor Red
    docker-compose -f docker-compose.backend.yml logs backend --tail 20
    exit 1
}