package main

import (
	"flag"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/fsnotify/fsnotify"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"be03/models"
	"be03/pkg/ocr"
)

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

// Global DB handle for helper funcs
var db *gorm.DB

// global flags (parsed in main)
var (
	verbose     bool
	simulateOCR bool
)

// (no global status server)

// MIME mapping to avoid opening files repeatedly
var extMime = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".txt":  "text/plain",
}

// preload caches
type preloadState struct {
	uploadsByFile map[string]*models.Upload          // fileName -> upload
	catByFile     map[string]*models.CatatanKeuangan // fileName -> catatan
	mu            sync.RWMutex
}

func newPreloadState() *preloadState {
	return &preloadState{
		uploadsByFile: make(map[string]*models.Upload, 1024),
		catByFile:     make(map[string]*models.CatatanKeuangan, 1024),
	}
}

func (ps *preloadState) getUpload(name string) (*models.Upload, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	u, ok := ps.uploadsByFile[name]
	return u, ok
}
func (ps *preloadState) putUpload(u *models.Upload) {
	ps.mu.Lock()
	ps.uploadsByFile[u.FileName] = u
	ps.mu.Unlock()
}
func (ps *preloadState) getCat(name string) (*models.CatatanKeuangan, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	c, ok := ps.catByFile[name]
	return c, ok
}
func (ps *preloadState) putCat(c *models.CatatanKeuangan) {
	ps.mu.Lock()
	ps.catByFile[c.FileName] = c
	ps.mu.Unlock()
}

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

// Main: scans a directory of image receipts, creates Upload rows, runs OCR to create/link CatatanKeuangan, optional watch mode.
func main() {
	dirFlag := flag.String("dir", "public/keu", "directory to scan for receipt images")
	profileID := flag.Uint("profile-id", 0, "Profile ID to assign uploads to (if omitted attempts admin profile)")
	dryRun := flag.Bool("dry-run", false, "Skip all DB queries and writes; just list / optionally OCR (see --simulate-ocr)")
	watch := flag.Bool("watch", false, "Watch directory for new files")
	workers := flag.Int("workers", 0, "Worker pool size (default NumCPU)")
	flag.BoolVar(&verbose, "verbose", false, "Verbose per-file logging")
	flag.BoolVar(&simulateOCR, "simulate-ocr", false, "In dry-run: actually run OCR to show potential amounts")
	flag.Parse()

	if *dryRun {
		// fast dry-run path (no DB) unless profile-id required for parity; we only need DB if not dry-run
		log.Printf("Dry-run: scanning %s (no DB interaction)", *dirFlag)
		files := listImageFiles(*dirFlag)
		log.Printf("Found %d candidate files", len(files))
		if simulateOCR {
			for _, f := range files {
				if amt, conf, found, err := ocr.ExtractAmountFromImage(filepath.Join(*dirFlag, f)); err == nil && amt > 0 {
					if found != "" {
						lf := strings.TrimSpace(found)
						if strings.Contains(lf, ".") || strings.HasSuffix(lf, ",00") || strings.HasSuffix(lf, ".00") {
							if amt%100 == 0 {
								amt = amt / 100
							}
						}
					}
					logV("OCR %s amount=%d conf=%.2f found=%s", f, amt, conf, found)
				}
			}
		}
		return
	}

	db = mustInitDBFromEnv()
	profile := resolveProfile(*profileID)
	// preload all uploads & catatan
	ps := preloadAll(profile)
	// no global status server
	log.Printf("Preloaded: uploads=%d catatan=%d", len(ps.uploadsByFile), len(ps.catByFile))

	// gather initial file list
	files := listImageFiles(*dirFlag)
	log.Printf("Scanning %d files (workers=%d)", len(files), effectiveWorkers(*workers))
	runWorkerPool(*dirFlag, profile, ps, files, effectiveWorkers(*workers))

	if *watch {
		// start watching without exposing HTTP status server
		if err := watchDirectory(*dirFlag, profile, ps, effectiveWorkers(*workers)); err != nil {
			log.Fatalf("watch failed: %v", err)
		}
	}
}

func effectiveWorkers(w int) int {
	if w <= 0 {
		return runtime.NumCPU()
	}
	return w
}

func logV(format string, args ...any) {
	if verbose {
		log.Printf(format, args...)
	}
}

