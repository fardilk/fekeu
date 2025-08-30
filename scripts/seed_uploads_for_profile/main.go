package main

// seeds uploads from uploads/keuangan for a given username (profile owner). For each file:
// - compute a deterministic folder prefix from first 3 chars + profile id
// - store path: public/keu/<prefix>/<filename>
// - create Upload if missing and CatatanKeuangan row (Amount=0) if missing, link them

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"be03/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mustDBFromEnv() *gorm.DB {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN not set in env")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	return gdb
}

func prefixFor(name string, profileID uint) string {
	first := name
	if len(name) > 3 {
		first = name[:3]
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d", first, profileID)))
	hexs := hex.EncodeToString(h[:])
	if len(hexs) < 4 {
		return hexs
	}
	return filepath.Join(hexs[:2], hexs[2:4])
}

func main() {
	username := flag.String("username", "fardiluser", "username to assign uploads to")
	dir := flag.String("dir", "uploads/keuangan", "directory to scan")
	dry := flag.Bool("dry-run", true, "don't write to db")
	flag.Parse()

	gdb := mustDBFromEnv()

	var user models.User
	if err := gdb.Where("username = ?", *username).First(&user).Error; err != nil {
		log.Fatalf("user not found: %v", err)
	}
	var profile models.Profile
	if err := gdb.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		log.Fatalf("profile for user not found: %v", err)
	}

	entries, err := os.ReadDir(*dir)
	if err != nil {
		log.Fatalf("read dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// skip non-image files
		if !strings.HasSuffix(strings.ToLower(name), ".png") && !strings.HasSuffix(strings.ToLower(name), ".jpg") && !strings.HasSuffix(strings.ToLower(name), ".jpeg") {
			continue
		}
		pref := prefixFor(name, profile.ID)
		store := filepath.ToSlash(filepath.Join("public/keu", pref, name))

		var up models.Upload
		if err := gdb.Where("profile_id = ? AND file_name = ?", profile.ID, name).First(&up).Error; err == nil {
			fmt.Printf("exists: %s -> %s\n", name, up.StorePath)
			if up.KeuanganID == nil {
				if *dry {
					fmt.Printf("DRY: would create CatatanKeuangan and link to upload %d\n", up.ID)
				} else {
					cat := models.CatatanKeuangan{UserID: profile.UserID, FileName: name, Amount: 0, Date: time.Now()}
					if err := gdb.Create(&cat).Error; err != nil {
						log.Printf("create catatan failed for %s: %v", name, err)
					} else {
						up.KeuanganID = &cat.ID
						_ = gdb.Save(&up).Error
						fmt.Printf("created catatan id=%d and linked to upload %d\n", cat.ID, up.ID)
					}
				}
			}
			continue
		}

		if *dry {
			fmt.Printf("DRY: would create Upload profile=%d file=%s store=%s\n", profile.ID, name, store)
			fmt.Printf("DRY: would create CatatanKeuangan user=%d file=%s amount=0\n", profile.UserID, name)
			continue
		}

		newUp := models.Upload{FileName: name, StorePath: store, ProfileID: profile.ID, ContentType: "application/octet-stream"}
		if err := gdb.Create(&newUp).Error; err != nil {
			log.Printf("create upload failed for %s: %v", name, err)
			continue
		}
		cat := models.CatatanKeuangan{UserID: profile.UserID, FileName: name, Amount: 0, Date: time.Now()}
		if err := gdb.Create(&cat).Error; err != nil {
			log.Printf("create catatan failed for %s: %v", name, err)
		} else {
			newUp.KeuanganID = &cat.ID
			_ = gdb.Save(&newUp).Error
			fmt.Printf("created upload id=%d and catatan id=%d\n", newUp.ID, cat.ID)
		}
	}
}
