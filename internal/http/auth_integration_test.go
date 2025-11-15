package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"be03/internal/persistence"
	"be03/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var testDB *gorm.DB

// setupTestDB initializes a test database connection
func setupTestDB(t *testing.T) *gorm.DB {
	if os.Getenv("DB_DSN_TEST") != "1" {
		t.Skip("integration tests disabled; set DB_DSN_TEST=1 to enable")
	}

	if testDB != nil {
		return testDB
	}

	db, err := persistence.NewDB()
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	testDB = db
	return testDB
}

// setupTestRouter creates a router with test DB
func setupTestRouter(t *testing.T) http.Handler {
	db := setupTestDB(t)
	return NewRouter(db)
}

// performRequest executes an HTTP request and returns the response recorder
func performRequest(r http.Handler, method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// createTestUser creates a user for testing
func createTestUser(t *testing.T, db *gorm.DB, username, email, password string) *models.User {
	// Find user role
	var role models.Role
	if err := db.Where("name = ?", "user").First(&role).Error; err != nil {
		t.Fatalf("failed to find user role: %v", err)
	}

	// Check if user exists
	var existing models.User
	if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
		// User exists, update password
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		existing.HashedPassword = hash
		db.Save(&existing)
		return &existing
	}

	// Create new user
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	rid := role.ID
	user := models.User{
		Username:       username,
		HashedPassword: hash,
		RoleID:         &rid,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create profile
	profile := models.Profile{
		UserID: user.ID,
		Name:   username,
		Email:  email,
	}
	db.Create(&profile)

	return &user
}

// Test: Health endpoint
func TestHealth(t *testing.T) {
	router := setupTestRouter(t)

	resp := performRequest(router, "GET", "/health", nil, nil)

	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", body["status"])
	}
}

// Test: Forgot password with valid email
func TestForgotPassword_WithEmail(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	// Create test user
	email := "testforgot@example.com"
	createTestUser(t, db, "testforgot", email, "password123")

	reqBody, _ := json.Marshal(map[string]string{
		"identifier": email,
	})

	resp := performRequest(router, "POST", "/auth/forgot-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)

	if !strings.Contains(body["message"], "OTP has been sent") {
		t.Errorf("unexpected message: %s", body["message"])
	}

	// Verify OTP record was created
	var pr models.PasswordReset
	if err := db.Where("email = ?", email).Order("id desc").First(&pr).Error; err != nil {
		t.Errorf("OTP record not found: %v", err)
	} else {
		t.Logf("OTP created: %s for email %s (expires: %s)", pr.OTP, pr.Email, pr.ExpiresAt)
	}
}

// Test: Forgot password with username
func TestForgotPassword_WithUsername(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	username := "testuser2"
	email := "testuser2@example.com"
	createTestUser(t, db, username, email, "password123")

	reqBody, _ := json.Marshal(map[string]string{
		"identifier": username,
	})

	resp := performRequest(router, "POST", "/auth/forgot-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Code)
	}
}

// Test: Forgot password with non-existent user (should still return 200)
func TestForgotPassword_NonExistent(t *testing.T) {
	router := setupTestRouter(t)

	reqBody, _ := json.Marshal(map[string]string{
		"identifier": "nonexistent@example.com",
	})

	resp := performRequest(router, "POST", "/auth/forgot-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	// Should still return 200 to prevent user enumeration
	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Code)
	}
}

// Test: Forgot password with invalid body
func TestForgotPassword_InvalidBody(t *testing.T) {
	router := setupTestRouter(t)

	resp := performRequest(router, "POST", "/auth/forgot-password", bytes.NewBuffer([]byte(`{}`)), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)

	if body["error"] != "invalid_body" {
		t.Errorf("expected error=invalid_body, got %s", body["error"])
	}
}

