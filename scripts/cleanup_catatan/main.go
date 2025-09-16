package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	user := flag.String("user", "", "Username to clean (optional). If empty, cleans all users.")
	dry := flag.Bool("dry-run", true, "Preview actions without modifying the DB")
	yes := flag.Bool("yes", false, "Confirm destructive action when dry-run=false")
	flag.Parse()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN must be set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect db: %v", err)
	}

	if *user == "" {
		fmt.Println("Planned actions:")
		fmt.Println(" - Set uploads.keuangan_id = NULL for ALL uploads")
		fmt.Println(" - DELETE FROM catatan_keuangans (all rows)")
		if *dry {
			fmt.Println("dry-run: no changes made. Use --dry-run=false --yes to execute.")
			return
		}
		if !*yes {
			fmt.Println("Destructive! Pass --yes to proceed.")
			return
		}
		if err := db.Exec("UPDATE uploads SET keuangan_id=NULL WHERE keuangan_id IS NOT NULL").Error; err != nil {
			log.Fatalf("unlink uploads failed: %v", err)
		}
		if err := db.Exec("DELETE FROM catatan_keuangans").Error; err != nil {
			log.Fatalf("delete catatan failed: %v", err)
		}
		fmt.Println("cleanup done (global)")
		return
	}

	// Per-user cleanup: unlink uploads for profiles of this user, then delete catatan rows for this user
	var userID int64
	if err := db.Raw("SELECT id FROM users WHERE username = ?", *user).Row().Scan(&userID); err != nil {
		log.Fatalf("user lookup failed for %s: %v", *user, err)
	}

	fmt.Printf("Planned actions for user %s (id=%d):\n", *user, userID)
	fmt.Println(" - Set uploads.keuangan_id = NULL for uploads whose profile belongs to this user")
	fmt.Println(" - DELETE FROM catatan_keuangans WHERE user_id = $userID")
	if *dry {
		fmt.Println("dry-run: no changes made. Use --dry-run=false --yes to execute.")
		return
	}
	if !*yes {
		fmt.Println("Destructive! Pass --yes to proceed.")
		return
	}
	// Unlink uploads for this user
	if err := db.Exec(`UPDATE uploads SET keuangan_id=NULL WHERE profile_id IN (SELECT id FROM profiles WHERE user_id=?)`, userID).Error; err != nil {
		log.Fatalf("unlink uploads for user failed: %v", err)
	}
	// Delete catatan for this user
	if err := db.Exec(`DELETE FROM catatan_keuangans WHERE user_id=?`, userID).Error; err != nil {
		log.Fatalf("delete catatan for user failed: %v", err)
	}
	fmt.Printf("cleanup done for %s\n", *user)
}
