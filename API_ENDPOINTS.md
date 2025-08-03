# API Endpoints Documentation

## Overview
The AI Resume Builder API now has a clear separation between public and protected endpoints. All resume-related operations require authentication.

## Public Endpoints (No Authentication Required)

### Authentication
```
POST /api/auth/register - Register new user
POST /api/auth/login    - Login user  
POST /api/auth/logout   - Logout user
```

## Protected Endpoints (Authentication Required)

All protected endpoints require a valid JWT token in the Authorization header:
```
Authorization: Bearer YOUR_JWT_TOKEN
```

### User Management
```
GET    /api/user/profile        - Get user profile
PUT    /api/user/profile        - Update user profile
POST   /api/user/change-password - Change password
POST   /api/user/save           - Save user data
GET    /api/user/load           - Load user data
```

### Resume Operations
```
POST   /api/resume/generate     - Generate resume
POST   /api/resume/generate-pdf - Generate PDF resume
POST   /api/resume/parse        - Parse resume
POST   /api/experience/optimize - Optimize experience
```

## Authentication Flow

### 1. Register a New User
```bash
curl -X POST http://localhost:8081/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password123",
    "name": "John Doe"
  }'
```

**Note:** The `name` field is optional. If not provided, it will be stored as an empty string.

**Response:**
```json
{
  "success": true,
  "message": "User registered successfully",
  "user": "user@example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### 2. Login
```bash
curl -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password123"
  }'
```

**Response:**
```json
{
  "success": true,
  "message": "Login successful",
  "user": "user@example.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### 3. Access Protected Endpoints
```bash
curl -X GET http://localhost:8081/api/user/profile \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Error Responses

### Authentication Errors
```json
{
  "success": false,
  "message": "Invalid email or password"
}
```

### Authorization Errors
```json
{
  "success": false,
  "message": "Authorization header required"
}
```

### Validation Errors
```json
{
  "success": false,
  "message": "Invalid request data: email is required"
}
```

## Important Notes

1. **All resume operations now require authentication** - This ensures data security and user-specific data management
2. **JWT tokens expire** - Default expiration is 24 hours, configurable via `JWT_EXPIRATION_HOURS`
3. **Database persistence** - User data is automatically saved and loaded per user
4. **CORS enabled** - Cross-origin requests are supported for frontend integration

## Migration from Legacy Endpoints

If you were using the old public endpoints, you'll need to:

1. **Implement authentication** in your frontend
2. **Include JWT tokens** in all API requests
3. **Update endpoint URLs** to use the new protected routes

### Old vs New Endpoints

| Old (Public) | New (Protected) |
|---------------|-----------------|
| `POST /api/resume/generate` | `POST /api/resume/generate` |
| `POST /api/resume/generate-pdf` | `POST /api/resume/generate-pdf` |
| `POST /api/resume/parse` | `POST /api/resume/parse` |
| `POST /api/experience/optimize` | `POST /api/experience/optimize` |
| `POST /api/user/save` | `POST /api/user/save` |
| `GET /api/user/load` | `GET /api/user/load` |

The endpoint URLs remain the same, but now require authentication headers. 