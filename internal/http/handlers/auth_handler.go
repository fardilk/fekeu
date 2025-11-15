package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"be03/internal/core/auth"

	"gorm.io/gorm"
)

type AuthHandler struct {
	db *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Identifier string `json:"identifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Identifier) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	// Always respond 200 to avoid user enumeration
	_ = auth.RequestPasswordReset(h.db, req.Identifier)
	writeJSON(w, http.StatusOK, map[string]any{"message": "If the account exists, an OTP has been sent"})
}

func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		OTP   string `json:"otp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.OTP == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	token, err := auth.VerifyOTP(h.db, req.Email, req.OTP)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_otp", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reset_token": token})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResetToken         string `json:"reset_token"`
		NewPassword        string `json:"new_password"`
		NewPasswordConfirm string `json:"new_password_confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ResetToken == "" || len(req.NewPassword) < 6 || req.NewPassword != req.NewPasswordConfirm {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	if err := auth.ResetPassword(h.db, req.ResetToken, req.NewPassword); err != nil {
		writeError(w, r, http.StatusBadRequest, "reset_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "password updated"})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by JWT middleware)
	uidVal := r.Context().Value("uid")
	if uidVal == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "", nil)
		return
	}
	userID := uidVal.(uint)
	var req struct {
		OldPassword        string `json:"old_password"`
		NewPassword        string `json:"new_password"`
		NewPasswordConfirm string `json:"new_password_confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || len(req.NewPassword) < 6 || req.NewPassword != req.NewPasswordConfirm {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "", nil)
		return
	}
	if err := auth.ChangePassword(h.db, userID, req.OldPassword, req.NewPassword); err != nil {
		if err.Error() == "invalid_old_password" {
			writeError(w, r, http.StatusBadRequest, "invalid_old_password", "", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "change_failed", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "password changed"})
}
