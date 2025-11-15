package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"be03/models"
	"be03/pkg/ocr"
	"be03/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// -------------------- helpers --------------------

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

// lightweight JSON helpers
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, extra map[string]any) {
	body := map[string]any{"error": code}
	if msg != "" {
		body["message"] = msg
	}
	for k, v := range extra {
		body[k] = v
	}
	if status >= 500 {
		log.Printf("HTTP %d error code=%s msg=%s path=%s", status, code, msg, r.URL.Path)
	}
	writeJSON(w, status, body)
}

// upload constraints & file sniffing
const maxUploadBytes = 1_000_000 // 1MB
var allowedUploadMimes = map[string]struct{}{"image/jpeg": {}, "image/png": {}}
var allowedUploadExts = map[string]struct{}{".jpg": {}, ".jpeg": {}, ".png": {}}

// validateAndSniff reads <= maxUploadBytes+1, determines mime by extension + magic bytes, returns mime + full bytes.
func validateAndSniff(f multipart.File, hdr *multipart.FileHeader) (string, []byte, error) {
	if hdr.Size > maxUploadBytes {
		return "", nil, errors.New("too_large")
	}
	// Read whole file (bounded by gin's multipart memory); copy into buffer
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, f, maxUploadBytes+1); err != nil && !errors.Is(err, io.EOF) {
		return "", nil, err
	}
	b := buf.Bytes()
	if len(b) > maxUploadBytes {
		return "", nil, errors.New("too_large")
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if _, ok := allowedUploadExts[ext]; !ok {
		return "", nil, errors.New("unsupported_type")
	}
	// quick magic sniff (jpeg/png only)
	mime := ""
	if len(b) >= 4 && b[0] == 0xFF && b[1] == 0xD8 {
		mime = "image/jpeg"
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		mime = "image/png"
	}
	if mime == "" { // fallback from extension map
		if ext == ".jpg" || ext == ".jpeg" {
			mime = "image/jpeg"
		} else if ext == ".png" {
			mime = "image/png"
		}
	}
	if mime == "" {
		return "", nil, errors.New("unsupported_type")
	}
	return mime, b, nil
}

// -------------------- auth & security helpers --------------------

// jwtAuthMiddleware validates bearer token and sets context values
// context keys
type ctxKey string

const (
	ctxUser     ctxKey = "user"
	ctxUsername ctxKey = "username"
	ctxRole     ctxKey = "role"
)

func jwtAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
			return
		}
		tokenStr := strings.TrimSpace(h[7:])
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return jwtSecret, nil
		})
		if err != nil || !token.Valid {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
			return
		}
		uidF, ok := claims["uid"].(float64)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
			return
		}
		username, _ := claims["sub"].(string)
		role, _ := claims["role"].(string)
		var user models.User
		if err := db.First(&user, uint(uidF)).Error; err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxUser, user)
		ctx = context.WithValue(ctx, ctxUsername, username)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserFromRequest(r *http.Request) (models.User, bool) {
	v := r.Context().Value(ctxUser)
	if v == nil {
		return models.User{}, false
	}
	u, ok := v.(models.User)
	return u, ok
}

// password helpers
func hashPassword(pw string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
}
func checkPassword(hash []byte, pw string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(pw)) == nil
}

// refresh token persistence & helpers
func storeRefreshToken(u models.User, raw string, ttl time.Duration) (*models.RefreshToken, error) {
	h := sha256.Sum256([]byte(raw))
	rt := &models.RefreshToken{UserID: u.ID, TokenHash: hex.EncodeToString(h[:]), ExpiresAt: time.Now().Add(ttl)}
	if err := db.Create(rt).Error; err != nil {
		log.Printf("storeRefreshToken failed for user=%s id=%d: %v", u.Username, u.ID, err)
		return nil, err
	}
	return rt, nil
}
func findRefreshTokenByRaw(raw string) (*models.RefreshToken, error) {
	h := sha256.Sum256([]byte(raw))
	var rt models.RefreshToken
	if err := db.Where("token_hash = ?", hex.EncodeToString(h[:])).First(&rt).Error; err != nil {
		return nil, err
	}
	if rt.Revoked || time.Now().After(rt.ExpiresAt) {
		return nil, gorm.ErrRecordNotFound
	}
	return &rt, nil
}

