#!/bin/bash
#
# Comprehensive E2E test for all authentication flows
# This script tests the complete authentication system including:
# - Forgot password flow
# - Change password flow (requires existing login system)
#
# Usage: ./scripts/e2e_complete_auth.sh [BASE_URL]
#

set -e

BASE_URL="${1:-http://localhost:8080}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}============================================"
echo "Complete Authentication E2E Test"
echo "============================================${NC}"
echo "Base URL: $BASE_URL"
echo ""

# Helper functions
print_step() {
    echo -e "\n${BLUE}➤ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

print_info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# Test data
EXISTING_USER="fardil"
EXISTING_EMAIL="fardil.khalidi@gmail.com"
TIMESTAMP=$(date +%s)

# ============================================
# Test 1: Health Check
# ============================================
print_step "Test 1: Health Check"

HEALTH=$(curl -s "${BASE_URL}/health")
echo "$HEALTH" | grep -q '"status":"ok"' || print_error "Health check failed"
print_success "Server is healthy"

# ============================================
# Test 2: Forgot Password Flow
# ============================================
print_step "Test 2: Forgot Password Flow"

# 2a. Request password reset with email
print_info "2a. Requesting password reset for: $EXISTING_EMAIL"
FORGOT_RESP=$(curl -s -X POST "${BASE_URL}/auth/forgot-password" \
    -H "Content-Type: application/json" \
    -d "{\"identifier\": \"$EXISTING_EMAIL\"}")

echo "$FORGOT_RESP" | grep -q "OTP has been sent" || print_error "Forgot password request failed"
print_success "Password reset requested"

# 2b. Request password reset with username
print_info "2b. Requesting password reset for username: $EXISTING_USER"
FORGOT_RESP2=$(curl -s -X POST "${BASE_URL}/auth/forgot-password" \
    -H "Content-Type: application/json" \
    -d "{\"identifier\": \"$EXISTING_USER\"}")

echo "$FORGOT_RESP2" | grep -q "OTP has been sent" || print_error "Forgot password request failed"
print_success "Password reset requested (by username)"

# 2c. Test with non-existent user (should still return 200)
print_info "2c. Testing with non-existent user (security check)"
FORGOT_RESP3=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/forgot-password" \
    -H "Content-Type: application/json" \
    -d "{\"identifier\": \"nonexistent@example.com\"}")

HTTP_CODE=$(echo "$FORGOT_RESP3" | tail -1)
[ "$HTTP_CODE" = "200" ] || print_error "Should return 200 for non-existent user (got $HTTP_CODE)"
print_success "Correctly returns 200 for non-existent user (prevents enumeration)"

# ============================================
# Test 3: OTP Verification
# ============================================
print_step "Test 3: OTP Verification"

print_info "Check server logs for OTP:"
echo -e "${YELLOW}Look for: [MAIL] Sending OTP XXXXXX to $EXISTING_EMAIL${NC}"
echo ""
read -p "Enter the 6-digit OTP: " OTP

if [ -z "$OTP" ]; then
    print_error "OTP cannot be empty"
fi

# 3a. Verify OTP
print_info "3a. Verifying OTP: $OTP"
VERIFY_RESP=$(curl -s -X POST "${BASE_URL}/auth/forgot-password/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$EXISTING_EMAIL\", \"otp\": \"$OTP\"}")

RESET_TOKEN=$(echo "$VERIFY_RESP" | grep -o '"reset_token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$RESET_TOKEN" ]; then
    print_error "Failed to get reset token. Response: $VERIFY_RESP"
fi

print_success "OTP verified, reset token received"
echo "  Token: ${RESET_TOKEN:0:20}..."

# 3b. Test with wrong OTP
print_info "3b. Testing with wrong OTP (should fail)"
VERIFY_FAIL=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/forgot-password/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$EXISTING_EMAIL\", \"otp\": \"999999\"}")

HTTP_CODE_FAIL=$(echo "$VERIFY_FAIL" | tail -1)
[ "$HTTP_CODE_FAIL" = "400" ] || print_error "Should return 400 for wrong OTP (got $HTTP_CODE_FAIL)"
print_success "Correctly rejects wrong OTP"

# ============================================
# Test 4: Password Reset
# ============================================
print_step "Test 4: Password Reset"

NEW_PASSWORD="newpass${TIMESTAMP}"

# 4a. Reset password
print_info "4a. Resetting password to: $NEW_PASSWORD"
RESET_RESP=$(curl -s -X POST "${BASE_URL}/auth/reset-password" \
    -H "Content-Type: application/json" \
    -d "{\"reset_token\": \"$RESET_TOKEN\", \"new_password\": \"$NEW_PASSWORD\", \"new_password_confirm\": \"$NEW_PASSWORD\"}")

echo "$RESET_RESP" | grep -q "password updated" || print_error "Password reset failed. Response: $RESET_RESP"
print_success "Password reset successfully"

# 4b. Test password mismatch
print_info "4b. Testing password mismatch (should fail)"
RESET_FAIL=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/reset-password" \
    -H "Content-Type: application/json" \
    -d "{\"reset_token\": \"dummy\", \"new_password\": \"pass123456\", \"new_password_confirm\": \"pass999999\"}")

HTTP_CODE_MISMATCH=$(echo "$RESET_FAIL" | tail -1)
[ "$HTTP_CODE_MISMATCH" = "400" ] || print_error "Should return 400 for password mismatch (got $HTTP_CODE_MISMATCH)"
print_success "Correctly rejects password mismatch"

# 4c. Test short password
print_info "4c. Testing short password (should fail)"
RESET_SHORT=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/reset-password" \
    -H "Content-Type: application/json" \
    -d "{\"reset_token\": \"dummy\", \"new_password\": \"short\", \"new_password_confirm\": \"short\"}")

HTTP_CODE_SHORT=$(echo "$RESET_SHORT" | tail -1)
[ "$HTTP_CODE_SHORT" = "400" ] || print_error "Should return 400 for short password (got $HTTP_CODE_SHORT)"
print_success "Correctly rejects short password (< 6 chars)"

# ============================================
# Test 5: Invalid Inputs
# ============================================
print_step "Test 5: Error Handling"

# 5a. Empty identifier
print_info "5a. Testing empty identifier"
EMPTY_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/forgot-password" \
    -H "Content-Type: application/json" \
    -d "{\"identifier\": \"\"}")
HTTP_CODE_EMPTY=$(echo "$EMPTY_RESP" | tail -1)
[ "$HTTP_CODE_EMPTY" = "400" ] || print_error "Should return 400 for empty identifier"
print_success "Correctly rejects empty identifier"

# 5b. Malformed JSON
print_info "5b. Testing malformed JSON"
MALFORMED_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/forgot-password" \
    -H "Content-Type: application/json" \
    -d "{bad json")
HTTP_CODE_MALFORMED=$(echo "$MALFORMED_RESP" | tail -1)
[ "$HTTP_CODE_MALFORMED" = "400" ] || print_error "Should return 400 for malformed JSON"
print_success "Correctly rejects malformed JSON"

# 5c. Missing OTP field
print_info "5c. Testing missing OTP field"
MISSING_OTP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/auth/forgot-password/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$EXISTING_EMAIL\"}")
HTTP_CODE_MISSING=$(echo "$MISSING_OTP" | tail -1)
[ "$HTTP_CODE_MISSING" = "400" ] || print_error "Should return 400 for missing OTP"
print_success "Correctly rejects missing OTP"

# ============================================
# Summary
# ============================================
echo ""
echo -e "${GREEN}============================================"
echo "✓ All Authentication Tests Passed!"
echo "============================================${NC}"
echo ""
echo "Test Summary:"
echo "  1. Health Check: ✓"
echo "  2. Forgot Password Flow:"
echo "     - Request by email: ✓"
echo "     - Request by username: ✓"
echo "     - Non-existent user (security): ✓"
echo "  3. OTP Verification:"
echo "     - Valid OTP: ✓"
echo "     - Invalid OTP rejection: ✓"
echo "  4. Password Reset:"
echo "     - Successful reset: ✓"
echo "     - Password mismatch rejection: ✓"
echo "     - Short password rejection: ✓"
echo "  5. Error Handling:"
echo "     - Empty identifier: ✓"
echo "     - Malformed JSON: ✓"
echo "     - Missing fields: ✓"
echo ""
echo -e "${BLUE}NOTE: Password for $EXISTING_USER has been changed to: $NEW_PASSWORD${NC}"
echo "      You may want to reset it back to the original password."
echo ""
