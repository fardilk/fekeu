package main

import "time"

// CatatanKeuangan represents a financial note belonging to a user
// Separated from main.go for clarity and modularity.
type CatatanKeuangan struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
	UserID    uint       `gorm:"index;not null"`
	FileName  string     `gorm:"size=255;not null"`
	Amount    int64      `gorm:"not null"` // smallest currency unit (e.g. cents)
	Date      time.Time  `gorm:"not null"`
}
