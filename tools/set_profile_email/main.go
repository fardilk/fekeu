package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"be03/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	username := flag.String("username", "", "username whose profile email to set")
	email := flag.String("email", "", "email to set on profile")
	name := flag.String("name", "", "optional name to set on profile")
	flag.Parse()
	if *username == "" || *email == "" {
		log.Fatal("--username and --email are required")
	}
	loadDotEnv()
	dsn := os.Getenv("DB_DSN")
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("DB_DSN not set in environment")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	var user models.User
	if err := db.Where("username = ?", *username).First(&user).Error; err != nil {
		log.Fatalf("user not found: %v", err)
	}
	var prof models.Profile
	if err := db.Where("user_id = ?", user.ID).First(&prof).Error; err != nil {
		// create if not exists
		prof = models.Profile{UserID: user.ID, Name: user.Username, Email: *email}
		if *name != "" {
			prof.Name = *name
		}
		if err := db.Create(&prof).Error; err != nil {
			log.Fatalf("create profile failed: %v", err)
		}
		fmt.Printf("created profile for %s with email %s\n", user.Username, *email)
		return
	}
	updates := map[string]interface{}{"email": *email}
	if *name != "" {
		updates["name"] = *name
	}
	if err := db.Model(&prof).Updates(updates).Error; err != nil {
		log.Fatalf("update profile failed: %v", err)
	}
	fmt.Printf("updated profile for %s with email %s\n", user.Username, *email)
}

// Minimal .env loader (non-destructive)
func loadDotEnv() {
	path := ".env"
	if _, err := os.Stat(path); err != nil {
		return
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.IndexByte(line, '='); eq > 0 {
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
}
