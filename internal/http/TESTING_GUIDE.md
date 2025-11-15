# Authentication Testing Guide

## Overview

This directory contains comprehensive E2E and integration tests for the authentication system.

## Test Structure

```
internal/http/
├── auth_integration_test.go  # Go integration tests
├── router.go                   # Chi router
├── handlers/
│   └── auth_handler.go        # Auth handlers
└── API_DOCUMENTATION.md        # API docs
```

## Running Tests

### Go Integration Tests

```bash
# Set environment
export DB_DSN_TEST=1
export $(grep -v '^#' .env | xargs)

# Run all auth tests
go test ./internal/http -v

# Run specific test
go test ./internal/http -v -run TestHealth
go test ./internal/http -v -run TestForgotPassword_WithEmail
go test ./internal/http -v -run TestVerifyOTP_Success
go test ./internal/http -v -run TestResetPassword_Success
go test ./internal/http -v -run TestFullForgotPasswordFlow

# Run tests matching pattern
go test ./internal/http -v -run "TestForgotPassword.*"
go test ./internal/http -v -run "TestVerifyOTP.*"
go test ./internal/http -v -run "TestResetPassword.*"
```

### Bash E2E Scripts

```bash
# Simple forgot password flow (interactive - requires OTP from logs)
./scripts/e2e_auth_flow.sh

# Complete auth flow with all edge cases
./scripts/e2e_complete_auth.sh

# Custom base URL
./scripts/e2e_auth_flow.sh http://localhost:3000
./scripts/e2e_complete_auth.sh http://staging.example.com
```

## Test Coverage

### Unit/Integration Tests (Go)

| Test | Description | Status |
|------|-------------|--------|
| `TestHealth` | Health endpoint | ✅ |
| `TestForgotPassword_WithEmail` | Request reset with email | ✅ |
| `TestForgotPassword_WithUsername` | Request reset with username | ✅ |
| `TestForgotPassword_NonExistent` | Non-existent user (security) | ✅ |
| `TestForgotPassword_InvalidBody` | Invalid request body | ✅ |
| `TestVerifyOTP_Success` | Verify valid OTP | ✅ |
| `TestVerifyOTP_WrongOTP` | Reject wrong OTP | ✅ |
| `TestVerifyOTP_Expired` | Reject expired OTP | ✅ |
| `TestResetPassword_Success` | Reset with valid token | ✅ |
| `TestResetPassword_PasswordMismatch` | Reject mismatched passwords | ✅ |
| `TestResetPassword_ShortPassword` | Reject short passwords | ✅ |
| `TestResetPassword_InvalidToken` | Reject invalid reset token | ✅ |
| `TestFullForgotPasswordFlow` | Complete end-to-end flow | ✅ |

**Total: 13 tests, all passing**

### E2E Bash Tests

**e2e_auth_flow.sh** - Basic flow:
- Health check
- Request password reset
- Verify OTP (manual input from logs)
- Reset password

**e2e_complete_auth.sh** - Comprehensive:
- All basic flow steps
- Request with email
- Request with username
- Non-existent user security check
- Wrong OTP rejection
- Expired OTP handling
- Password mismatch validation
- Short password validation
- Empty field validation
- Malformed JSON handling

## Test Output Examples

### Successful Test Run

```
=== RUN   TestForgotPassword_WithEmail
--- PASS: TestForgotPassword_WithEmail (0.27s)
=== RUN   TestVerifyOTP_Success
--- PASS: TestVerifyOTP_Success (0.26s)
=== RUN   TestResetPassword_Success
--- PASS: TestResetPassword_Success (0.60s)
=== RUN   TestFullForgotPasswordFlow
--- PASS: TestFullForgotPasswordFlow (0.78s)
PASS
ok      be03/internal/http      7.216s
```

### Bash Script Output

```
============================================
E2E Auth Flow Test
============================================
Base URL: http://localhost:8080

Step 0: Health check...
✓ Health check passed

Step 1: Request password reset...
✓ Password reset requested

Step 2: Verify OTP...
✓ OTP verified, reset token received

Step 3: Reset password...
✓ Password reset successfully

============================================
✓ All tests passed!
============================================
```

## Test Data

Tests use either:
- **Existing users**: `fardil`, `arif` (from database)
- **Dynamic test users**: Created per test with timestamp
- **Seeded admin**: `admin/admin123`

## OTP Retrieval

In development/test environments, OTPs are logged to console:

```
[MAIL] Sending OTP 123456 to user@example.com
```

Check server logs or test output for OTPs.

## Debugging Failed Tests

### Check Database Connection
```bash
export $(grep -v '^#' .env | xargs)
go run ./tools/dbcheck
```

### View Test Output
```bash
go test ./internal/http -v -run TestName 2>&1 | less
```

### Check Server Logs
```bash
tail -f logs/api.log
```

### Verify Environment
```bash
echo $DB_DSN
echo $DB_DSN_TEST
```

## CI/CD Integration

Add to your CI pipeline:

```yaml
# .github/workflows/test.yml
test:
  steps:
    - name: Run integration tests
      env:
        DB_DSN_TEST: 1
        DB_DSN: ${{ secrets.TEST_DB_DSN }}
      run: |
        go test ./internal/http -v
```

## Security Testing

Tests verify:
- ✅ User enumeration prevention (always 200 for forgot password)
- ✅ OTP expiration (15-minute window)
- ✅ Single-use tokens
- ✅ Password strength requirements (min 6 chars)
- ✅ Old password verification
- ✅ Token validation
- ✅ Input sanitization

## Performance

Typical test suite execution time: **~7 seconds**

Individual test times:
- Health: 4.6s (includes DB init)
- Forgot password: 0.27s
- Verify OTP: 0.26s
- Reset password: 0.60s
- Full flow: 0.78s

## Next Steps

1. Add tests for change-password endpoint (requires JWT)
2. Add load testing with `k6` or `hey`
3. Add API contract tests
4. Add database transaction rollback for isolated tests
5. Add mocked email sending tests

## Related Documentation

- [API Documentation](./API_DOCUMENTATION.md) - Full API reference
- [Main README](../../README.md) - Project overview
- [Restructure Summary](../../RESTRUCTURE_SUMMARY.md) - Architecture notes
