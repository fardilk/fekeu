package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"be03/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var jwtSecret = []byte("your-secret-key") // TODO: Load from env

// context keys
type ctxKey string

const (
	ctxUser     ctxKey = "user"
	ctxUsername ctxKey = "username"
	ctxRole     ctxKey = "role"
)

// UserHandler handles user-related endpoints
type UserHandler struct {
	db *gorm.DB
}

// getUserFromRequest retrieves the user from the request context
func getUserFromRequest(r *http.Request) (models.User, bool) {
	v := r.Context().Value(ctxUser)
	if v == nil {
		return models.User{}, false
	}
	u, ok := v.(models.User)
	return u, ok
}

// NewUserHandler creates a new user handler
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// Register handles user registration
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Username) == "" || len(req.Password) < 6 {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	var cnt int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&cnt)
	if cnt > 0 {
		writeError(w, r, http.StatusConflict, "duplicate", "username taken", nil)
		return
	}
	hpw, _ := hashPassword(req.Password)
	// default role user
	var role models.Role
	h.db.Where("name = ?", "user").First(&role)
	rid := role.ID
	user := models.User{Username: req.Username, HashedPassword: hpw, RoleID: &rid}
	if err := h.db.Create(&user).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "create_failed", "", nil)
		return
	}
	// auto create profile placeholder
	prof := models.Profile{UserID: user.ID, Name: user.Username}
	_ = h.db.Create(&prof).Error
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID})
}

// Login handles user login and returns JWT token
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
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
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
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
		if err := h.db.First(&r, *user.RoleID).Error; err == nil {
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
	if _, err := h.storeRefreshToken(user, rawRT, 7*24*time.Hour); err != nil {
		// Non-fatal: return access token so FE can proceed. Include empty refresh token to keep response shape stable.
		log.Printf("login: refresh token store failed (non-fatal): %v", err)
		writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "refresh_token": "", "token_type": "bearer", "expires_in": 900})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": at, "refresh_token": rawRT, "token_type": "bearer", "expires_in": 900})
}

// Refresh handles refresh token rotation
func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	rt, err := h.findRefreshTokenByRaw(req.RefreshToken)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_refresh", "", nil)
		return
	}
	var user models.User
	if err := h.db.First(&user, rt.UserID).Error; err != nil {
		writeError(w, r, http.StatusUnauthorized, "invalid_refresh", "", nil)
		return
	}
	roleName := "user"
	if user.RoleID != nil {
		var r models.Role
		if err := h.db.First(&r, *user.RoleID).Error; err == nil {
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

// Revoke handles refresh token revocation
func (h *UserHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	rt, err := h.findRefreshTokenByRaw(req.RefreshToken)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_token", "", nil)
		return
	}
	if err := h.db.Delete(&rt).Error; err != nil {
		writeError(w, r, http.StatusInternalServerError, "revoke_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "token revoked"})
}

// Me returns current user info
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	usernameVal := r.Context().Value(ctxUsername)
	if usernameVal == nil {
		writeError(w, r, http.StatusInternalServerError, "context_missing", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": usernameVal.(string)})
}

// Helper functions

func hashPassword(pw string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
}

func checkPassword(hash []byte, pw string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(pw)) == nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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

func (h *UserHandler) storeRefreshToken(u models.User, raw string, ttl time.Duration) (*models.RefreshToken, error) {
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	hashed := hex.EncodeToString(hasher.Sum(nil))
	rt := models.RefreshToken{UserID: u.ID, TokenHash: hashed, ExpiresAt: time.Now().Add(ttl)}
	if err := h.db.Create(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

func (h *UserHandler) findRefreshTokenByRaw(raw string) (*models.RefreshToken, error) {
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	hashed := hex.EncodeToString(hasher.Sum(nil))
	var rt models.RefreshToken
	if err := h.db.Where("token_hash = ? AND expires_at > ?", hashed, time.Now()).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

// JWTAuthMiddleware validates bearer token and sets context values
// We need to create a middleware factory that has access to the DB
func NewJWTAuthMiddleware(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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

			// Load user from DB
			var user models.User
			if err := db.First(&user, uint(uidF)).Error; err != nil {
				writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxUser, user)
			ctx = context.WithValue(ctx, ctxUsername, username)
			ctx = context.WithValue(ctx, ctxRole, role)
			ctx = context.WithValue(ctx, "uid", uint(uidF))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
