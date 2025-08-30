package ocrupdater

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"be03/models"

	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"be03/pkg/ocr"
)

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

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

// Run scans dir for files, performs OCR, and updates CatatanKeuangan.Amount and Date
// If dry true, only prints proposed changes.
func Run(dir string, dry bool, minConf float64) error {
	gdb := mustDBFromEnv()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		full := filepath.Join(dir, name)
		amt, conf, found, err := ocr.ExtractAmountFromImage(full)
		if err != nil {
			log.Printf("ocr error %s: %v", name, err)
			continue
		}
		if amt <= 0 || conf < minConf {
			log.Printf("ocr skipped %s amt=%d conf=%.2f (min=%.2f)", name, amt, conf, minConf)
			continue
		}

		// Normalize only when original matched string contains decimal/cents (like .00 or ,00)
		if found != "" {
			lf := strings.TrimSpace(found)
			if centsRE.MatchString(lf) {
				if amt > 0 && amt%100 == 0 {
					norm := amt / 100
					log.Printf("normalizing OCR amount for %s: %d -> %d (found=%s)", name, amt, norm, found)
					amt = norm
				}
			}
		}

		// find the catatan for this filename (assume unique per user)
		var cat models.CatatanKeuangan
		if err := gdb.Where("file_name = ?", name).First(&cat).Error; err != nil {
			log.Printf("no catatan found for %s: %v", name, err)
			continue
		}

		if dry {
			fmt.Printf("DRY: would update catatan id=%d file=%s old_amount=%d new_amount=%d conf=%.2f\n", cat.ID, name, cat.Amount, amt, conf)
			continue
		}

		cat.Amount = amt
		cat.Date = time.Now()
		if err := gdb.Save(&cat).Error; err != nil {
			log.Printf("failed update catatan %s: %v", name, err)
		} else {
			fmt.Printf("updated catatan id=%d file=%s amount=%d\n", cat.ID, name, amt)

			// after successful DB update, move the processed file to public/processed
			if err := moveToProcessed(full, name); err != nil {
				log.Printf("WARN failed to move processed file %s: %v", name, err)
			} else {
				log.Printf("moved processed %s to public/processed", name)
			}
		}
	}
	return nil
}

// moveToProcessed moves a file from public/keu to public/processed/<name>.
// It attempts an atomic rename and falls back to copy+remove when necessary.
func moveToProcessed(srcFullPath, name string) error {
	processedDir := filepath.Join("public", "processed")
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(processedDir, name)
	// try rename
	if err := os.Rename(srcFullPath, dst); err == nil {
		return nil
	}
	// fallback: copy then remove
	in, err := os.Open(srcFullPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		// ignore
	}
	if err := os.Remove(srcFullPath); err != nil {
		return err
	}
	return nil
}
