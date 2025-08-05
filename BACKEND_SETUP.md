# Backend Deployment Setup

## Overview
This guide sets up the backend CI/CD pipeline for the AI Resume Builder backend service.

## Prerequisites
- AWS EC2 instance at `3.134.103.103`
- RDS PostgreSQL database
- GitHub repository for backend code

## Step 1: Create Backend GitHub Repository

1. Create a new GitHub repository named `resume_back`
2. Make it public
3. Push the backend code to this repository

## Step 2: Set Up GitHub Secrets

In your GitHub repository settings, add these secrets:

### AWS Credentials
- `AWS_ACCESS_KEY_ID`: Your AWS access key
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret key

### EC2 Connection
- `BACKEND_EC2_HOST`: `3.134.103.103`
- `BACKEND_EC2_SSH_KEY`: Your EC2 private key (entire content including BEGIN/END lines)

## Step 3: EC2 Security Group Configuration

Add these inbound rules to your EC2 security group:

| Type | Port | Source | Description |
|------|------|--------|-------------|
| SSH | 22 | Your IP | SSH access |
| Custom TCP | 8081 | 0.0.0.0/0 | Backend API |

## Step 4: Environment Variables

Create a `.env` file on your EC2 instance with:

```env
DB_HOST=your-rds-endpoint
DB_PORT=5432
DB_USER=your-db-username
DB_PASSWORD=your-db-password
DB_NAME=your-db-name
JWT_SECRET=your-jwt-secret-key
```

## Step 5: Deploy

1. Push code to the `main` branch
2. GitHub Actions will automatically deploy to EC2
3. Check deployment status in GitHub Actions

## Step 6: Test Backend

Test the backend API:
```bash
curl http://3.134.103.103:8081/api/auth/login
```

## Troubleshooting

### Check Container Status
```bash
docker ps
docker logs ai-resume-backend-backend-1
```

### Check Security Group
Ensure port 8081 is open in your EC2 security group.

### Check Environment Variables
Ensure the `.env` file exists and has correct database credentials.

## API Endpoints

- **Health Check**: `GET /api/auth/login`
- **Register**: `POST /api/auth/register`
- **Login**: `POST /api/auth/login`
- **Generate Resume**: `POST /api/resume/generate`
- **Generate PDF**: `POST /api/resume/generate-pdf`

## Frontend Integration

Update your frontend to point to the backend:
```javascript
REACT_APP_API_URL=http://3.134.103.103:8081
``` 