package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID             uint
	Username       string
	HashedPassword []byte
}

func main() {
	username := flag.String("username", "", "username to reset")
	password := flag.String("password", "", "new plaintext password (min 6 chars)")
	flag.Parse()
	if *username == "" || *password == "" {
		log.Fatal("--username and --password are required")
	}
	if len(*password) < 6 {
		log.Fatal("password too short (min 6)")
	}
	loadDotEnv()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN not set in env")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	var user User
	if err := db.Where("username = ?", *username).First(&user).Error; err != nil {
		log.Fatalf("user not found: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}
	if err := db.Model(&user).Update("hashed_password", hash).Error; err != nil {
		log.Fatalf("update failed: %v", err)
	}
	fmt.Printf("Password reset for user %s\n", user.Username)
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
