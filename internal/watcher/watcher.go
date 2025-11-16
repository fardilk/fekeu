package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fsnotify/fsnotify"
	"gorm.io/gorm"

	"be03/internal/config"
	"be03/models"
	"be03/pkg/ocr"
)

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

type Watcher struct {
	db      *gorm.DB
	cfg     *config.Config
	watcher *fsnotify.Watcher
	preload *preloadState
	ctx     context.Context
	cancel  context.CancelFunc
}

type preloadState struct {
	mu            sync.RWMutex
	uploadsByFile map[string]*models.Upload
	catByFile     map[string]*models.CatatanKeuangan
}

func New(db *gorm.DB, cfg *config.Config) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		db:      db,
		cfg:     cfg,
		watcher: fsWatcher,
		preload: &preloadState{
			uploadsByFile: make(map[string]*models.Upload),
			catByFile:     make(map[string]*models.CatatanKeuangan),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	return w, nil
}

func (w *Watcher) Start() {
	log.Println("[WATCHER] Starting file watcher...")

	w.preloadExistingData()

	watchDir := w.cfg.WatchDir
	if err := w.watcher.Add(watchDir); err != nil {
		log.Printf("[WATCHER] Failed to watch directory %s: %v", watchDir, err)
		return
	}
	log.Printf("[WATCHER] Watching directory: %s", watchDir)

	w.processExistingFiles(watchDir)

	pending := make(map[string]time.Time)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Println("[WATCHER] Context cancelled, stopping watcher")
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				log.Println("[WATCHER] Watcher events channel closed")
				return
			}

			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if isImageFile(event.Name) {
					pending[event.Name] = time.Now()
					log.Printf("[WATCHER] File detected: %s (waiting for stability)", filepath.Base(event.Name))
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				log.Println("[WATCHER] Watcher errors channel closed")
				return
			}
			log.Printf("[WATCHER] Error: %v", err)

		case <-ticker.C:
			now := time.Now()
			for filePath, detectedAt := range pending {
				if now.Sub(detectedAt) > 300*time.Millisecond {
					delete(pending, filePath)
					go w.processFile(filePath, filepath.Base(filePath))
				}
			}
		}
	}
}

func (w *Watcher) Stop() {
	log.Println("[WATCHER] Stopping watcher...")
	w.cancel()
	if w.watcher != nil {
		w.watcher.Close()
	}
}

func (w *Watcher) StartWithRecovery() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[WATCHER] Recovered from panic: %v", r)
			time.Sleep(5 * time.Second)
			log.Println("[WATCHER] Restarting watcher...")
			w.StartWithRecovery()
		}
	}()
	w.Start()
}

func (w *Watcher) preloadExistingData() {
	log.Println("[WATCHER] Preloading existing uploads and catatan...")

	var uploads []models.Upload
	if err := w.db.Find(&uploads).Error; err != nil {
		log.Printf("[WATCHER] Failed to preload uploads: %v", err)
		return
	}

	var catatans []models.CatatanKeuangan
	if err := w.db.Find(&catatans).Error; err != nil {
		log.Printf("[WATCHER] Failed to preload catatan: %v", err)
		return
	}

	w.preload.mu.Lock()
	defer w.preload.mu.Unlock()

	for i := range uploads {
		w.preload.uploadsByFile[uploads[i].FileName] = &uploads[i]
	}

	for i := range catatans {
		w.preload.catByFile[catatans[i].FileName] = &catatans[i]
	}

	log.Printf("[WATCHER] Preloaded %d uploads and %d catatan", len(uploads), len(catatans))
}

func (w *Watcher) processExistingFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[WATCHER] Failed to read directory: %v", err)
		return
	}

	log.Printf("[WATCHER] Processing %d existing files...", len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && isImageFile(entry.Name()) {
			fullPath := filepath.Join(dir, entry.Name())
			go w.processFile(fullPath, entry.Name())
		}
	}
}

