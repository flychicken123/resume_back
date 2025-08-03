# Database Troubleshooting Guide

## Error: "必须是表 users 的属主" (Must be the owner of table users)

This error occurs when your database user doesn't have sufficient permissions to create tables.

## Solutions

### Option 1: Grant Permissions to Your Database User

1. **Connect to PostgreSQL as superuser** (usually `postgres`):
   ```bash
   psql -U postgres -d resumeai
   ```

2. **Grant necessary permissions**:
   ```sql
   -- Replace 'your_db_user' with your actual database user
   GRANT CREATE ON SCHEMA public TO your_db_user;
   GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO your_db_user;
   GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO your_db_user;
   ```

3. **Grant future permissions**:
   ```sql
   ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO your_db_user;
   ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO your_db_user;
   ```

### Option 2: Create Tables Manually

1. **Connect to your database**:
   ```bash
   psql -U your_db_user -d resumeai
   ```

2. **Run the setup script**:
   ```bash
   psql -U your_db_user -d resumeai -f setup_database.sql
   ```

### Option 3: Use a Different Database User

1. **Create a new database user with full permissions**:
   ```sql
   -- Connect as postgres superuser
   CREATE USER resumeai_user WITH PASSWORD 'your_password';
   GRANT ALL PRIVILEGES ON DATABASE resumeai TO resumeai_user;
   ```

2. **Update your .env file**:
   ```bash
   DB_USER=resumeai_user
   DB_PASSWORD=your_password
   ```

### Option 4: Use PostgreSQL Superuser (Development Only)

For development, you can temporarily use the postgres superuser:

```bash
# In your .env file
DB_USER=postgres
DB_PASSWORD=your_postgres_password
```

**Warning**: Never use superuser in production!

## Common PostgreSQL Permission Commands

### Check Current User
```sql
SELECT current_user;
```

### Check User Permissions
```sql
SELECT grantee, privilege_type 
FROM information_schema.role_table_grants 
WHERE table_name = 'users';
```

### List All Users
```sql
SELECT usename FROM pg_user;
```

### Grant All Permissions to User
```sql
-- Replace 'username' with your actual username
GRANT ALL PRIVILEGES ON DATABASE resumeai TO username;
GRANT ALL PRIVILEGES ON SCHEMA public TO username;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO username;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO username;
```

## Environment Configuration

Make sure your `.env` file has the correct database settings:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=your_database_user
DB_PASSWORD=your_database_password
DB_NAME=resumeai
DB_SSLMODE=disable
```

## Testing Database Connection

You can test your database connection manually:

```bash
psql -h localhost -p 5432 -U your_db_user -d resumeai
```

If this works, your connection parameters are correct.

## Application Startup

After fixing permissions, restart your application:

```bash
go run main.go
```

The application should now start without permission errors.

## Production Considerations

For production environments:

1. **Use dedicated database users** with minimal required permissions
2. **Never use superuser accounts** for application connections
3. **Set up proper database backups**
4. **Use connection pooling** for better performance
5. **Monitor database performance** and logs

## Still Having Issues?

If you're still experiencing problems:

1. **Check PostgreSQL logs** for detailed error messages
2. **Verify PostgreSQL is running** and accessible
3. **Test connection** with `psql` command line tool
4. **Check firewall settings** if connecting to remote database
5. **Verify database exists** and is accessible to your user 