// Test: Verify OTP success
func TestVerifyOTP_Success(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	email := "testverify@example.com"
	createTestUser(t, db, "testverify", email, "password123")

	// Create OTP record directly
	otp := "123456"
	pr := models.PasswordReset{
		UserID:    1,
		Email:     email,
		OTP:       otp,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	db.Create(&pr)

	reqBody, _ := json.Marshal(map[string]string{
		"email": email,
		"otp":   otp,
	})

	resp := performRequest(router, "POST", "/auth/forgot-password/verify", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(resp.Body.Bytes(), &body)

	if resetToken, ok := body["reset_token"].(string); !ok || resetToken == "" {
		t.Errorf("expected reset_token in response, got %+v", body)
	} else {
		t.Logf("Reset token received: %s", resetToken)
	}
}

// Test: Verify OTP with wrong OTP
func TestVerifyOTP_WrongOTP(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	email := "testwrong@example.com"
	createTestUser(t, db, "testwrong", email, "password123")

	// Create OTP record
	pr := models.PasswordReset{
		UserID:    1,
		Email:     email,
		OTP:       "123456",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	db.Create(&pr)

	reqBody, _ := json.Marshal(map[string]string{
		"email": email,
		"otp":   "999999", // wrong OTP
	})

	resp := performRequest(router, "POST", "/auth/forgot-password/verify", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)

	if body["error"] != "invalid_otp" {
		t.Errorf("expected error=invalid_otp, got %s", body["error"])
	}
}

// Test: Verify OTP with expired OTP
func TestVerifyOTP_Expired(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	email := "testexpired@example.com"
	createTestUser(t, db, "testexpired", email, "password123")

	// Create expired OTP record
	otp := "123456"
	pr := models.PasswordReset{
		UserID:    1,
		Email:     email,
		OTP:       otp,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // expired 1 minute ago
	}
	db.Create(&pr)

	reqBody, _ := json.Marshal(map[string]string{
		"email": email,
		"otp":   otp,
	})

	resp := performRequest(router, "POST", "/auth/forgot-password/verify", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}
}

// Test: Reset password success
func TestResetPassword_Success(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	email := "testreset@example.com"
	user := createTestUser(t, db, "testreset", email, "oldpassword123")

	// Create password reset record with token
	resetToken := "valid_reset_token_12345"
	pr := models.PasswordReset{
		UserID:     user.ID,
		Email:      email,
		OTP:        "123456",
		ExpiresAt:  time.Now().Add(15 * time.Minute),
		ResetToken: resetToken,
	}
	db.Create(&pr)

	newPassword := "newpassword456"
	reqBody, _ := json.Marshal(map[string]string{
		"reset_token":          resetToken,
		"new_password":         newPassword,
		"new_password_confirm": newPassword,
	})

	resp := performRequest(router, "POST", "/auth/reset-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)

	if body["message"] != "password updated" {
		t.Errorf("unexpected message: %s", body["message"])
	}

	// Verify password was actually changed
	var updatedUser models.User
	db.First(&updatedUser, user.ID)
	if err := bcrypt.CompareHashAndPassword(updatedUser.HashedPassword, []byte(newPassword)); err != nil {
		t.Error("password was not updated correctly")
	}
}

// Test: Reset password with mismatched passwords
func TestResetPassword_PasswordMismatch(t *testing.T) {
	router := setupTestRouter(t)

	reqBody, _ := json.Marshal(map[string]string{
		"reset_token":          "some_token",
		"new_password":         "password123",
		"new_password_confirm": "different456",
	})

	resp := performRequest(router, "POST", "/auth/reset-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}
}

// Test: Reset password with short password
func TestResetPassword_ShortPassword(t *testing.T) {
	router := setupTestRouter(t)

	reqBody, _ := json.Marshal(map[string]string{
		"reset_token":          "some_token",
		"new_password":         "short",
		"new_password_confirm": "short",
	})

	resp := performRequest(router, "POST", "/auth/reset-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}
}

// Test: Reset password with invalid token
func TestResetPassword_InvalidToken(t *testing.T) {
	router := setupTestRouter(t)

	reqBody, _ := json.Marshal(map[string]string{
		"reset_token":          "invalid_token",
		"new_password":         "newpass123",
		"new_password_confirm": "newpass123",
	})

	resp := performRequest(router, "POST", "/auth/reset-password", bytes.NewBuffer(reqBody), map[string]string{
		"Content-Type": "application/json",
	})

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Code)
	}

	var body map[string]string
	json.Unmarshal(resp.Body.Bytes(), &body)

	if body["error"] != "reset_failed" {
		t.Errorf("expected error=reset_failed, got %s", body["error"])
	}
}

// Test: Full forgot password flow (integration)
func TestFullForgotPasswordFlow(t *testing.T) {
	router := setupTestRouter(t)
	db := setupTestDB(t)

	email := "fullflow@example.com"
	username := "fullflow"
	oldPassword := "oldpassword123"
	createTestUser(t, db, username, email, oldPassword)

	t.Log("Step 1: Request password reset")
	reqBody1, _ := json.Marshal(map[string]string{
		"identifier": email,
	})
	resp1 := performRequest(router, "POST", "/auth/forgot-password", bytes.NewBuffer(reqBody1), map[string]string{
		"Content-Type": "application/json",
	})
	if resp1.Code != 200 {
		t.Fatalf("Step 1 failed: %d", resp1.Code)
	}

	t.Log("Step 2: Get OTP from database (simulating email)")
	var pr models.PasswordReset
	if err := db.Where("email = ?", email).Order("id desc").First(&pr).Error; err != nil {
		t.Fatalf("Failed to get OTP: %v", err)
	}
	t.Logf("OTP: %s", pr.OTP)

	t.Log("Step 3: Verify OTP")
	reqBody2, _ := json.Marshal(map[string]string{
		"email": email,
		"otp":   pr.OTP,
	})
	resp2 := performRequest(router, "POST", "/auth/forgot-password/verify", bytes.NewBuffer(reqBody2), map[string]string{
		"Content-Type": "application/json",
	})
	if resp2.Code != 200 {
		t.Fatalf("Step 3 failed: %d %s", resp2.Code, resp2.Body.String())
	}

	var verifyResp map[string]interface{}
	json.Unmarshal(resp2.Body.Bytes(), &verifyResp)
	resetToken := verifyResp["reset_token"].(string)
	t.Logf("Reset token: %s", resetToken)

	t.Log("Step 4: Reset password")
	newPassword := "newpassword456"
	reqBody3, _ := json.Marshal(map[string]string{
		"reset_token":          resetToken,
		"new_password":         newPassword,
		"new_password_confirm": newPassword,
	})
	resp3 := performRequest(router, "POST", "/auth/reset-password", bytes.NewBuffer(reqBody3), map[string]string{
		"Content-Type": "application/json",
	})
	if resp3.Code != 200 {
		t.Fatalf("Step 4 failed: %d %s", resp3.Code, resp3.Body.String())
	}

	t.Log("Step 5: Verify password was changed")
	var user models.User
	db.Where("username = ?", username).First(&user)
	if err := bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(newPassword)); err != nil {
		t.Error("Password was not updated")
	}
	if bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(oldPassword)) == nil {
		t.Error("Old password still works!")
	}

	t.Log("✓ Full forgot password flow completed successfully")
}