// chooseBestAmount parses OCR matches and returns the most plausible amount and raw string.
// Heuristics:
// - parse all matches; apply cents scaling only when string ends with two decimals
// - ignore tiny values (< 1000)
// - prefer numbers with currency hints ("rp", "idr") and/or thousands separators
// - otherwise take the numerically largest
func chooseBestAmount(matches []string) (best int64, bestRaw string) {
	// first pass: currency hinted
	for _, m := range matches {
		raw := strings.TrimSpace(m)
		low := strings.ToLower(raw)
		if !strings.Contains(low, "rp") && !strings.Contains(low, "idr") {
			continue
		}
		amt, err := ocr.ParseAmountFromMatch(raw)
		if err != nil || amt <= 0 {
			continue
		}
		if centsRE.MatchString(raw) && amt%100 == 0 {
			amt /= 100
		}
		if amt < 1000 {
			continue
		}
		if amt > best {
			best, bestRaw = amt, raw
		}
	}
	if best > 0 {
		return
	}
	// second pass: prefer with grouping separators
	for _, m := range matches {
		raw := strings.TrimSpace(m)
		if !(strings.Contains(raw, ".") || strings.Contains(raw, ",")) {
			continue
		}
		amt, err := ocr.ParseAmountFromMatch(raw)
		if err != nil || amt <= 0 {
			continue
		}
		if centsRE.MatchString(raw) && amt%100 == 0 {
			amt /= 100
		}
		if amt < 1000 {
			continue
		}
		if amt > best {
			best, bestRaw = amt, raw
		}
	}
	if best > 0 {
		return
	}
	// final pass: largest numeric
	for _, m := range matches {
		raw := strings.TrimSpace(m)
		amt, err := ocr.ParseAmountFromMatch(raw)
		if err != nil || amt <= 0 {
			continue
		}
		if centsRE.MatchString(raw) && amt%100 == 0 {
			amt /= 100
		}
		if amt < 1000 {
			continue
		}
		if amt > best {
			best, bestRaw = amt, raw
		}
	}
	return
}

// preloadAll fetches existing uploads and catatan to minimize per-file queries.
func preloadAll(profile models.Profile) *preloadState {
	ps := newPreloadState()
	var ups []models.Upload
	if err := db.Where("profile_id = ?", profile.ID).Find(&ups).Error; err == nil {
		for i := range ups {
			u := ups[i]
			ps.uploadsByFile[u.FileName] = &u
		}
	}
	var cats []models.CatatanKeuangan
	if err := db.Where("user_id = ?", profile.UserID).Find(&cats).Error; err == nil {
		for i := range cats {
			c := cats[i]
			ps.catByFile[c.FileName] = &c
		}
	}
	return ps
}

// resolveProfile finds the profile either by explicit id or by admin username.
func resolveProfile(id uint) models.Profile {
	var p models.Profile
	if id != 0 {
		if err := db.First(&p, id).Error; err != nil {
			log.Fatalf("failed to find profile id %d: %v", id, err)
		}
		return p
	}
	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		log.Fatalf("no --profile-id provided and admin user not found: %v", err)
	}
	if err := db.Where("user_id = ?", admin.ID).First(&p).Error; err != nil {
		log.Fatalf("admin profile not found: %v", err)
	}
	return p
}

func listImageFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// include all files except OCR temp artifacts; processing will decide
		// whether extension is supported and set proper failure messages.
		if strings.Contains(e.Name(), ".ocr.") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func watchDirectory(dir string, profile models.Profile, ps *preloadState, workers int) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		return err
	}
	log.Printf("Watching %s (debounced) ...", dir)

	fileCh := make(chan string, 256)
	go func() {
		// simple debounce map of pending files
		pending := map[string]time.Time{}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					close(fileCh)
					return
				}
				if ev.Op&fsnotify.Create == fsnotify.Create {
					name := filepath.Base(ev.Name)
					// ignore OCR temp files; otherwise allow all created files so
					// we can surface 'file not recognized' for unsupported types.
					if strings.Contains(name, ".ocr.") {
						continue
					}
					pending[name] = time.Now()
				}
			case <-ticker.C:
				now := time.Now()
				for name, t := range pending {
					if now.Sub(t) > 300*time.Millisecond { // stable
						fileCh <- name
						delete(pending, name)
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					close(fileCh)
					return
				}
				log.Printf("watch error: %v", err)
			}
		}
	}()

	// Use worker pool for watch events too
	go runWorkerPool(dir, profile, ps, nil, workers, fileCh)
	// block forever (Ctrl+C to exit)
	select {}
}

