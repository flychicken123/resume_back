# Database Connection Troubleshooting Guide

## Current Issue: PostgreSQL Connection Error

**Error Message:**
```
pq: no pg_hba.conf entry for host "172.31.32.78", user "postgres", database "resumeai", no encryption
```

## Root Cause Analysis

The error indicates that your PostgreSQL RDS instance is rejecting connections from your EC2 instance because:

1. **Database Name Mismatch**: Error shows `resumeai` but config shows `airesume`
2. **Security Group Issues**: RDS security group doesn't allow connections from EC2
3. **User Authentication**: Connection using `postgres` user but might need different credentials
4. **Network Configuration**: EC2 and RDS might be in different subnets/VPCs

## Step-by-Step Fix

### 1. Verify RDS Configuration

First, check your RDS instance details in AWS Console:

```bash
# Get your RDS endpoint
aws rds describe-db-instances --query 'DBInstances[*].[DBInstanceIdentifier,Endpoint.Address,Endpoint.Port,DBName]' --output table
```

### 2. Update Environment Variables

Create a proper `.env` file on your EC2 instance:

```bash
# SSH into your EC2 instance
ssh -i your-key.pem ubuntu@your-ec2-public-ip

# Create .env file
cat > .env << EOF
# Database Configuration
DB_HOST=airesume.czy822q0a52j.us-east-2.rds.amazonaws.com
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=lichking1991
DB_NAME=airesume
DB_SSLMODE=disable

# Application Configuration
PORT=8081
ENVIRONMENT=production
JWT_SECRET=your-super-secure-jwt-secret-key-here
EOF
```

### 3. Check RDS Security Group

In AWS Console → RDS → Your Database → Security:

1. **Security Group Rules**:
   - Type: PostgreSQL
   - Port: 5432
   - Source: Your EC2 security group ID or EC2 private IP

2. **Add Security Group Rule**:
   ```
   Type: PostgreSQL
   Protocol: TCP
   Port: 5432
   Source: sg-xxxxxxxxx (your EC2 security group)
   ```

### 4. Test Database Connection

```bash
# Test from EC2 instance
psql "postgresql://postgres:lichking1991@airesume.czy822q0a52j.us-east-2.rds.amazonaws.com:5432/airesume"

# Or test with Docker
docker run --rm -it postgres:13 psql "postgresql://postgres:lichking1991@airesume.czy822q0a52j.us-east-2.rds.amazonaws.com:5432/airesume"
```

### 5. Update Docker Compose Configuration

Update your `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  backend:
    build: ./back
    ports:
      - "8081:8081"
    volumes:
      - ./back/static:/app/static
    environment:
      - DB_HOST=airesume.czy822q0a52j.us-east-2.rds.amazonaws.com
      - DB_PORT=5432
      - DB_NAME=airesume
      - DB_USER=postgres
      - DB_PASSWORD=lichking1991
      - DB_SSLMODE=disable
      - PORT=8081
      - JWT_SECRET=${JWT_SECRET:-your-super-secure-jwt-secret-key-here}
    restart: unless-stopped
    networks:
      - ai-resume-network

networks:
  ai-resume-network:
    driver: bridge
```

### 6. Verify VPC and Subnet Configuration

Ensure EC2 and RDS are in the same VPC:

```bash
# Check EC2 VPC
aws ec2 describe-instances --instance-ids i-xxxxxxxxx --query 'Reservations[*].Instances[*].[VpcId,SubnetId]'

# Check RDS VPC
aws rds describe-db-instances --db-instance-identifier airesume --query 'DBInstances[*].[DBSubnetGroup.VpcId]'
```

### 7. Database Schema Setup

If the database exists but tables don't:

```bash
# Connect and create tables
docker exec -it ai-resume-backend-1 psql "postgresql://postgres:lichking1991@airesume.czy822q0a52j.us-east-2.rds.amazonaws.com:5432/airesume" -f /app/database/schema.sql
```

## Quick Fix Commands

### Option 1: Update Environment and Restart

```bash
# On your EC2 instance
cd AIResume

# Create correct .env file
cat > .env << EOF
DB_HOST=airesume.czy822q0a52j.us-east-2.rds.amazonaws.com
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=lichking1991
DB_NAME=airesume
DB_SSLMODE=disable
PORT=8081
JWT_SECRET=your-super-secure-jwt-secret-key-here
EOF

# Restart services
docker-compose -f docker-compose.prod.yml down
docker-compose -f docker-compose.prod.yml up -d

# Check logs
docker-compose -f docker-compose.prod.yml logs -f backend
```

### Option 2: Test Connection First

```bash
# Test database connection
docker run --rm -it postgres:13 psql "postgresql://postgres:lichking1991@airesume.czy822q0a52j.us-east-2.rds.amazonaws.com:5432/airesume" -c "\l"

# If successful, restart your application
docker-compose -f docker-compose.prod.yml restart backend
```

## Common Issues and Solutions

### Issue 1: Database Name Mismatch
- **Problem**: Error shows `resumeai` but config shows `airesume`
- **Solution**: Ensure all configs use the same database name

### Issue 2: Security Group
- **Problem**: RDS not accepting connections from EC2
- **Solution**: Add EC2 security group to RDS security group rules

### Issue 3: User Authentication
- **Problem**: Wrong username/password
- **Solution**: Verify RDS master username and password

### Issue 4: SSL Mode
- **Problem**: SSL configuration mismatch
- **Solution**: Use `DB_SSLMODE=disable` for testing

## Monitoring and Debugging

### Check Application Logs
```bash
docker-compose -f docker-compose.prod.yml logs -f backend
```

### Check Database Connectivity
```bash
# From EC2 instance
telnet airesume.czy822q0a52j.us-east-2.rds.amazonaws.com 5432
```

### Check Security Groups
```bash
# List security groups
aws ec2 describe-security-groups --query 'SecurityGroups[*].[GroupId,GroupName]' --output table
```

## Success Indicators

When fixed, you should see:
```
✅ Database connection successful!
Server starting on port 8081
```

## Next Steps

1. ✅ Fix database connection
2. ✅ Verify application starts
3. ✅ Test API endpoints
4. ✅ Monitor logs for any remaining issues 