package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID       uint
	Username string
}
type Profile struct {
	ID     uint
	UserID uint
}
type Upload struct {
	ID           uint
	ProfileID    uint
	FileName     string
	StorePath    string
	ContentType  string
	KeuanganID   *uint
	Failed       bool
	FailedReason string
}

func (Upload) TableName() string  { return "uploads" }
func (Profile) TableName() string { return "profiles" }

func main() {
	username := flag.String("username", "", "username")
	file := flag.String("file", "", "file name")
	flag.Parse()
	if *username == "" || *file == "" {
		log.Fatal("--username and --file required")
	}
	dsn := os.Getenv("DB_DSN")
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("DB_DSN not set in env")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	var u User
	if err := db.Where("username = ?", *username).First(&u).Error; err != nil {
		log.Fatalf("user: %v", err)
	}
	var p Profile
	if err := db.Where("user_id = ?", u.ID).First(&p).Error; err != nil {
		log.Fatalf("profile: %v", err)
	}
	var up Upload
	err = db.Where("profile_id = ? AND file_name = ?", p.ID, *file).Order("id desc").First(&up).Error
	if err != nil {
		log.Fatalf("upload: %v", err)
	}
	fmt.Printf("upload id=%d keuangan_id=%v failed=%v reason=%q store=%s ct=%s\n", up.ID, up.KeuanganID, up.Failed, up.FailedReason, up.StorePath, up.ContentType)
}
