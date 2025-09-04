package ocrupdater

import (
	"fmt"
	"io"
	"log"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"be03/models"

	"os"

	"github.com/disintegration/imaging"
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
	const maxBytes = 1_000_000
	processedDir := filepath.Join("public", "processed")
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(processedDir, name)
	fi, err := os.Stat(srcFullPath)
	if err != nil {
		return err
	}
	if fi.Size() <= maxBytes { // fast path rename/copy
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	img, err := imaging.Open(srcFullPath)
	if err != nil { // fallback raw
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	scale := math.Sqrt(float64(maxBytes) / float64(fi.Size()))
	if scale > 0.95 {
		scale = 0.95
	}
	if scale < 0.1 {
		scale = 0.1
	}
	if scale < 1 {
		w := img.Bounds().Dx()
		h := img.Bounds().Dy()
		nw := int(math.Max(1, math.Round(float64(w)*scale)))
		nh := int(math.Max(1, math.Round(float64(h)*scale)))
		img = imaging.Resize(img, nw, nh, imaging.Lanczos)
	}
	if err := imaging.Save(img, dst); err != nil {
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	_ = os.Remove(srcFullPath)
	if fi2, err2 := os.Stat(dst); err2 == nil && fi2.Size() > maxBytes {
		if img2, errOpen2 := imaging.Open(dst); errOpen2 == nil {
			img2 = imaging.Resize(img2, int(float64(img2.Bounds().Dx())*0.8), 0, imaging.Lanczos)
			_ = imaging.Save(img2, dst)
		}
	}
	return nil
}

func copyRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	_ = out.Close()
	if err := os.Remove(src); err != nil {
		return err
	}
	return nil
}
