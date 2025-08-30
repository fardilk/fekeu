package main

import (
	"fmt"
	"strings"

	"be03/models"

	"golang.org/x/crypto/bcrypt"
)

// Auth helpers duplicated into root package so handlers in the root can call them.
func RegisterUser(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username required")
	}
	if len(password) < 6 { // basic password policy
		return fmt.Errorf("password too short (min 6)")
	}
	// pre-check existing (optimistic)
	var existing models.User
	if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
		return fmt.Errorf("user already exists")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// ensure role exists (idempotent)
	var role models.Role
	if err := db.Where("name = ?", "user").First(&role).Error; err != nil {
		// try create
		role = models.Role{Name: "user", Description: "regular user"}
		if err2 := db.Where("name = ?", role.Name).FirstOrCreate(&role).Error; err2 != nil {
			return fmt.Errorf("failed to ensure user role: %v", err2)
		}
	}
	rid := role.ID
	user := models.User{Username: username, HashedPassword: hashedPassword, RoleID: &rid}
	if err := db.Create(&user).Error; err != nil {
		if isUniqueConstraintError(err) { // race condition after initial check
			return fmt.Errorf("user already exists")
		}
		return err
	}
	return nil
}

func Authenticate(username, password string) (models.User, error) {
	username = strings.TrimSpace(username)
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return models.User{}, fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword(user.HashedPassword, []byte(password)); err != nil {
		return models.User{}, fmt.Errorf("invalid credentials")
	}
	return user, nil
}

// local copy (cannot rely on process binary helper)
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key") || strings.Contains(s, "unique constraint") || strings.Contains(s, "already exists")
}

// Compatibility wrappers expected by handlers.go
func Register(username, password string) error {
	return RegisterUser(username, password)
}

func Login(username, password string) (models.User, error) {
	return Authenticate(username, password)
}
