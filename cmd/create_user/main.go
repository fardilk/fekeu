package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"be03/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run ./cmd/create_user <username> <password>")
		os.Exit(2)
	}
	username := os.Args[1]
	password := os.Args[2]

	dsn := os.Getenv("DB_DSN")
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("DB_DSN not set in environment")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}

	// ensure roles exist
	var role models.Role
	if err := db.Where("name = ?", "user").First(&role).Error; err != nil {
		// try to create
		role = models.Role{Name: "user", Description: "regular user"}
		db.Create(&role)
	}

	// check existing
	var existing models.User
	if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
		fmt.Printf("user %s already exists (id=%d)\n", username, existing.ID)
		os.Exit(0)
	}

	hpw, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt failed: %v", err)
	}
	rid := role.ID
	user := models.User{Username: username, HashedPassword: hpw, RoleID: &rid}
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("failed to create user: %v", err)
	}
	// create profile
	prof := models.Profile{UserID: user.ID, Name: username}
	if err := db.Create(&prof).Error; err != nil {
		log.Printf("warning: failed to create profile: %v", err)
	}
	fmt.Printf("created user %s id=%d\n", username, user.ID)
}
