package main

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
	dir := flag.String("dir", "public/keu", "directory to scan for uploaded files")
	username := flag.String("username", "fardiluser", "username to assign uploads to")
	dry := flag.Bool("dry-run", true, "dry-run: don't write to DB")
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

	// Walk files under dir (non-recursive by default: top-level files + subdirs)
	err := filepath.WalkDir(*dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("public", path)
		// store path should be posix style
		storePath := filepath.ToSlash(filepath.Join("public", rel))
		fileName := d.Name()

		// Normalize filename for deterministic prefix if needed
		pref := prefixFor(fileName, profile.ID)
		// expected store layout public/keu/<aa>/<bb>/<filename>
		expected := filepath.ToSlash(filepath.Join("public/keu", pref, fileName))

		// Use existing storePath if it already matches expected or is under public/keu
		chosenStore := storePath
		if !strings.HasPrefix(storePath, "public/keu/") {
			chosenStore = expected
		}

		// Check existing upload
		var up models.Upload
		err = gdb.Where("profile_id = ? AND file_name = ?", profile.ID, fileName).First(&up).Error
		if err == nil {
			fmt.Printf("EXISTS: upload id=%d file=%s store=%s\n", up.ID, fileName, up.StorePath)
			// ensure catatan exists and link if missing
			if up.KeuanganID == nil {
				if *dry {
					fmt.Printf("DRY: would create CatatanKeuangan and link to upload id=%d\n", up.ID)
				} else {
					cat := models.CatatanKeuangan{UserID: profile.UserID, FileName: fileName, Amount: 0, Date: time.Now()}
					if err := gdb.Create(&cat).Error; err != nil {
						log.Printf("create catatan failed for %s: %v", fileName, err)
					} else {
						up.KeuanganID = &cat.ID
						_ = gdb.Save(&up).Error
						fmt.Printf("created catatan id=%d and linked to upload %d\n", cat.ID, up.ID)
					}
				}
			}
			return nil
		}

		// create upload
		if *dry {
			fmt.Printf("DRY: would create Upload profile=%d file=%s store=%s\n", profile.ID, fileName, chosenStore)
			fmt.Printf("DRY: would create CatatanKeuangan user=%d file=%s amount=0\n", profile.UserID, fileName)
			return nil
		}

		newUp := models.Upload{FileName: fileName, StorePath: chosenStore, ProfileID: profile.ID, ContentType: "application/octet-stream"}
		if err := gdb.Create(&newUp).Error; err != nil {
			log.Printf("create upload failed for %s: %v", fileName, err)
			return nil
		}
		cat := models.CatatanKeuangan{UserID: profile.UserID, FileName: fileName, Amount: 0, Date: time.Now()}
		if err := gdb.Create(&cat).Error; err != nil {
			log.Printf("create catatan failed for %s: %v", fileName, err)
		} else {
			newUp.KeuanganID = &cat.ID
			_ = gdb.Save(&newUp).Error
			fmt.Printf("created upload id=%d and catatan id=%d\n", newUp.ID, cat.ID)
		}

		return nil
	})
	if err != nil {
		log.Fatalf("walk error: %v", err)
	}
}