func (w *Watcher) processFile(fullPath, fileName string) {
	log.Printf("[WATCHER] Processing file: %s", fileName)

	w.preload.mu.RLock()
	existingUpload, existsInPreload := w.preload.uploadsByFile[fileName]
	w.preload.mu.RUnlock()

	if existsInPreload && existingUpload.KeuanganID != nil {
		log.Printf("[WATCHER] File %s already processed (KeuanganID=%d), skipping", fileName, *existingUpload.KeuanganID)
		return
	}

	var upload models.Upload
	for retry := 0; retry < 3; retry++ {
		if err := w.db.Where("file_name = ?", fileName).First(&upload).Error; err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if upload.ID == 0 {
		log.Printf("[WATCHER] No upload record found for %s, skipping", fileName)
		return
	}

	if upload.KeuanganID != nil {
		log.Printf("[WATCHER] File %s already has KeuanganID=%d, skipping", fileName, *upload.KeuanganID)
		return
	}

	log.Printf("[OCR] Running OCR on %s", fileName)
	matches, _, err := ocr.FindAllMatches(fullPath)
	if err != nil {
		log.Printf("[OCR] Failed to run OCR on %s: %v", fileName, err)
		w.moveToFailed(fullPath, fileName, fmt.Sprintf("OCR failed: %v", err))
		return
	}

	if len(matches) == 0 {
		log.Printf("[OCR] No amounts found in %s", fileName)
		w.moveToFailed(fullPath, fileName, "No amounts detected")
		return
	}

	log.Printf("[OCR] Found %d matches in %s", len(matches), fileName)

	bestMatch := w.chooseBestAmount(matches)
	if bestMatch == "" {
		log.Printf("[OCR] No valid amount found in %s", fileName)
		w.moveToFailed(fullPath, fileName, "No valid amount found")
		return
	}

	amountFloat, err := ocr.ParseAmountFromMatch(bestMatch)
	if err != nil {
		log.Printf("[OCR] Failed to parse amount from %s: %v", bestMatch, err)
		w.moveToFailed(fullPath, fileName, fmt.Sprintf("Parse error: %v", err))
		return
	}

	log.Printf("[OCR] Best match for %s: %s (%d)", fileName, bestMatch, amountFloat)

	catatan := models.CatatanKeuangan{
		FileName: fileName,
		Amount:   amountFloat,
		UserID:   upload.ProfileID,
		Date:     time.Now(),
	}

	if err := w.db.Create(&catatan).Error; err != nil {
		log.Printf("[DB] Failed to create catatan for %s: %v", fileName, err)
		w.moveToFailed(fullPath, fileName, fmt.Sprintf("DB error: %v", err))
		return
	}

	log.Printf("[DB] Created catatan ID=%d for %s with amount=%d", catatan.ID, fileName, amountFloat)

	if err := w.db.Model(&upload).Updates(map[string]interface{}{
		"keuangan_id":   catatan.ID,
		"failed":        false,
		"failed_reason": nil,
	}).Error; err != nil {
		log.Printf("[DB] Failed to update upload for %s: %v", fileName, err)
		return
	}

	log.Printf("[DB] Updated upload ID=%d with KeuanganID=%d", upload.ID, catatan.ID)

	w.preload.mu.Lock()
	upload.KeuanganID = &catatan.ID
	w.preload.uploadsByFile[fileName] = &upload
	w.preload.catByFile[fileName] = &catatan
	w.preload.mu.Unlock()

	w.moveToProcessed(fullPath, fileName)

	log.Printf("[WATCHER] Successfully processed %s", fileName)
}

func (w *Watcher) chooseBestAmount(matches []string) string {
	if len(matches) == 0 {
		return ""
	}

	var hinted []string
	for _, m := range matches {
		lower := strings.ToLower(m)
		if strings.Contains(lower, "rp") || strings.Contains(lower, "idr") {
			hinted = append(hinted, m)
		}
	}
	if len(hinted) == 1 {
		return hinted[0]
	}
	if len(hinted) > 1 {
		matches = hinted
	}

	var withSep []string
	for _, m := range matches {
		digitPart := extractDigits(m)
		if strings.Contains(digitPart, ".") || strings.Contains(digitPart, ",") {
			withSep = append(withSep, m)
		}
	}
	if len(withSep) == 1 {
		return withSep[0]
	}
	if len(withSep) > 1 {
		matches = withSep
	}

	var best string
	var bestVal int64
	for _, m := range matches {
		val, err := ocr.ParseAmountFromMatch(m)
		if err == nil && val > bestVal {
			bestVal = val
			best = m
		}
	}

	return best
}

func (w *Watcher) moveToProcessed(srcPath, fileName string) {
	processedDir := filepath.Join(filepath.Dir(srcPath), "..", "processed")
	if err := os.MkdirAll(processedDir, 0755); err != nil {
		log.Printf("[WATCHER] Failed to create processed directory: %v", err)
		return
	}

	dstPath := filepath.Join(processedDir, fileName)

	info, err := os.Stat(srcPath)
	if err == nil && info.Size() > 1024*1024 {
		log.Printf("[WATCHER] Compressing large file %s (%.2f MB)", fileName, float64(info.Size())/(1024*1024))

		img, err := imaging.Open(srcPath)
		if err == nil {
			bounds := img.Bounds()
			width := bounds.Dx()
			height := bounds.Dy()

			newWidth := int(float64(width) / 1.414)
			newHeight := int(float64(height) / 1.414)

			resized := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)
			if err := imaging.Save(resized, dstPath); err != nil {
				log.Printf("[WATCHER] Failed to save compressed image: %v", err)
				os.Rename(srcPath, dstPath)
			} else {
				os.Remove(srcPath)
				log.Printf("[WATCHER] Compressed and moved %s", fileName)
			}
		} else {
			os.Rename(srcPath, dstPath)
		}
	} else {
		if err := os.Rename(srcPath, dstPath); err != nil {
			log.Printf("[WATCHER] Failed to move %s to processed: %v", fileName, err)
		}
	}
}

func (w *Watcher) moveToFailed(srcPath, fileName, reason string) {
	failedDir := filepath.Join(filepath.Dir(srcPath), "..", "failed")
	if err := os.MkdirAll(failedDir, 0755); err != nil {
		log.Printf("[WATCHER] Failed to create failed directory: %v", err)
		return
	}

	dstPath := filepath.Join(failedDir, fileName)
	if err := os.Rename(srcPath, dstPath); err != nil {
		log.Printf("[WATCHER] Failed to move %s to failed: %v", fileName, err)
		return
	}

	var upload models.Upload
	if err := w.db.Where("file_name = ?", fileName).First(&upload).Error; err != nil {
		log.Printf("[DB] Failed to find upload for failed file %s: %v", fileName, err)
		return
	}

	if err := w.db.Model(&upload).Updates(map[string]interface{}{
		"failed":        true,
		"failed_reason": reason,
	}).Error; err != nil {
		log.Printf("[DB] Failed to update failed status for %s: %v", fileName, err)
	}

	log.Printf("[WATCHER] Moved %s to failed: %s", fileName, reason)
}

func isImageFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".bmp")
}

func extractDigits(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
