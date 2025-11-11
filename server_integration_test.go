package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// helper to perform requests with auth token
func performRequest(r http.Handler, method, path string, body io.Reader, token string, contentType string) *httptest.ResponseRecorder {
	// allow callers to pass nil for body safely
	req, _ := http.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func setupTestServer(t *testing.T) http.Handler {
	// integration tests are opt-in. Set DB_DSN_TEST=1 and DB_DSN to run them.
	if os.Getenv("DB_DSN_TEST") != "1" {
		t.Skip("integration tests are disabled; set DB_DSN_TEST=1 to enable")
	}
	initDB()
	tmp := t.TempDir()
	_ = os.Setenv("UPLOAD_BASE", tmp)
	seedDB()
	return BuildChiRouter()
}

func TestFullFlow(t *testing.T) {
	r := setupTestServer(t)

	// Use seeded admin user instead of creating new user (DB is persistent)
	// 1. Login with admin
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	resp := performRequest(r, http.MethodPost, "/login", bytes.NewBuffer(loginBody), "", "application/json")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("login failed status=%d body=%s", resp.Code, b)
	}
	var loginResp map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &loginResp)
	token, _ := loginResp["access_token"].(string)
	if token == "" {
		t.Fatalf("empty token in login response: %+v", loginResp)
	}

	// 2. Get profile (admin already has profile from seedDB)
	resp = performRequest(r, http.MethodGet, "/profile", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("get profile failed status=%d body=%s", resp.Code, b)
	}

	// 3. Create catatan
	fname := fmt.Sprintf("test_%d.txt", time.Now().Unix())
	catBody, _ := json.Marshal(map[string]any{"file_name": fname, "amount": 12345, "date": time.Now().Format(time.RFC3339)})
	resp = performRequest(r, http.MethodPost, "/catatan", bytes.NewBuffer(catBody), token, "application/json")
	if resp.Code != 200 && resp.Code != 409 {
		b := resp.Body.String()
		t.Fatalf("create catatan failed status=%d body=%s", resp.Code, b)
	}

	// 4. List catatan
	resp = performRequest(r, http.MethodGet, "/catatan", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("list catatan failed status=%d body=%s", resp.Code, b)
	}

	// 5. Get catatan total
	resp = performRequest(r, http.MethodGet, "/catatan/total", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("get catatan total failed status=%d body=%s", resp.Code, b)
	}

	// 6. Revenue summary
	resp = performRequest(r, http.MethodGet, "/catatan/revenue", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("revenue summary failed status=%d body=%s", resp.Code, b)
	}

	// 7. List uploads
	resp = performRequest(r, http.MethodGet, "/uploads", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("list uploads failed status=%d body=%s", resp.Code, b)
	}

	// 8. Test /me endpoint
	resp = performRequest(r, http.MethodGet, "/me", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("me endpoint failed status=%d body=%s", resp.Code, b)
	}

	// 9. Unauthorized access to protected endpoint should be 401
	unauth := performRequest(r, http.MethodGet, "/catatan", nil, "", "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized list catatan got %d", unauth.Code)
	}
}

func TestMigrateCommand(t *testing.T) {
	if os.Getenv("DB_DSN_TEST") != "1" {
		t.Skip("integration tests are disabled; set DB_DSN_TEST=1 to enable")
	}
	initDB()
}
