# Registration Test Guide

## Test Registration Without Name

```bash
curl -X POST http://localhost:8081/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

**Expected Response:**
```json
{
  "success": true,
  "message": "User registered successfully",
  "user": "test@example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Test Registration With Name

```bash
curl -X POST http://localhost:8081/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test2@example.com",
    "password": "password123",
    "name": "Test User"
  }'
```

**Expected Response:**
```json
{
  "success": true,
  "message": "User registered successfully",
  "user": "test2@example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Test Login

```bash
curl -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

**Expected Response:**
```json
{
  "success": true,
  "message": "Login successful",
  "user": "test@example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Test Protected Endpoint

```bash
curl -X GET http://localhost:8081/api/user/profile \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

**Expected Response:**
```json
{
  "success": true,
  "profile": {
    "id": 1,
    "email": "test@example.com",
    "name": ""
  }
}
```

## Common Issues and Solutions

### 1. Database Connection Issues
- Make sure PostgreSQL is running
- Check your `.env` file has correct database credentials
- Ensure database user has proper permissions

### 2. Validation Errors
- Email must be valid format
- Password must be at least 6 characters
- Name is optional

### 3. Duplicate Email
- Each email can only be registered once
- Use a different email for testing

### 4. JWT Token Issues
- Tokens expire after 24 hours by default
- Make sure to include "Bearer " prefix in Authorization header 