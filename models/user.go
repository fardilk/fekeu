package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID             uint   `gorm:"primaryKey"`
	UUID           string `gorm:"type:uuid;uniqueIndex;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time `gorm:"index"`
	Username       string     `gorm:"size:255;not null;unique"`
	HashedPassword []byte     `gorm:"not null"`
	Catatan        []CatatanKeuangan
	Profile        *Profile `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	RoleID         *uint    `gorm:"index"`
	Role           Role     `gorm:"foreignKey:RoleID;references:ID"`
}

// BeforeCreate hook sets a UUID on the model if not already provided.
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.UUID == "" {
		u.UUID = uuid.New().String()
	}
	return nil
}
