package models

import "time"

// PasswordReset stores OTP and reset token state for forgot-password flows.
// A record is created when user requests password reset (via email/username).
// OTP is a short numeric code, expires after a short duration.
// Upon successful OTP verification, a reset token is issued to allow setting a new password.
// The record is marked Used after a successful reset.
type PasswordReset struct {
	ID         uint `gorm:"primaryKey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	UserID     uint       `gorm:"index;not null"`
	Email      string     `gorm:"size:255;index;not null"`
	OTP        string     `gorm:"size:12;index;not null"`
	ExpiresAt  time.Time  `gorm:"index;not null"`
	Used       bool       `gorm:"default:false;not null"`
	UsedAt     *time.Time `gorm:"index"`
	ResetToken string     `gorm:"size:255;index"`
}
