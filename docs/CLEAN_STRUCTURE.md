# Clean Project Structure

## ✅ Current Organized Structure

```
be03/
├── cmd/                      # Application entrypoints
│   └── api/                  # Main API server
│       └── main.go
├── internal/                 # Private application code
│   ├── core/                 # Business logic
│   │   └── auth/             # Authentication services
│   │       └── service.go
│   ├── http/                 # HTTP layer
│   │   ├── router.go         # Route definitions
│   │   └── handlers/         # HTTP handlers
│   │       ├── auth_handler.go
│   │       ├── user_handler.go
│   │       └── helpers.go
│   └── persistence/          # Data layer
│       └── db.go
├── models/                   # Data models (shared)
│   ├── user.go
│   ├── profile.go
│   ├── role.go
│   ├── catatan.go
│   ├── upload.go
│   ├── refresh_token.go
│   └── password_reset.go
├── pkg/                      # Public libraries
│   ├── ocr/                  # OCR processing
│   └── mail/                 # Email service
├── bin/                      # Compiled binaries
│   └── api_new               # New API server binary
├── backup/                   # Old files backup
├── scripts/                  # Utility scripts
├── migration/                # Database migrations
├── public/                   # Public uploads
├── logs/                     # Log files
└── Root files:
    ├── go.mod, go.sum        # Go modules
    ├── .env*                 # Configuration
    ├── Dockerfile            # Container config
    ├── docker-compose.yml    # Docker compose
    └── README*.md            # Documentation
```

## 🔧 Running the Application

### New Structure (Recommended)
```bash
# Compile
go build -o bin/api_new ./cmd/api

# Run
./bin/api_new

# Or with custom port
PORT=8082 ./bin/api_new
```

### Available Endpoints

#### Public Endpoints
- `GET /health` - Health check
- `POST /register` - User registration
- `POST /login` - User login (returns JWT token)
- `POST /refresh` - Refresh access token
- `POST /revoke` - Revoke refresh token

#### Password Reset Flow
- `POST /auth/forgot-password` - Request password reset (sends OTP)
- `POST /auth/forgot-password/verify` - Verify OTP (returns reset token)
- `POST /auth/reset-password` - Reset password with token

#### Protected Endpoints (require JWT)
- `GET /me` - Get current user info
- `PUT /auth/change-password` - Change password (authenticated)

### Testing Login

```bash
# Login with username
curl -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"fardil","password":"admin123"}'

# Response includes JWT token:
{
  "access_token": "eyJhbGc...",
  "refresh_token": "a282fc...",
  "token_type": "bearer",
  "expires_in": 900
}

# Use token for protected endpoints
curl -X GET http://localhost:8081/me \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## 📁 What Was Cleaned

### Moved to backup/
- `auth.go`, `db.go`, `handlers.go` - Duplicate of internal/ code
- `main.go`, `main_new.go` - Old entrypoints
- `models.go`, `login.go` - Stub files
- `be03`, `be03_app` - Old binaries

### Moved to bin/
- All utility binaries: `create_user`, `ocr_dump`, `process_keu`, etc.

### Removed
- Temporary CSV backups
- Build artifacts (`build_err.txt`)
- Runtime PID files

### To Be Migrated (TODO)
- `services/auth_service.go` - Can be removed (duplicate of internal/core/auth)
- `main.go` - Root main.go (legacy, should point to cmd/api or be removed)
- Integration tests - Need update to use new structure

## 🎯 Benefits of Clean Structure

1. **Scalability**: Clear separation of concerns
2. **Maintainability**: Easy to find and modify code
3. **Testability**: Each layer can be tested independently
4. **Onboarding**: New developers understand structure quickly
5. **Deployment**: Single binary from cmd/api
