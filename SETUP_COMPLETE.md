# Complete Backend Setup Guide

## ✅ Security Issue Resolved
- **Removed leaked GCP credentials** from the repository
- **Updated code** to use environment variables instead of JSON files
- **Added comprehensive `.gitignore`** to prevent future credential leaks

## 🚀 Next Steps for Deployment

### 1. Create Backend Repository
```bash
# Create a new GitHub repository named 'resume_back'
# Make it public
# Push the backend code to this repository
```

### 2. Set Up GitHub Secrets

In your GitHub repository settings, add these secrets to the "prod" environment:

#### AWS Credentials
- `AWS_ACCESS_KEY_ID`: Your AWS access key
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret key

#### EC2 Connection
- `BACKEND_EC2_HOST`: `3.134.103.103`
- `BACKEND_EC2_SSH_KEY`: Your EC2 private key (entire content including BEGIN/END lines)

#### Database Credentials
- `DB_HOST`: Your RDS endpoint
- `DB_PORT`: `5432`
- `DB_USER`: Your database username
- `DB_PASSWORD`: Your database password
- `DB_NAME`: Your database name

#### JWT Configuration
- `JWT_SECRET`: A strong secret key for JWT tokens

#### GCP Credentials (New Secure Service Account)
- `GCP_PROJECT_ID`: `inner-melody-461103-f0`
- `GCP_PRIVATE_KEY_ID`: `42bbb663cfb32af63519216308cec77e17bfd691`
- `GCP_PRIVATE_KEY`: The entire private key from your JSON file
- `GCP_CLIENT_EMAIL`: `airesume-504@inner-melody-461103-f0.iam.gserviceaccount.com`
- `GCP_CLIENT_ID`: `110823767453707251651`
- `GCP_CLIENT_X509_CERT_URL`: `https://www.googleapis.com/robot/v1/metadata/x509/airesume-504%40inner-melody-461103-f0.iam.gserviceaccount.com`

#### API Keys
- `GEMINI_API_KEY`: Your Gemini API key

### 3. EC2 Security Group Configuration

Add these inbound rules to your EC2 security group:

| Type | Port | Source | Description |
|------|------|--------|-------------|
| SSH | 22 | Your IP | SSH access |
| Custom TCP | 8081 | 0.0.0.0/0 | Backend API |

### 4. Test Local Setup

1. **Create `.env` file** from `env.template`:
   ```bash
   cp env.template .env
   ```

2. **Update database credentials** in `.env` file

3. **Test locally**:
   ```bash
   go run main.go
   ```

### 5. Deploy to GitHub

1. **Push code** to the `main` branch
2. **GitHub Actions** will automatically deploy to EC2
3. **Check deployment status** in GitHub Actions

### 6. Test Backend API

Test the backend API:
```bash
curl http://3.134.103.103:8081/api/auth/login
```

## 🔒 Security Features Implemented

- ✅ **No credentials in Git** (all in environment variables)
- ✅ **Secure service account** with minimal permissions
- ✅ **Environment-based deployment** with GitHub secrets
- ✅ **Comprehensive logging** for security monitoring
- ✅ **Health checks** for service monitoring

## 🛠️ Troubleshooting

### Check Container Status
```bash
docker ps
docker logs ai-resume-backend-backend-1
```

### Check Environment Variables
```bash
docker exec ai-resume-backend-backend-1 env | grep GCP
```

### Check Security Group
Ensure port 8081 is open in your EC2 security group.

## 📋 API Endpoints

- **Health Check**: `GET /api/auth/login`
- **Register**: `POST /api/auth/register`
- **Login**: `POST /api/auth/login`
- **Generate Resume**: `POST /api/resume/generate`
- **Generate PDF**: `POST /api/resume/generate-pdf`

## 🔗 Frontend Integration

Update your frontend to point to the backend:
```javascript
REACT_APP_API_URL=http://3.134.103.103:8081
```

## 🎯 Success Criteria

Your backend is successfully deployed when:
1. ✅ **GitHub Actions** completes successfully
2. ✅ **Container is running** on EC2
3. ✅ **API responds** to health check
4. ✅ **Frontend can connect** to backend
5. ✅ **No credentials** are exposed in Git

**Ready to deploy! 🚀** 