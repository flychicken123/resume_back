#!/bin/bash

# Zero-downtime deployment script for backend
# Run this script from the backend directory

set -e

echo "🚀 Starting zero-downtime backend deployment..."

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose not found. Please install Docker Compose."
    exit 1
fi

# Check if .env file exists
if [ ! -f .env ]; then
    echo "⚠️  .env file not found. Creating from template..."
    if [ -f .env.example ]; then
        cp .env.example .env
        echo "📝 Please edit .env file with your actual values and run again."
        exit 1
    else
        echo "❌ .env.example not found. Please create .env file manually."
        exit 1
    fi
fi

# Load environment variables
source .env

# Ensure network exists
echo "🔗 Setting up Docker network..."
docker network create ai-resume-network || true

# Build the new image
echo "🔨 Building new backend image..."
docker-compose -f docker-compose.backend.yml build --no-cache backend

# Check if there's an existing service running
if docker-compose -f docker-compose.backend.yml ps backend | grep -q "Up"; then
    echo "📊 Existing service detected, performing zero-downtime deployment..."
    
    # Scale up to 2 instances (new + old)
    docker-compose -f docker-compose.backend.yml up -d --scale backend=2 --no-recreate
    
    # Wait for new instance to be healthy
    echo "⏳ Waiting for new instance to be healthy..."
    sleep 30
    
    # Check health of new instance
    if curl -f http://localhost:8081/api/health > /dev/null 2>&1; then
        echo "✅ New instance is healthy, scaling down to 1 instance"
        docker-compose -f docker-compose.backend.yml up -d --scale backend=1 --no-recreate
        echo "🎉 Zero-downtime deployment completed successfully!"
    else
        echo "❌ New instance failed health check, rolling back..."
        docker-compose -f docker-compose.backend.yml up -d --scale backend=1 --no-recreate
        echo "🔄 Rollback completed - service restored to previous version"
        exit 1
    fi
else
    echo "🆕 No existing service, performing fresh deployment..."
    docker-compose -f docker-compose.backend.yml up -d
fi

# Wait for service to be ready
echo "⏳ Waiting for service to be ready..."
sleep 10

# Verify deployment
echo "✅ Verifying deployment..."
if curl -f http://localhost:8081/api/health; then
    echo "🎉 Deployment successful!"
    echo "💚 Health check: PASSED"
    
    # Clean up old images
    echo "🧹 Cleaning up old images..."
    docker image prune -f
    
    echo "✨ Zero-downtime deployment completed successfully!"
    echo "📊 Service status:"
    docker-compose -f docker-compose.backend.yml ps
else
    echo "❌ Health check failed. Please check the logs:"
    docker-compose -f docker-compose.backend.yml logs backend --tail=20
    exit 1
fi