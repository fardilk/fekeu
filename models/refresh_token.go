package models

import "time"

// RefreshToken stores a hashed representation of a refresh token for session rotation and revocation.
type RefreshToken struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uint      `gorm:"index;not null"`
	TokenHash string    `gorm:"size:128;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"index;not null"`
	Revoked   bool      `gorm:"default:false"`
}
