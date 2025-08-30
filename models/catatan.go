package models

import "time"

// CatatanKeuangan represents a financial note belonging to a user
type CatatanKeuangan struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uint      `gorm:"index;not null;uniqueIndex:idx_user_file"`
	FileName  string    `gorm:"size:255;not null;uniqueIndex:idx_user_file"`
	Amount    int64     `gorm:"not null"`
	Date      time.Time `gorm:"not null"`
}
