package persistence

import (
	"log"
	"os"
	"strings"

	"be03/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewDB initializes and returns a DB connection with migrations and seeding.
func NewDB() (*gorm.DB, error) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		return nil, &ErrMissingConfig{Param: "DB_DSN"}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Run migrations and seeding if enabled
	if err := runMigrations(db); err != nil {
		log.Printf("migration warning: %v", err)
	}
	seedDB(db)

	return db, nil
}

// ErrMissingConfig represents a missing configuration error
type ErrMissingConfig struct {
	Param string
}

func (e *ErrMissingConfig) Error() string {
	return e.Param + " is not set"
}

func runMigrations(db *gorm.DB) error {
	shouldMigrate := true
	if v := os.Getenv("DB_AUTO_MIGRATE"); v != "" {
		lv := strings.ToLower(v)
		if lv == "false" || lv == "0" || lv == "no" {
			shouldMigrate = false
		}
	}

	if !shouldMigrate {
		return nil
	}

	// Ensure roles table first
	if err := db.AutoMigrate(&models.Role{}); err != nil {
		log.Printf("migration warning (roles): %v", err)
	}

	// Seed master roles immediately
	roles := []models.Role{
		{Name: "administrator", Description: "full access"},
		{Name: "user", Description: "regular user"},
	}
	for _, r := range roles {
		var cnt int64
		db.Model(&models.Role{}).Where("name = ?", r.Name).Count(&cnt)
		if cnt == 0 {
			db.Create(&r)
		}
	}

	// Migrate all models
	models := []interface{}{
		&models.User{},
		&models.CatatanKeuangan{},
		&models.Profile{},
		&models.Upload{},
		&models.RefreshToken{},
		&models.PasswordReset{},
	}

	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			log.Printf("migration warning: %v", err)
		}
	}

	// Ensure custom migrations
	if err := ensureUploadProfileFK(db); err != nil {
		log.Printf("warning: ensuring uploads->profiles FK failed: %v", err)
	}
	if err := ensureUUIDColumns(db); err != nil {
		log.Printf("warning: ensuring uuid columns failed: %v", err)
	}

	return nil
}

func ensureUploadProfileFK(db *gorm.DB) error {
	if err := db.Exec(`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS profile_id BIGINT`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_uploads_profile_id ON uploads(profile_id)`).Error; err != nil {
		return err
	}

	type cnt struct{ N int }
	var c cnt
	fkCheckSQL := `SELECT count(*) AS n
		FROM pg_constraint ct
		JOIN pg_class rel ON rel.oid = ct.conrelid
		WHERE rel.relname = 'uploads' AND ct.contype = 'f'
		  AND pg_get_constraintdef(ct.oid) ILIKE '%profile_id%' AND pg_get_constraintdef(ct.oid) ILIKE '%profiles%'`
	if err := db.Raw(fkCheckSQL).Scan(&c).Error; err != nil {
		return err
	}
	if c.N == 0 {
		if err := db.Exec(`ALTER TABLE uploads
			ADD CONSTRAINT fk_uploads_profiles
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
			ON UPDATE CASCADE ON DELETE CASCADE`).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureUUIDColumns(db *gorm.DB) error {
	if err := db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS uuid UUID`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_uuid ON users(uuid)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS uuid UUID`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_uuid ON profiles(uuid)`).Error; err != nil {
		return err
	}
	return nil
}

func seedDB(db *gorm.DB) {
	// Ensure master roles exist
	roles := []models.Role{
		{Name: "administrator", Description: "full access"},
		{Name: "user", Description: "regular user"},
	}
	for _, r := range roles {
		var cnt int64
		db.Model(&models.Role{}).Where("name = ?", r.Name).Count(&cnt)
		if cnt == 0 {
			db.Create(&r)
		}
	}

	// Check if admin user exists
	var count int64
	db.Model(&models.User{}).Where("username = ?", "admin").Count(&count)
	if count == 0 {
		var role models.Role
		if err := db.Where("name = ?", "administrator").First(&role).Error; err != nil {
			log.Printf("failed to find administrator role: %v", err)
			return
		}
		rid := role.ID
		admin := models.User{
			Username: "admin",
			RoleID:   &rid,
		}
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		admin.HashedPassword = hashedPassword
		db.Create(&admin)
		log.Println("Seeded admin user: username=admin, password=admin123")
	}

	// Ensure admin has profile
	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		log.Printf("failed to find admin user after seeding: %v", err)
		return
	}
	var pcount int64
	db.Model(&models.Profile{}).Where("user_id = ?", admin.ID).Count(&pcount)
	if pcount == 0 {
		profile := models.Profile{
			UserID: admin.ID,
			Name:   "Administrator",
			Email:  "admin@example.com",
		}
		if err := db.Create(&profile).Error; err != nil {
			log.Printf("failed to create profile for admin: %v", err)
		} else {
			log.Println("Seeded admin profile for user id:", admin.ID)
		}
	}

	// Ensure upload directory exists
	ensureUploadBase()
}

func ensureUploadBase() {
	base := uploadBaseDir()
	if err := os.MkdirAll(base, 0755); err != nil {
		log.Printf("failed to create upload base dir %s: %v", base, err)
	}
}

func uploadBaseDir() string {
	if v := os.Getenv("UPLOAD_BASE"); v != "" {
		return v
	}
	return "uploads"
}
