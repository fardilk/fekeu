package models

import (
	"time"
)

// User model
type User struct {
	ID             uint `gorm:"primaryKey"`
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
