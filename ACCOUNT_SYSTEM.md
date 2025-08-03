# Account System Documentation

## Overview
The AI Resume Builder now includes a complete user account system with authentication, authorization, and user data management.

## Features

### Authentication
- **User Registration**: Create new accounts with email, password, and name
- **User Login**: Authenticate with email and password
- **JWT Tokens**: Secure session management with JSON Web Tokens
- **Password Security**: Bcrypt hashing for password storage
- **Logout**: Client-side token removal

### User Profile Management
- **Get Profile**: Retrieve current user information
- **Update Profile**: Modify user name and basic information
- **Change Password**: Secure password change with current password verification

### Data Persistence
- **Save User Data**: Store resume data per user
- **Load User Data**: Retrieve user-specific resume data
- **Database Integration**: PostgreSQL with proper indexing and relationships

## API Endpoints

### Public Endpoints (No Authentication Required)
```
POST /api/auth/register - Register new user
POST /api/auth/login    - Login user
POST /api/auth/logout   - Logout user
```

### Protected Endpoints (Authentication Required)
```
GET    /api/user/profile        - Get user profile
PUT    /api/user/profile        - Update user profile
POST   /api/user/change-password - Change password
POST   /api/user/save           - Save user data
GET    /api/user/load           - Load user data
POST   /api/resume/generate     - Generate resume
POST   /api/resume/generate-pdf - Generate PDF resume
POST   /api/resume/parse        - Parse resume
POST   /api/experience/optimize - Optimize experience
```

## Database Schema

### Users Table
```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Resumes Table
```sql
CREATE TABLE resumes (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(100),
    summary TEXT,
    skills TEXT,
    selected_format VARCHAR(50) DEFAULT 'temp1',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Authentication Flow

1. **Registration**:
   - User provides email, password, and name
   - System checks for existing email
   - Password is hashed with bcrypt
   - User record is created
   - JWT token is generated and returned

2. **Login**:
   - User provides email and password
   - System verifies email exists
   - Password is compared with stored hash
   - JWT token is generated and returned

3. **Protected Routes**:
   - Client includes JWT token in Authorization header
   - Server validates token and extracts user information
   - User context is set for the request

## Security Features

- **Password Hashing**: Bcrypt with default cost
- **JWT Tokens**: Secure session management
- **Input Validation**: Request data validation with Gin
- **SQL Injection Prevention**: Parameterized queries
- **CORS Support**: Cross-origin resource sharing enabled

## Configuration

Create a `.env` file based on `env.example`:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password_here
DB_NAME=resumeai
DB_SSLMODE=disable

# Application Configuration
PORT=8081
ENVIRONMENT=development

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRATION_HOURS=24
```

## Usage Examples

### Register a User
```bash
curl -X POST http://localhost:8081/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password123",
    "name": "John Doe"
  }'
```

### Login
```bash
curl -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password123"
  }'
```

### Access Protected Route
```bash
curl -X GET http://localhost:8081/api/user/profile \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Dependencies

- `github.com/gin-gonic/gin` - Web framework
- `github.com/golang-jwt/jwt/v5` - JWT handling
- `golang.org/x/crypto/bcrypt` - Password hashing
- `github.com/lib/pq` - PostgreSQL driver
- `github.com/joho/godotenv` - Environment variable loading

## Setup Instructions

1. **Install Dependencies**:
   ```bash
   go mod tidy
   ```

2. **Set up PostgreSQL Database**:
   - Create database named `resumeai`
   - Update `.env` file with database credentials

3. **Run the Application**:
   ```bash
   go run main.go
   ```

4. **Initialize Database** (automatic):
   - Tables are created automatically on startup
   - No manual schema setup required

## Error Handling

The system includes comprehensive error handling:
- Input validation errors
- Database connection errors
- Authentication failures
- Authorization errors
- Server errors with appropriate HTTP status codes

## Future Enhancements

- Email verification
- Password reset functionality
- Social login integration
- Multi-factor authentication
- User roles and permissions
- Account deletion
- Data export functionality 