package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Profile represents a user's profile (one-to-one with User)
type Profile struct {
	ID        uint   `gorm:"primaryKey"`
	UUID      string `gorm:"type:uuid;uniqueIndex;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
	// Active indicates whether the profile is active. Use this for soft-state
	// instead of physically deleting the record. Defaults to true.
	Active     bool   `gorm:"default:true;not null"`
	UserID     uint   `gorm:"uniqueIndex;not null"` // one-to-one relation
	User       User   `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Name       string `gorm:"size:255;not null"` // mandatory
	Address    string `gorm:"size:512"`
	Email      string `gorm:"size:255"`
	Phone      string `gorm:"size:64"`
	Occupation string `gorm:"size:255"`
	// Uploads is a one-to-many relation from Profile to Upload
	Uploads []Upload `gorm:"foreignKey:ProfileID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

// BeforeCreate hook sets a UUID on the model if not already provided.
func (p *Profile) BeforeCreate(tx *gorm.DB) (err error) {
	if p.UUID == "" {
		p.UUID = uuid.New().String()
	}
	return nil
}
