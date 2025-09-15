package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID       uint
	Username string
}

type Catatan struct {
	ID       uint
	UserID   uint
	FileName string
	Amount   int64
}

// TableName overrides GORM's default pluralization to match the CatatanKeuangan model's table.
func (Catatan) TableName() string { return "catatan_keuangans" }

func main() {
	username := flag.String("username", "", "username")
	file := flag.String("file", "", "file name")
	wait := flag.Int("wait", 15, "seconds to wait/poll")
	flag.Parse()
	if *username == "" || *file == "" {
		log.Fatal("--username and --file are required")
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
		log.Fatalf("user not found: %v", err)
	}
	deadline := time.Now().Add(time.Duration(*wait) * time.Second)
	for {
		var c Catatan
		err := db.Where("user_id = ? AND file_name = ?", u.ID, *file).Order("id desc").First(&c).Error
		if err == nil {
			fmt.Printf("FOUND amount=%d for file=%s\n", c.Amount, c.FileName)
			return
		}
		if time.Now().After(deadline) {
			log.Fatalf("not found after %ds waiting", *wait)
		}
		time.Sleep(2 * time.Second)
	}
}
