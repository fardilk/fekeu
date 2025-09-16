package main

import (
	"log"
	"os"
	"strings"

	"be03/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var db *gorm.DB

func initDB() {
	var err error
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN is not set. This project requires a Postgres DSN in DB_DSN.")
	}
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect postgres database:", err)
	}
	// Control schema migrations with env DB_AUTO_MIGRATE (default true). Any permission errors will be logged and ignored.
	shouldMigrate := true
	if v := os.Getenv("DB_AUTO_MIGRATE"); v != "" {
		lv := strings.ToLower(v)
		if lv == "false" || lv == "0" || lv == "no" {
			shouldMigrate = false
		}
	}
	// Ensure the roles master table exists first and seed it so users FK can be applied safely.
	if shouldMigrate {
		if err := db.AutoMigrate(&models.Role{}); err != nil {
			log.Printf("migration warning (roles): %v", err)
		}
	}
	// seed master roles immediately
	roles := []models.Role{{Name: "administrator", Description: "full access"}, {Name: "user", Description: "regular user"}}
	for _, r := range roles {
		var cnt int64
		db.Model(&models.Role{}).Where("name = ?", r.Name).Count(&cnt)
		if cnt == 0 {
			db.Create(&r)
		}
	}

	// Now migrate the rest (users will get FK to roles)
	if shouldMigrate {
		// Migrate models individually so a failure on one doesn't block others
		if err := db.AutoMigrate(&models.User{}); err != nil {
			log.Printf("migration warning (users): %v", err)
		}
		if err := db.AutoMigrate(&models.CatatanKeuangan{}); err != nil {
			log.Printf("migration warning (catatan_keuangans): %v", err)
		}
		if err := db.AutoMigrate(&models.Profile{}); err != nil {
			log.Printf("migration warning (profiles): %v", err)
		}
		if err := db.AutoMigrate(&models.Upload{}); err != nil {
			log.Printf("migration warning (uploads): %v", err)
		}
		if err := db.AutoMigrate(&models.RefreshToken{}); err != nil {
			log.Printf("migration warning (refresh_tokens): %v", err)
		}
	}

	// Ensure uploads -> profiles FK exists (in case table existed before adding ProfileID)
	if shouldMigrate {
		if err := ensureUploadProfileFK(); err != nil {
			log.Printf("warning: ensuring uploads->profiles FK failed: %v", err)
		}
	}
	seedDB()
}

// ensureUploadProfileFK adds the profile_id column and FK constraint if they are missing.
func ensureUploadProfileFK() error {
	// 1. Ensure profile_id column exists
	if err := db.Exec(`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS profile_id BIGINT`).Error; err != nil {
		return err
	}
	// 2. Create index (idempotent)
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_uploads_profile_id ON uploads(profile_id)`).Error; err != nil {
		return err
	}
	// 3. Check if FK already present
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
		// 4. Add FK (will fail if existing nulls & NOT NULL required; leave NOT NULL to AutoMigrate)
		if err := db.Exec(`ALTER TABLE uploads
			ADD CONSTRAINT fk_uploads_profiles
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
			ON UPDATE CASCADE ON DELETE CASCADE`).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedDB() {
	// Ensure master roles exist
	roles := []models.Role{{Name: "administrator", Description: "full access"}, {Name: "user", Description: "regular user"}}
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
		// find administrator role id
		var role models.Role
		if err := db.Where("name = ?", "administrator").First(&role).Error; err != nil {
			log.Printf("failed to find administrator role: %v", err)
		}
		// Seed admin user
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
	// Ensure admin has a one-to-one profile
	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		log.Printf("failed to find admin user after seeding: %v", err)
		return
	}
	var pcount int64
	db.Model(&models.Profile{}).Where("user_id = ?", admin.ID).Count(&pcount)
	if pcount == 0 {
		profile := models.Profile{UserID: admin.ID, Name: "Administrator", Email: "admin@example.com"}
		if err := db.Create(&profile).Error; err != nil {
			log.Printf("failed to create profile for admin: %v", err)
		} else {
			log.Println("Seeded admin profile for user id:", admin.ID)
		}
	}
	// Ensure upload directory exists
	ensureUploadBase()
}

// ensureUploadBase creates the base uploads directory.
func ensureUploadBase() {
	base := uploadBaseDir()
	if err := os.MkdirAll(base, 0755); err != nil {
		log.Printf("failed to create upload base dir %s: %v", base, err)
	}
}

// uploadBaseDir returns the base directory for local uploads (configurable via UPLOAD_BASE env)
func uploadBaseDir() string {
	if v := os.Getenv("UPLOAD_BASE"); v != "" {
		return v
	}
	return "uploads"
}
