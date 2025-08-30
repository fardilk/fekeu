package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func setupTestServer(t *testing.T) *gin.Engine {
	// integration tests are opt-in. Set DB_DSN_TEST=1 and DB_DSN to run them.
	if os.Getenv("DB_DSN_TEST") != "1" {
		t.Skip("integration tests are disabled; set DB_DSN_TEST=1 to enable")
	}
	gin.SetMode(gin.TestMode)
	initDB()
	tmp := t.TempDir()
	_ = os.Setenv("UPLOAD_BASE", tmp)
	seedDB()
	r := gin.Default()
	setupRoutes(r)
	return r
}

func TestFullFlow(t *testing.T) {
	r := setupTestServer(t)

	// 1. Register user
	regBody, _ := json.Marshal(map[string]string{"username": "user1", "password": "pass1"})
	resp := performRequest(r, http.MethodPost, "/register", bytes.NewBuffer(regBody), "", "application/json")
	if resp.Code != 200 && resp.Code != 409 {
		b := resp.Body.String()
		t.Fatalf("register failed status=%d body=%s", resp.Code, b)
	}

	// 2. Login
	loginBody, _ := json.Marshal(map[string]string{"username": "user1", "password": "pass1"})
	resp = performRequest(r, http.MethodPost, "/login", bytes.NewBuffer(loginBody), "", "application/json")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("login failed status=%d body=%s", resp.Code, b)
	}
	var loginResp map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &loginResp)
	token, _ := loginResp["token"].(string)
	if token == "" {
		t.Fatalf("empty token in login response: %+v", loginResp)
	}

	// 3. Create profile
	profBody, _ := json.Marshal(map[string]string{"name": "User One", "email": "u1@example.com"})
	resp = performRequest(r, http.MethodPost, "/profile", bytes.NewBuffer(profBody), token, "application/json")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("create profile failed status=%d body=%s", resp.Code, b)
	}

	// 4. Upload file (multipart)
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	_ = mw.WriteField("folder", "keuangan")
	w, _ := mw.CreateFormFile("file", "sample.txt")
	_, _ = w.Write([]byte("SOME CONTENT"))
	_ = mw.Close()
	resp = performRequest(r, http.MethodPost, "/uploads", buf, token, mw.FormDataContentType())
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("upload failed status=%d body=%s", resp.Code, b)
	}

	// 5. Create catatan
	catBody, _ := json.Marshal(map[string]any{"file_name": "sample.txt", "amount": 12345, "date": time.Now().Format(time.RFC3339)})
	resp = performRequest(r, http.MethodPost, "/catatan", bytes.NewBuffer(catBody), token, "application/json")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("create catatan failed status=%d body=%s", resp.Code, b)
	}

	// 6. List catatan
	resp = performRequest(r, http.MethodGet, "/catatan", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("list catatan failed status=%d body=%s", resp.Code, b)
	}

	// 7. Revenue summary
	resp = performRequest(r, http.MethodGet, "/catatan/revenue", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("revenue summary failed status=%d body=%s", resp.Code, b)
	}

	// 8. List uploads
	resp = performRequest(r, http.MethodGet, "/uploads", nil, token, "")
	if resp.Code != 200 {
		b := resp.Body.String()
		t.Fatalf("list uploads failed status=%d body=%s", resp.Code, b)
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
