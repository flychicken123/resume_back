# Security Guide - Credential Management

## 🚨 CRITICAL: GCP Credentials Leaked

**IMMEDIATE ACTIONS REQUIRED:**

### 1. Revoke Compromised Credentials
- Go to [Google Cloud Console](https://console.cloud.google.com/)
- Navigate to **IAM & Admin > Service Accounts**
- Find: `airesume@inner-melody-461103-f0.iam.gserviceaccount.com`
- **DELETE** this service account immediately
- Create a new service account with minimal permissions

### 2. Secure Credential Storage

**NEVER commit credentials to Git repositories!**

#### Option A: Environment Variables (Recommended)
```env
GCP_PROJECT_ID=your_new_project_id
GCP_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nYour new private key\n-----END PRIVATE KEY-----\n"
GCP_CLIENT_EMAIL=your_new_service_account@project.iam.gserviceaccount.com
```

#### Option B: Secure File Storage
- Store credentials in a secure location (not in Git)
- Use environment variables to reference the file path
- Add credential files to `.gitignore`

### 3. Update Application Code

Update your Go application to use environment variables instead of JSON files:

```go
// Instead of loading from JSON file
// Use environment variables
gcpProjectID := os.Getenv("GCP_PROJECT_ID")
gcpPrivateKey := os.Getenv("GCP_PRIVATE_KEY")
gcpClientEmail := os.Getenv("GCP_CLIENT_EMAIL")
```

### 4. Production Deployment

For production (EC2), set environment variables securely:

```bash
# On EC2 instance
export GCP_PROJECT_ID="your_project_id"
export GCP_PRIVATE_KEY="your_private_key"
export GCP_CLIENT_EMAIL="your_service_account_email"
```

### 5. GitHub Secrets

Add GCP credentials to GitHub repository secrets:
- `GCP_PROJECT_ID`
- `GCP_PRIVATE_KEY`
- `GCP_CLIENT_EMAIL`
- `GCP_CLIENT_ID`
- `GCP_CLIENT_X509_CERT_URL`

### 6. Verification

After implementing these changes:
1. ✅ **Test locally** with environment variables
2. ✅ **Deploy to EC2** with secure credentials
3. ✅ **Verify no credentials** are in Git history
4. ✅ **Monitor for unauthorized access**

## Best Practices

1. **Never commit credentials** to version control
2. **Use environment variables** for sensitive data
3. **Rotate credentials regularly**
4. **Use minimal permissions** for service accounts
5. **Monitor access logs** for suspicious activity
6. **Use secret management** services in production

## Emergency Contacts

If credentials are compromised:
1. **Immediately revoke** the service account
2. **Check access logs** for unauthorized usage
3. **Create new credentials** with minimal permissions
4. **Update all deployments** with new credentials
5. **Monitor for any billing anomalies** 