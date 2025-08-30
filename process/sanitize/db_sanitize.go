package sanitize

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"be03/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Run executes the db_sanitize CLI behavior. Exported so a small cmd/main can call it.
func Run() {
	var (
		dryRun = flag.Bool("dry-run", true, "Don't perform destructive actions; show what would be done")
		yes    = flag.Bool("yes", false, "Confirm destructive action (required to actually truncate)")
		reseed = flag.Bool("reseed", false, "After truncation, reseed master roles and admin user/profile")
		tables = flag.String("tables", "roles,users,profiles,uploads,catatan_keuangans", "Comma-separated list of tables to truncate (default app tables)")
	)
	flag.Parse()

	if os.Getenv("DB_DSN") == "" {
		log.Fatal("DB_DSN must be set to run db_sanitize")
	}
	gdb := mustInitDBFromEnv()

	// sanitize and validate table names (allow letters, digits, underscore, start with letter or underscore)
	nameRe := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	parts := strings.Split(*tables, ",")
	wanted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !nameRe.MatchString(p) {
			log.Printf("warning: skipping invalid table name '%s'", p)
			continue
		}
		wanted = append(wanted, p)
	}

	existing := []string{}
	// check presence individually to avoid any injection risk
	for _, t := range wanted {
		var cnt int64
		if err := gdb.Raw("SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename = ?", t).Scan(&cnt).Error; err != nil {
			log.Fatalf("failed to query pg_tables for %s: %v", t, err)
		}
		if cnt > 0 {
			existing = append(existing, t)
		} else {
			log.Printf("info: table %s not found, skipping", t)
		}
	}
	if len(existing) == 0 {
		log.Println("no requested tables present in the database; nothing to do")
		return
	}

	fmt.Println("Tables considered for truncation:")
	for _, t := range existing {
		fmt.Printf(" - %s\n", t)
	}

	if *dryRun {
		fmt.Println("dry-run enabled; no changes will be made. Use --dry-run=false --yes to execute.")
		return
	}
	if !*yes {
		fmt.Println("Destructive operation. Pass --yes to confirm execution. Aborting.")
		return
	}

	// build a quoted list of identifiers (we validated names) to avoid accidental injection
	quoted := make([]string, 0, len(existing))
	for _, t := range existing {
		// double-quote the identifier to preserve case and safety
		quoted = append(quoted, fmt.Sprintf("\"%s\"", t))
	}
	stmt := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(quoted, ", "))
	log.Printf("Executing: %s", stmt)
	// execute with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := gdb.WithContext(ctx).Exec(stmt).Error; err != nil {
		log.Fatalf("truncate failed: %v", err)
	}
	log.Println("Truncate completed.")

	if *reseed {
		if err := reseedRolesAndAdmin(gdb); err != nil {
			log.Fatalf("reseed failed: %v", err)
		}
	}
}

func reseedRolesAndAdmin(gdb *gorm.DB) error {
	roles := []models.Role{{Name: "administrator", Description: "full access"}, {Name: "user", Description: "regular user"}}
	for _, r := range roles {
		if err := gdb.Where("name = ?", r.Name).FirstOrCreate(&r).Error; err != nil {
			return fmt.Errorf("failed to ensure role %s: %w", r.Name, err)
		}
	}
	var role models.Role
	if err := gdb.Where("name = ?", "administrator").First(&role).Error; err != nil {
		return fmt.Errorf("failed to find administrator role: %w", err)
	}
	rid := role.ID
	hashed, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}
	admin := models.User{Username: "admin", HashedPassword: hashed, RoleID: &rid}
	if err := gdb.Create(&admin).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	profile := models.Profile{UserID: admin.ID, Name: "Administrator", Email: "admin@example.com"}
	if err := gdb.Create(&profile).Error; err != nil {
		return fmt.Errorf("failed to create admin profile: %w", err)
	}
	return nil
}

// mustInitDBFromEnv is a light DB initializer used by this CLI.
func mustInitDBFromEnv() *gorm.DB {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatalf("DB_DSN must be set in environment to run this tool")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	return gdb
}
