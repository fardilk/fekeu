# API Endpoint Documentation

## Base URL
Local development: `http://localhost:8080` (default) or configured via `PORT` env var

Production / VPS: `http://103.172.204.34:8080` (replace port if you run the service on a different port)

Important production notes:
- Use HTTPS in production (terminate TLS at a reverse proxy like nginx or a load balancer).
- Ensure the server is reachable on the chosen port and firewall rules allow inbound traffic.
- Set `PORT` (or `SERVER_PORT`) in your service environment or your Docker configuration to the port you want the server to listen on.

---

## Health Check

### GET `/health`
Check if the API server is running.

**Response:**
```json
{
  "status": "ok"
}
```

**Status Codes:**
- `200 OK` - Server is healthy

---

## Authentication Endpoints

### POST `/auth/forgot-password`
Request a password reset OTP to be sent to the user's email.

**Request Body:**
```json
{
  "identifier": "fardil.khalidi@gmail.com"  // email or username
}
```

**Response:**
```json
{
  "message": "If the account exists, an OTP has been sent"
}
```

**Status Codes:**
- `200 OK` - Request processed (always returns 200 to prevent user enumeration)
- `400 Bad Request` - Invalid request body

**Notes:**
- Always returns 200 even if user doesn't exist (security feature)
- OTP is logged to server logs in development (check `[MAIL]` entries)
- OTP expires after 15 minutes
- Lookup works with email (from profile) or username

---

### POST `/auth/forgot-password/verify`
Verify the OTP and receive a reset token.

**Request Body:**
```json
{
  "email": "fardil.khalidi@gmail.com",
  "otp": "123456"
}
```

**Response:**
```json
{
  "reset_token": "a1b2c3d4e5f6..."
}
```

**Status Codes:**
- `200 OK` - OTP verified, reset token issued
- `400 Bad Request` - Invalid OTP or expired
- `400 Bad Request` - Missing email or OTP

**Notes:**
- OTP must match exactly (6 digits)
- OTP must not be expired
- OTP must not have been used already
- Reset token is valid for the same duration as OTP (15 minutes)

---

### POST `/auth/reset-password`
Reset password using the reset token from OTP verification.

**Request Body:**
```json
{
  "reset_token": "a1b2c3d4e5f6...",
  "new_password": "newSecurePass123",
  "new_password_confirm": "newSecurePass123"
}
```

**Response:**
```json
{
  "message": "password updated"
}
```

**Status Codes:**
- `200 OK` - Password successfully reset
- `400 Bad Request` - Invalid reset token, expired, or passwords don't match
- `400 Bad Request` - Password too short (min 6 chars)

**Password Requirements:**
- Minimum 6 characters
- Must match confirmation
- Automatically hashed with bcrypt

**Notes:**
- Reset token is single-use
- Token expires after same duration as OTP (15 minutes)
- After successful reset, user should login with new password

---

### PUT `/auth/change-password`
Change password for authenticated user (requires JWT token).

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request Body:**
```json
{
  "old_password": "currentPassword123",
  "new_password": "newPassword456",
  "new_password_confirm": "newPassword456"
}
```

**Response:**
```json
{
  "message": "password changed"
}
```

**Status Codes:**
- `200 OK` - Password changed successfully
- `400 Bad Request` - Old password incorrect
- `400 Bad Request` - New passwords don't match or too short
- `401 Unauthorized` - No valid JWT token provided
- `500 Internal Server Error` - Database error

**Notes:**
- Requires valid JWT token from login
- Old password must match current password
- New password minimum 6 characters
- Automatically hashed with bcrypt

---

## Error Response Format

All error responses follow this format:

```json
{
  "error": "error_code",
  "message": "Human readable error message"
}
```

**Common Error Codes:**
- `invalid_body` - Request body is malformed or missing required fields
- `invalid_otp` - OTP is incorrect or expired
- `reset_failed` - Password reset failed (token invalid/expired)
- `invalid_old_password` - Current password is incorrect
- `change_failed` - Password change failed
- `unauthorized` - Missing or invalid JWT token

---

## Testing Flow

### Forgot Password Flow
1. POST `/auth/forgot-password` with email or username
2. Check server logs for OTP (format: `[MAIL] Sending OTP 123456 to email@example.com`)
3. POST `/auth/forgot-password/verify` with email and OTP
4. Receive `reset_token` in response
5. POST `/auth/reset-password` with reset_token and new password
6. Password is updated

### Change Password Flow (Authenticated)
1. Login to get JWT token (via existing `/login` endpoint)
2. PUT `/auth/change-password` with Authorization header
3. Provide old password and new password (twice)
4. Password is updated

---

## Database Models

### PasswordReset
```go
type PasswordReset struct {
    ID         uint
    CreatedAt  time.Time
    UpdatedAt  time.Time
    UserID     uint
    Email      string
    OTP        string      // 6-digit code
    ExpiresAt  time.Time   // 15 minutes from creation
    Used       bool
    UsedAt     *time.Time
    ResetToken string      // Issued after OTP verification
}
```

---

## Security Features

1. **User Enumeration Prevention**: Forgot password always returns 200
2. **OTP Expiration**: 15-minute window for OTP usage
3. **Single-Use Tokens**: Reset tokens and OTPs can only be used once
4. **Password Hashing**: bcrypt with default cost (10)
5. **Token Validation**: Reset token must be valid and not expired
6. **Old Password Verification**: Change password requires correct old password

---

## Environment Variables

- `DB_DSN` - PostgreSQL connection string
- `PORT` or `SERVER_PORT` - HTTP server port (default: 8080)
- `JWT_SECRET` - Secret for JWT signing (for change-password auth)
- `UPLOAD_BASE` - Base directory for uploads (default: uploads)

---

## Development Notes

### Email Sending (Development)
Current implementation uses dummy mailer that logs OTP to console:
```
[MAIL] Sending OTP 123456 to user@example.com
```

For production, replace `pkg/mail/mailer.go` with real SMTP or email service integration.

### Database Migrations
Auto-migration runs on startup. To run manually:
```bash
./bin/api migrate
```

### Testing
```bash
# Unit tests
go test ./internal/http/...

# Integration tests (requires DB)
DB_DSN_TEST=1 DB_DSN="..." go test ./internal/http -v

# E2E bash scripts
./scripts/e2e_auth_flow.sh
```
