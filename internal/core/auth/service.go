package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"be03/models"
	"be03/pkg/mail"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func randHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func generateOTP() string {
	// 6-digit numeric OTP
	var b [6]byte
	rand.Read(b[:])
	// Map bytes to digits 0-9
	digits := make([]byte, 6)
	for i := 0; i < 6; i++ {
		digits[i] = '0' + (b[i] % 10)
	}
	return string(digits)
}

// RequestPasswordReset finds user by email or username, generates OTP and creates PasswordReset record, sends email.
func RequestPasswordReset(db *gorm.DB, emailOrUsername string) error {
	var user models.User
	if strings.Contains(emailOrUsername, "@") {
		// lookup via profiles.email
		var prof models.Profile
		if err := db.Where("email = ?", emailOrUsername).First(&prof).Error; err != nil {
			return gorm.ErrRecordNotFound
		}
		if err := db.First(&user, prof.UserID).Error; err != nil {
			return err
		}
	} else {
		if err := db.Where("username = ?", emailOrUsername).First(&user).Error; err != nil {
			return err
		}
	}
	// fetch or fallback email
	email := emailOrUsername
	if !strings.Contains(email, "@") {
		var p models.Profile
		if err := db.Where("user_id = ?", user.ID).First(&p).Error; err == nil && p.Email != "" {
			email = p.Email
		}
	}
	if !strings.Contains(email, "@") {
		return errors.New("email_not_found")
	}
	otp := generateOTP()
	pr := models.PasswordReset{
		UserID:    user.ID,
		Email:     email,
		OTP:       otp,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	if err := db.Create(&pr).Error; err != nil {
		return err
	}
	// send email (dummy)
	_ = mail.SendOTP(email, otp)
	return nil
}

// VerifyOTP validates email+OTP and returns a reset token if valid.
func VerifyOTP(db *gorm.DB, email, otp string) (string, error) {
	var pr models.PasswordReset
	if err := db.Where("email = ? AND otp = ? AND used = false", email, otp).Order("id desc").First(&pr).Error; err != nil {
		return "", err
	}
	if time.Now().After(pr.ExpiresAt) {
		return "", errors.New("expired")
	}
	token := randHex(32)
	if err := db.Model(&pr).Update("reset_token", token).Error; err != nil {
		return "", err
	}
	return token, nil
}

// ResetPassword sets a new password if the reset token is valid and marks the record used.
func ResetPassword(db *gorm.DB, token, newPassword string) error {
	var pr models.PasswordReset
	if err := db.Where("reset_token = ? AND used = false", token).First(&pr).Error; err != nil {
		return err
	}
	if time.Now().After(pr.ExpiresAt) {
		return errors.New("expired")
	}
	var user models.User
	if err := db.First(&user, pr.UserID).Error; err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := db.Model(&user).Update("hashed_password", hash).Error; err != nil {
		return err
	}
	now := time.Now()
	if err := db.Model(&pr).Updates(map[string]any{"used": true, "used_at": &now}).Error; err != nil {
		return err
	}
	return nil
}

// ChangePassword verifies the old password and updates to the new one.
func ChangePassword(db *gorm.DB, userID uint, oldPassword, newPassword string) error {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(oldPassword)) != nil {
		return errors.New("invalid_old_password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.Model(&user).Update("hashed_password", hash).Error
}