// token generation
func generateAccessToken(u models.User, roleName string, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":  u.Username,
		"uid":  u.ID,
		"role": roleName,
		"exp":  time.Now().Add(ttl).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func randomHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }

// -------------------- forgot/reset/change password --------------------

func forgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Identifier string `json:"identifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	// Always respond 200 to avoid user enumeration
	_ = services.RequestPasswordReset(db, req.Identifier)
	writeJSON(w, http.StatusOK, map[string]any{"message": "If the account exists, an OTP has been sent"})
}

func verifyForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		OTP   string `json:"otp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.OTP == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	token, err := services.VerifyOTP(db, req.Email, req.OTP)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_otp", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reset_token": token})
}

func resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req struct{ ResetToken, NewPassword, NewPasswordConfirm string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ResetToken == "" || len(req.NewPassword) < 6 || req.NewPassword != req.NewPasswordConfirm {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	if err := services.ResetPassword(db, req.ResetToken, req.NewPassword); err != nil {
		writeError(w, r, http.StatusBadRequest, "reset_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "password updated"})
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var req struct{ OldPassword, NewPassword, NewPasswordConfirm string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || len(req.NewPassword) < 6 || req.NewPassword != req.NewPasswordConfirm {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	if err := services.ChangePassword(db, user.ID, req.OldPassword, req.NewPassword); err != nil {
		if err.Error() == "invalid_old_password" {
			writeError(w, r, http.StatusBadRequest, "invalid_old_password", "", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "change_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "password changed"})
}

// register/login/refresh/revoke/me handlers
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Username) == "" || len(req.Password) < 6 {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	var cnt int64
	db.Model(&models.User{}).Where("username = ?", req.Username).Count(&cnt)
	if cnt > 0 {
		writeError(w, r, http.StatusConflict, "duplicate", "username taken", nil)
		return
	}
	hpw, _ := hashPassword(req.Password)
	// default role user
	var role models.Role
	db.Where("name = ?", "user").First(&role)
	rid := role.ID
	user := models.User{Username: req.Username, HashedPassword: hpw, RoleID: &rid}
	if err := db.Create(&user).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "create_failed", "", nil)
		return
	}
	// auto create profile placeholder
	prof := models.Profile{UserID: user.ID, Name: user.Username}
	_ = db.Create(&prof).Error
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	var buf bytes.Buffer
	tee := io.TeeReader(r.Body, &buf)
	decErr := json.NewDecoder(tee).Decode(&req)
	raw := buf.Bytes()
	if decErr != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		// Fallback: accept form-encoded credentials as well
		if err := r.ParseForm(); err == nil {
			if u := strings.TrimSpace(r.Form.Get("username")); u != "" {
				if p := r.Form.Get("password"); p != "" {
					req.Username, req.Password = u, p
				}
			}
		}
		if req.Username == "" || req.Password == "" {
			log.Printf("login: bind error=%v headers=%v raw=%q", decErr, r.Header, string(raw))
			writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
			return
		}
	}
	var user models.User
	if err := db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials", "", nil)
		return
	}
	if !checkPassword(user.HashedPassword, req.Password) {
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials", "", nil)
		return
	}
	roleName := "user"
	if user.RoleID != nil {
		var r models.Role
		if err := db.First(&r, *user.RoleID).Error; err == nil {
			roleName = r.Name
		}
	}
	at, err := generateAccessToken(user, roleName, 15*time.Minute)
	if err != nil {
		log.Printf("generateAccessToken failed: %v", err)
		writeError(w, r, http.StatusInternalServerError, "token_failed", "", nil)
		return
	}
	rawRT := randomHex(32)
	if _, err := storeRefreshToken(user, rawRT, 7*24*time.Hour); err != nil {
		// Non-fatal: return access token so FE can proceed. Include empty refresh token to keep response shape stable.
		log.Printf("login: refresh token store failed (non-fatal): %v", err)
		writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "refresh_token": "", "token_type": "bearer", "expires_in": 900})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "refresh_token": rawRT, "token_type": "bearer", "expires_in": 900})
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	rt, err := findRefreshTokenByRaw(req.RefreshToken)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_refresh", "", nil)
		return
	}
	var user models.User
	if err := db.First(&user, rt.UserID).Error; err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_refresh", "", nil)
		return
	}
	roleName := "user"
	if user.RoleID != nil {
		var r models.Role
		if err := db.First(&r, *user.RoleID).Error; err == nil {
			roleName = r.Name
		}
	}
	at, err := generateAccessToken(user, roleName, 15*time.Minute)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "token_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "token_type": "bearer", "expires_in": 900})
}

func revokeRefreshHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error(), nil)
		return
	}
	rt, err := findRefreshTokenByRaw(req.RefreshToken)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "refresh token not found", nil)
		return
	}
	rt.Revoked = true
	if err := db.Save(rt).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "revoke_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "refresh token revoked"})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	usernameVal := r.Context().Value(ctxUsername)
	if usernameVal == nil {
		writeError(w, r, http.StatusInternalServerError, "context_missing", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": usernameVal.(string)})
}

// -------------------- profile --------------------

func createProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var req struct {
		Name                              string `json:"name" binding:"required"`
		Address, Email, Phone, Occupation string
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error(), nil)
		return
	}
	profile := models.Profile{UserID: user.ID, Name: req.Name, Address: req.Address, Email: req.Email, Phone: req.Phone, Occupation: req.Occupation}
	if err := db.Create(&profile).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "create_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": profile.ID})
}

func getProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var p models.Profile
	if err := db.Where("user_id = ?", user.ID).First(&p).Error; err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "profile not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// -------------------- catatan --------------------

func createCatatanHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var req struct {
		FileName string `json:"file_name" binding:"required"`
		Amount   int64  `json:"amount" binding:"required"`
		Date     string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error(), nil)
		return
	}
	var existing models.CatatanKeuangan
	if err := db.Where("user_id = ? AND file_name = ?", user.ID, req.FileName).First(&existing).Error; err == nil {
		writeError(w, r, http.StatusConflict, "duplicate", "file already recorded", nil)
		return
	}
	ct := models.CatatanKeuangan{UserID: user.ID, FileName: req.FileName, Amount: req.Amount}
	if req.Date != "" {
		if t, err := time.Parse(time.RFC3339, req.Date); err == nil {
			ct.Date = t
		} else {
			ct.Date = time.Now()
		}
	} else {
		ct.Date = time.Now()
	}
	if err := db.Create(&ct).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "create_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": ct.ID})
}

func listCatatanHandler(w http.ResponseWriter, r *http.Request) {
	role, _ := r.Context().Value(ctxRole).(string)
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var items []models.CatatanKeuangan
	q := db.Model(&models.CatatanKeuangan{})
	if role != "administrator" {
		q = q.Where("user_id = ?", user.ID)
	}
	if err := q.Order("id desc").Limit(200).Find(&items).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func revenueSummaryHandler(w http.ResponseWriter, r *http.Request) {
	role, _ := r.Context().Value(ctxRole).(string)
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	type Result struct {
		Month string
		Total int64
	}
	var results []Result
	q := db.Model(&models.CatatanKeuangan{})
	if role != "administrator" {
		q = q.Where("user_id = ?", user.ID)
	}
	rows, err := q.Select("to_char(date, 'YYYY-MM') as month, sum(amount) as total").Group("month").Rows()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", "", nil)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r Result
		rows.Scan(&r.Month, &r.Total)
		results = append(results, r)
	}
	writeJSON(w, http.StatusOK, results)
}

// getCatatanTotalHandler returns a single total (sum of amount) for the authenticated user.
func getCatatanTotalHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	// Sum with a single query
	type Row struct{ Total int64 }
	var row Row
	if err := db.Raw("SELECT COALESCE(SUM(amount),0) AS total FROM catatan_keuangans WHERE user_id = ?", user.ID).Scan(&row).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": row.Total})
}

// -------------------- uploads (atomic DB-first) --------------------

func uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var profile models.Profile
	if err := db.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		writeError(w, r, http.StatusBadRequest, "profile_missing", "profile missing", nil)
		return
	}
	// Force uploads into the folder watched by the watcher: public/keu
	_ = r.ParseMultipartForm(10 << 20)
	folder := strings.ToLower(strings.TrimSpace(r.FormValue("folder")))
	if folder != "keu" { // normalize any value to the single supported folder
		folder = "keu"
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "missing_file", "file missing", nil)
		return
	}
	// sanitize filename to prevent directory traversal or weird paths
	cleanName := filepath.Base(header.Filename)
	mime, firstBytes, verr := validateAndSniff(file, header)
	if verr != nil {
		switch verr.Error() {
		case "too_large":
			writeError(w, r, http.StatusBadRequest, "file_too_large", "file too large (max 1MB)", nil)
		case "unsupported_type":
			writeError(w, r, http.StatusBadRequest, "unsupported_type", "File tidak dikenali, gunakan file lain!", map[string]any{"allowed": []string{"image/jpeg", "image/png"}})
		default:
			writeError(w, r, http.StatusBadRequest, "invalid_file", "", nil)
		}
		return
	}
	baseDir := "public"
	relPath := folder + "/" + cleanName
	fullPath := filepath.Join(baseDir, relPath)
	storePath := filepath.ToSlash(filepath.Join("public", relPath))
	// optional manual linkage (declared early as it may be used in creation branch)
	var keuID *uint
	var catatanID *uint
	// duplicate check with reprocess support
	var up models.Upload
	var reprocess bool
	if err := db.Where("profile_id = ? AND file_name = ?", profile.ID, cleanName).First(&up).Error; err == nil {
		// Reuse existing record to allow re-uploads (e.g., previous OCR failed)
		reprocess = true
		up.StorePath = storePath
		up.ContentType = mime
		// reset failure state; will update after OCR
		up.Failed = false
		up.FailedReason = ""
		if keuID != nil {
			up.KeuanganID = keuID
		}
		_ = db.Save(&up).Error
	} else {
		up = models.Upload{ProfileID: profile.ID, FileName: cleanName, StorePath: storePath, KeuanganID: keuID, ContentType: mime}
		if err := db.Create(&up).Error; err != nil {
			writeError(w, r, http.StatusInternalServerError, "db_save_failed", "", nil)
			return
		}
	}
	// optional manual linkage
	if v := r.FormValue("keuangan_id"); v != "" {
		if parsed, _ := strconv.ParseUint(v, 10, 64); parsed != 0 {
			pv := uint(parsed)
			keuID = &pv
		}
	}
	if amtStr := r.FormValue("amount"); amtStr != "" {
		if amtVal, err := strconv.ParseInt(amtStr, 10, 64); err == nil && amtVal > 0 {
			var existing models.CatatanKeuangan
			if err := db.Where("user_id = ? AND file_name = ?", user.ID, cleanName).First(&existing).Error; err == nil {
				keuID = &existing.ID
				cid := existing.ID
				catatanID = &cid
			} else {
				ck := models.CatatanKeuangan{UserID: user.ID, FileName: cleanName, Amount: amtVal, Date: time.Now()}
				if err := db.Create(&ck).Error; err == nil {
					cid := ck.ID
					catatanID = &cid
					keuID = &cid
				}
			}
		}
	}
	stagingDir := filepath.Join(baseDir, ".staging")
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		if !reprocess {
			db.Delete(&up)
		}
		writeError(w, r, http.StatusInternalServerError, "mkdir_failed", "", nil)
		return
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		if !reprocess {
			db.Delete(&up)
		}
		writeError(w, r, http.StatusInternalServerError, "mkdir_failed", "", nil)
		return
	}
	tmpName := filepath.Join(stagingDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), header.Filename))
	if err := os.WriteFile(tmpName, firstBytes, 0644); err != nil {
		if !reprocess {
			db.Delete(&up)
		}
		writeError(w, r, http.StatusInternalServerError, "save_failed", "", nil)
		return
	}
	if err := os.Rename(tmpName, fullPath); err != nil {
		if !reprocess {
			db.Delete(&up)
		}
		_ = os.Remove(tmpName)
		writeError(w, r, http.StatusInternalServerError, "save_failed", "", nil)
		return
	}
	log.Printf("OCR: starting on %s for user=%d file=%s", fullPath, profile.UserID, cleanName)
	amt, _, raw, err := ocr.ExtractAmountFromImage(fullPath)
	if err != nil {
		log.Printf("OCR: error on %s: %v", fullPath, err)
		writeError(w, r, http.StatusInternalServerError, "ocr_error", "", nil)
		return
	}
	log.Printf("OCR: result amount=%d raw=%q for %s", amt, raw, fullPath)
	if amt <= 0 {
		up.Failed = true
		up.FailedReason = "Nominal tidak ditemukan, gunakan file lain"
		db.Save(&up)
		_ = os.Remove(fullPath)
		writeError(w, r, http.StatusBadRequest, "amount_not_found", "Nominal tidak ditemukan, gunakan file lain", nil)
		return
	}
	if amt > 0 {
		var existingCat models.CatatanKeuangan
		if err := db.Where("user_id = ? AND file_name = ?", profile.UserID, up.FileName).First(&existingCat).Error; err == nil {
			up.KeuanganID = &existingCat.ID
			db.Save(&up)
		} else {
			// Never create catatan for admin (user_id=1)
			if profile.UserID != 1 {
				ct := models.CatatanKeuangan{UserID: profile.UserID, FileName: up.FileName, Amount: amt, Date: time.Now()}
				if err := db.Create(&ct).Error; err == nil {
					up.KeuanganID = &ct.ID
					db.Save(&up)
					log.Printf("OCR: created catatan id=%d amount=%d for user=%d file=%s", ct.ID, amt, profile.UserID, up.FileName)
				} else {
					log.Printf("OCR: failed to create catatan for user=%d file=%s: %v", profile.UserID, up.FileName, err)
				}
			}
		}
	}
	respCatID := up.KeuanganID
	if catatanID != nil {
		respCatID = catatanID
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": up.ID, "path": relPath, "store_path": storePath, "catatan_id": respCatID})
}

