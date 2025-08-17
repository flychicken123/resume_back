# Backend Zero-Downtime Deployment

This backend uses Docker Compose for zero-downtime deployments with environment secrets from GitHub.

## Environment Setup

### GitHub Environment Secrets

Your GitHub Actions workflow uses the `prod` environment. Ensure these secrets are configured:

**Database:**
- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`
- `DB_SSLMODE`

**Authentication:**
- `JWT_SECRET`

**Google Cloud Platform:**
- `GCP_PROJECT_ID`
- `GCP_PRIVATE_KEY_ID`
- `GCP_PRIVATE_KEY`
- `GCP_CLIENT_EMAIL`
- `GCP_CLIENT_ID`
- `GCP_CLIENT_X509_CERT_URL`

**AI Services:**
- `GEMINI_API_KEY`

**AWS:**
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`
- `AWS_S3_BUCKET`

**Infrastructure:**
- `BACKEND_EC2_HOST`
- `BACKEND_EC2_SSH_KEY`

## Deployment Methods

### 1. Automatic Deployment (GitHub Actions)

✅ **Current Setup** - Your existing workflow now includes zero-downtime deployment:

```yaml
# Triggers on push to main branch
# Uses environment: prod
# Automatically scales containers for zero-downtime
```

**What Changed:**
- Added health check endpoint: `/api/health`
- Modified deployment to scale up before scaling down
- Added rollback on health check failure
- Improved logging and verification

### 2. Manual Deployment

For local or manual deployments:

```bash
# Make script executable
chmod +x deploy.sh

# Create environment file
cp .env.example .env
# Edit .env with your values

# Run zero-downtime deployment
./deploy.sh
```

## Zero-Downtime Process

1. **Build**: New Docker image built without stopping current service
2. **Scale Up**: Start new container alongside existing one
3. **Health Check**: Verify new container responds to `/api/health`
4. **Scale Down**: Remove old container once new one is healthy
5. **Cleanup**: Remove unused Docker images

## Health Check Endpoint

- **URL**: `GET /api/health`
- **Response**: `{"status": "healthy", "timestamp": 1234567890, "version": "1.0.1"}`
- **Used by**: Docker Compose health checks and deployment verification

## Benefits

✅ **Zero Downtime** - Service stays available during deployments  
✅ **Health Verification** - New version tested before going live  
✅ **Automatic Rollback** - Falls back to working version on failure  
✅ **Environment Secrets** - Secure secret management via GitHub  
✅ **Comprehensive Logging** - Clear deployment status and error reporting  

## Troubleshooting

### Check Service Status
```bash
docker-compose -f docker-compose.backend.yml ps
```

### View Logs
```bash
docker-compose -f docker-compose.backend.yml logs backend --tail=50
```

### Manual Health Check
```bash
curl http://localhost:8081/api/health
# or for production:
curl https://hihired.org/api/health
```

### Rollback if Needed
```bash
# Restart with last known good configuration
docker-compose -f docker-compose.backend.yml restart backend
```

## Monitoring

The deployment includes several verification steps:
- Health endpoint testing
- CORS preflight testing
- Key API endpoint verification
- Docker container status checks

All deployment steps are logged with clear emoji indicators for easy monitoring in GitHub Actions.