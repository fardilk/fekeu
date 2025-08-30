package models

import "time"

// Role represents user roles with numeric primary key
type Role struct {
	ID          uint `gorm:"primaryKey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Name        string `gorm:"size:32;uniqueIndex;not null"`
	Description string `gorm:"size:255"`
}
