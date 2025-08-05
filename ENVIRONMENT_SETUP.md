# Environment Setup Guide

## Overview
This project supports two environments:
- **Local Development**: Uses local PostgreSQL and environment files
- **Production**: Uses AWS PostgreSQL and GitHub Secrets

## Local Development Setup

### 1. Database Setup
- Install PostgreSQL locally
- Create database: `resumeai`
- Set password: `admin`
- Update `.env.local` if needed

### 2. Environment Configuration
```powershell
# Switch to local environment
.\switch-env.ps1 -Environment local

# Start the application
.\resumeai.exe
```

### 3. Local Environment Variables
The `.env.local` file contains:
- `DB_HOST=localhost`
- `DB_PORT=5432`
- `DB_USER=postgres`
- `DB_PASSWORD=admin`
- `DB_NAME=resumeai`

## Production Deployment

### 1. GitHub Secrets Required
The following secrets must be configured in your GitHub repository:

**Database Configuration:**
- `DB_HOST` - AWS RDS endpoint
- `DB_PORT` - Database port (usually 5432)
- `DB_USER` - Database username
- `DB_PASSWORD` - Database password
- `DB_NAME` - Database name

**AWS Configuration:**
- `AWS_ACCESS_KEY_ID` - AWS access key
- `AWS_SECRET_ACCESS_KEY` - AWS secret key
- `BACKEND_EC2_HOST` - EC2 instance hostname/IP
- `BACKEND_EC2_SSH_KEY` - SSH private key for EC2

**GCP Configuration:**
- `GCP_PROJECT_ID` - Google Cloud project ID
- `GCP_PRIVATE_KEY_ID` - Private key ID
- `GCP_PRIVATE_KEY` - Private key content
- `GCP_CLIENT_EMAIL` - Service account email
- `GCP_CLIENT_ID` - Client ID
- `GCP_CLIENT_X509_CERT_URL` - Certificate URL

**Application Configuration:**
- `JWT_SECRET` - JWT signing secret
- `GEMINI_API_KEY` - Gemini API key

### 2. Deployment Process
1. Push changes to `main` branch
2. GitHub Actions automatically deploys to EC2
3. Environment variables are created from GitHub Secrets
4. Docker containers are built and started

### 3. Pipeline Workflow
The `.github/workflows/deploy-backend.yml` file:
- Configures AWS credentials
- Sets up environment variables from secrets
- Deploys to EC2 via SSH
- Creates `.env` file from GitHub Secrets
- Builds and starts Docker containers

## Environment Files

- `.env.local` - Local development configuration
- `.env.production` - Not used (production uses GitHub Secrets)
- `.env` - Active environment file (copied from .env.local for local dev)

## Troubleshooting

### Local Issues
- Ensure PostgreSQL is running
- Check database credentials in `.env.local`
- Verify port 8081 is available

### Production Issues
- Check GitHub Secrets are configured
- Verify EC2 instance is accessible
- Check Docker logs on EC2 instance 