func isSupportedExt(name string) bool {
	// ignore OCR-generated temp files to avoid recursive processing
	if strings.Contains(name, ".ocr.") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

// processSingleFile executes idempotent logic to create/fill Upload & Catatan.
// worker pool orchestrator
func runWorkerPool(dir string, profile models.Profile, ps *preloadState, initial []string, workers int, extraCh ...<-chan string) {
	fileCh := make(chan string, 1024)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range fileCh {
				processSingleFile(dir, name, profile, ps)
			}
		}()
	}
	// feed initial
	go func() {
		for _, f := range initial {
			fileCh <- f
		}
		// also relay from extra channels if provided
		for _, ch := range extraCh {
			go func(c <-chan string) {
				for n := range c {
					fileCh <- n
				}
			}(ch)
		}
		// if no extraCh (scan only) close when done
		if len(extraCh) == 0 {
			close(fileCh)
		}
	}()
	if len(extraCh) == 0 {
		wg.Wait()
	}
}

// processSingleFile processes a single filename using preloaded maps & minimal queries.
func processSingleFile(dir, name string, profile models.Profile, ps *preloadState) {
	storePath := filepath.ToSlash(filepath.Join("public", filepath.Base(dir), name))
	filePath := filepath.Join(dir, name)

	if _, ok := ps.getCat(name); ok { // catatan already exists
		logV("SKIP catatan exists %s", name)
		return
	}
	up, upExists := ps.getUpload(name)
	// Retry a few times to allow API handler to create Upload row before watcher races to create its own
	if !upExists {
		for attempt := 0; attempt < 3 && !upExists; attempt++ {
			var dbUp models.Upload
			if err := db.Where("store_path = ? OR file_name = ?", storePath, name).First(&dbUp).Error; err == nil {
				up = &dbUp
				upExists = true
				ps.putUpload(up)
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	if upExists && up.KeuanganID != nil { // already linked
		logV("SKIP upload already linked %s", name)
		return
	}

	// Only run OCR if no catatan & (no upload OR upload without linkage)
	var amt int64
	var bestRaw string
	// defer heavy OCR until after we know we might need it
	needOCR := true

	// if extension is not supported (e.g., .pdf handled elsewhere or .exe/text),
	// create upload and mark as not recognized so front-end sees the proper message.
	if !isSupportedExt(name) {
		// create upload if not exists (above logic will create it), but if it exists
		// we still set Failed/FailedReason accordingly.
		// Note: proceed to create upload by leaving upExists handling unchanged.
	}

	// If upload doesn't exist, create it (DB write). Do not create under admin profile.
	if !upExists {
		if profile.UserID == 1 {
			log.Printf("SKIP creating upload for admin profile (user_id=1) file=%s", name)
			if err := moveToProcessed(filepath.Join(dir, name), name); err != nil {
				log.Printf("WARN failed to move processed file %s: %v", name, err)
			}
			return
		}
		newUp := models.Upload{ProfileID: profile.ID, FileName: name, StorePath: storePath}
		if ct := mimeFromExt(name); ct != "" {
			newUp.ContentType = ct
		}
		if err := db.Create(&newUp).Error; err != nil {
			if isUniqueConstraintError(err) { // race: someone else created
				if err2 := db.Where("store_path = ?", storePath).First(&newUp).Error; err2 != nil {
					log.Printf("WARN fetch after race failed %s: %v", storePath, err2)
					return
				}
			} else {
				log.Printf("ERROR create upload %s: %v", storePath, err)
				return
			}
		}
		ps.putUpload(&newUp)
		up = &newUp
		log.Printf("NEW upload id=%d file=%s", newUp.ID, name)
	}

	// Fill missing content type cheaply
	if up.ContentType == "" {
		if ct := mimeFromExt(name); ct != "" {
			up.ContentType = ct
			_ = db.Save(up).Error
		}
	}

	if needOCR {
		// Use FindAllMatches to detect zero / multiple matches cases
		matches, isLikelyNonAmount, mErr := ocr.FindAllMatches(filePath)
		if mErr != nil {
			logV("OCR fail %s: %v", name, mErr)
			return
		}
		if len(matches) == 0 {
			// no amount: differentiate logo-like images vs generic no-digits
			up.Failed = true
			if isLikelyNonAmount {
				log.Printf("NO AMOUNT / likely non-amount for %s: marking upload failed and moving file to failed", name)
				up.FailedReason = "File tidak dikenali, gunakan file lain!"
				_ = db.Save(up).Error
				_ = moveToFailed(filePath, name)
				return
			}
			log.Printf("NO AMOUNT found for %s: marking upload failed and moving file to failed", name)
			up.FailedReason = "Nominal tidak ditemukan, gunakan file lain"
			_ = db.Save(up).Error
			_ = moveToFailed(filePath, name)
			return
		}
		// Choose the best amount from all matches
		if bAmt, bRaw := chooseBestAmount(matches); bAmt > 0 {
			amt, bestRaw = bAmt, bRaw
		} else {
			// Fallback: try a full-image extraction which may catch the primary amount
			if fAmt, _, fFound, ferr := ocr.ExtractAmountFromImage(filePath); ferr == nil && fAmt > 0 {
				amt, bestRaw = fAmt, fFound
			} else {
				// Could not determine amount
				up.Failed = true
				up.FailedReason = "Nominal tidak ditemukan, gunakan file lain"
				_ = db.Save(up).Error
				_ = moveToFailed(filePath, name)
				return
			}
		}
	}

	// Re-check if catatan created concurrently
	if _, ok := ps.getCat(name); ok {
		return
	}

	// by here, amt must be > 0
	if amt <= 0 {
		return
	}

	// Resolve owner from Upload (retry if needed). Do NOT default to admin; determine from upload/profile.
	var ownerUserID uint = 0
	for i := 0; i < 3 && up == nil; i++ { // small retry to avoid race
		if !upExists {
			var dbUp models.Upload
			if err := db.Where("store_path = ? OR file_name = ?", storePath, name).First(&dbUp).Error; err == nil {
				up = &dbUp
				upExists = true
				ps.putUpload(up)
			}
		}
		if up != nil {
			var ownerProfile models.Profile
			if err := db.First(&ownerProfile, up.ProfileID).Error; err == nil {
				ownerUserID = ownerProfile.UserID
			}
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// If owner couldn't be determined, as a safety do not attribute to admin implicitly.
	if ownerUserID == 0 {
		log.Printf("SKIP unknown owner for %s: no upload owner resolved; not creating catatan", name)
		if err := moveToProcessed(filepath.Join(dir, name), name); err != nil {
			log.Printf("WARN failed to move processed file %s: %v", name, err)
		}
		return
	}

	// Never attribute to admin (user_id=1) per business rule.
	if ownerUserID == 1 {
		log.Printf("SKIP admin ownership for %s: not creating catatan for admin (user_id=1)", name)
		if err := moveToProcessed(filepath.Join(dir, name), name); err != nil {
			log.Printf("WARN failed to move processed file %s: %v", name, err)
		}
		return
	}

	// Create or fetch catatan for the correct owner
	cat := models.CatatanKeuangan{UserID: ownerUserID, FileName: name, Amount: amt, Date: time.Now()}
	if err := db.Create(&cat).Error; err != nil {
		var existing models.CatatanKeuangan
		if err2 := db.Where("user_id = ? AND file_name = ?", ownerUserID, name).First(&existing).Error; err2 == nil {
			// Optionally update amount if new detection is clearly larger (e.g., fix from 20285 -> 600000)
			if amt > existing.Amount && amt >= existing.Amount*2 {
				existing.Amount = amt
				_ = db.Save(&existing).Error
			}
			cat = existing
		} else {
			log.Printf("ERROR creating catatan for %s owner=%d: %v", name, ownerUserID, err)
			return
		}
	}
	// Link upload if present
	if up != nil && up.KeuanganID == nil {
		up.KeuanganID = &cat.ID
		_ = db.Save(up).Error
	}
	log.Printf("Pencatatan Sukses amount=%d raw=%q owner=%d file=%s", amt, bestRaw, ownerUserID, name)
	// Move the processed file out of public/keu into public/processed so new images are processed only once
	if err := moveToProcessed(filepath.Join(dir, name), name); err != nil {
		log.Printf("WARN failed to move processed file %s: %v", name, err)
	} else {
		logV("moved processed %s to public/processed", name)
	}
}

// fillUpload ensures ContentType and KeuanganID present (creates Catatan if OCR finds amount)
// legacy fillUpload removed (logic integrated in processSingleFile with preload state)

// sniffContentType reads first 512 bytes and returns MIME type.
func sniffContentType(path string) string { // fallback only
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}
	return http.DetectContentType(buf[:n])
}

func mimeFromExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if m, ok := extMime[ext]; ok {
		return m
	}
	return "" // sniff later if needed
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key") || strings.Contains(s, "unique constraint") || strings.Contains(s, "already exists")
}

// moveToProcessed moves a file from public/keu to public/processed/<name>.
// It attempts an atomic rename and falls back to copy+remove when necessary.
func moveToProcessed(srcFullPath, name string) error {
	const maxBytes = 1_000_000 // 1 MB budget
	processedDir := filepath.Join("public", "processed")
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(processedDir, name)

	fi, err := os.Stat(srcFullPath)
	if err != nil {
		return err
	}
	// Fast path: already small enough -> attempt rename/copy
	if fi.Size() <= maxBytes {
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	// Need compression / resizing
	img, err := imaging.Open(srcFullPath)
	if err != nil { // fallback to raw move if cannot decode
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	// Estimate scale factor based on sqrt(max/current) (size roughly scales with area)
	scale := math.Sqrt(float64(maxBytes) / float64(fi.Size()))
	if scale > 0.95 { // still enforce some small reduction to help container formats
		scale = 0.95
	}
	if scale < 0.1 { // avoid absurd downscale
		scale = 0.1
	}
	if scale < 1 {
		w := img.Bounds().Dx()
		h := img.Bounds().Dy()
		newW := int(math.Max(1, math.Round(float64(w)*scale)))
		newH := int(math.Max(1, math.Round(float64(h)*scale)))
		img = imaging.Resize(img, newW, newH, imaging.Lanczos)
	}
	// Save to dst (overwrite if exists)
	if err := imaging.Save(img, dst); err != nil {
		// fallback to original move
		if err := os.Rename(srcFullPath, dst); err == nil {
			return nil
		}
		return copyRemove(srcFullPath, dst)
	}
	// Remove original after successful save
	_ = os.Remove(srcFullPath)
	// If still > maxBytes, try one more uniform 80% scale pass
	if fi2, err2 := os.Stat(dst); err2 == nil && fi2.Size() > maxBytes {
		img2, errOpen2 := imaging.Open(dst)
		if errOpen2 == nil {
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

// moveToFailed moves a file to public/failed preserving the original filename.
// It behaves similarly to moveToProcessed but without image re-encoding.
func moveToFailed(srcFullPath, name string) error {
	failedDir := filepath.Join("public", "failed")
	if err := os.MkdirAll(failedDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(failedDir, name)
	if err := os.Rename(srcFullPath, dst); err == nil {
		return nil
	}
	return copyRemove(srcFullPath, dst)
}

// chooseBestMatch tries to pick the most likely amount string from multiple OCR matches.
// It returns (chosenMatch, parsedAmount, ok). The heuristic prefers strings containing
// an explicit "Rp" or the largest numeric value (assuming totals are larger than ids).
func chooseBestMatch(matches []string) (string, int64, bool) {
	if len(matches) == 0 {
		return "", 0, false
	}
	// prefer matches that contain Rp or other currency hints
	for _, m := range matches {
		if strings.Contains(strings.ToLower(m), "rp") || strings.Contains(strings.ToLower(m), "idr") {
			if a, err := ocr.ParseAmountFromMatch(m); err == nil && a > 0 {
				return m, a, true
			}
		}
	}
	// otherwise choose the numerically largest valid parse
	var best string
	var bestAmt int64
	for _, m := range matches {
		if a, err := ocr.ParseAmountFromMatch(m); err == nil {
			if a > bestAmt {
				bestAmt = a
				best = m
			}
		}
	}
	if bestAmt > 0 {
		return best, bestAmt, true
	}
	return "", 0, false
}