func listUploadsHandler(w http.ResponseWriter, r *http.Request) {
	role, _ := r.Context().Value(ctxRole).(string)
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var profile models.Profile
	db.Where("user_id = ?", user.ID).First(&profile)
	var uploads []models.Upload
	q := db.Model(&models.Upload{})
	if role != "administrator" {
		q = q.Where("profile_id = ?", profile.ID)
	}
	if err := q.Order("id desc").Limit(100).Find(&uploads).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, uploads)
}

func getUploadHandler(w http.ResponseWriter, r *http.Request) {
	role, _ := r.Context().Value(ctxRole).(string)
	user, ok := getUserFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	var profile models.Profile
	db.Where("user_id = ?", user.ID).First(&profile)
	id := chi.URLParam(r, "id")
	var up models.Upload
	if err := db.First(&up, id).Error; err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "", nil)
		return
	}
	if role != "administrator" && up.ProfileID != profile.ID {
		writeError(w, r, http.StatusForbidden, "forbidden", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, up)
}

// -------------------- health --------------------
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// -------------------- routes wiring --------------------
// BuildChiRouter constructs the chi router with all routes and middleware
func BuildChiRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Get("/health", healthHandler)
	r.Post("/register", registerHandler)
	r.Post("/login", loginHandler)
	r.Post("/refresh", refreshHandler)
	r.Post("/revoke", revokeRefreshHandler)
	// forgot/reset password flows
	r.Post("/auth/forgot-password", forgotPasswordHandler)
	r.Post("/auth/forgot-password/verify", verifyForgotPasswordHandler)
	r.Post("/auth/reset-password", resetPasswordHandler)
	r.Group(func(ar chi.Router) {
		ar.Use(jwtAuthMiddleware)
		ar.Get("/me", meHandler)
		ar.Put("/auth/change-password", changePasswordHandler)
		ar.Post("/profile", createProfileHandler)
		ar.Get("/profile", getProfileHandler)
		ar.Post("/catatan", createCatatanHandler)
		ar.Get("/catatan", listCatatanHandler)
		ar.Get("/catatan/total", getCatatanTotalHandler)
		ar.Get("/catatan/revenue", revenueSummaryHandler)
		ar.Post("/uploads", uploadFileHandler)
		ar.Get("/uploads", listUploadsHandler)
		ar.Get("/uploads/{id}", getUploadHandler)
	})
	return r
}
