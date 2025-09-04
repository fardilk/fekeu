package models

import (
	"time"
)

// Upload represents a user's profile-related uploaded file. Simplified to requested fields.
type Upload struct {
	ID          uint `gorm:"primaryKey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FileName    string  `gorm:"size:255;not null"`
	StorePath   string  `gorm:"column:store_path;size:512"` // public relative path (e.g. public/keu/xxx.jpg)
	ProfileID   uint    `gorm:"index;not null"`             // FK to profiles.id (profile_id)
	Profile     Profile `gorm:"foreignKey:ProfileID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ContentType string  `gorm:"size:128"`
	KeuanganID  *uint   `gorm:"index"` // FK to catatan_keuangans.id (nullable)
	// Mark upload as failed for OCR processing (do not delete record so front-end/admin can review)
	Failed       bool   `gorm:"default:false;index"`
	FailedReason string `gorm:"size:255"`
}
