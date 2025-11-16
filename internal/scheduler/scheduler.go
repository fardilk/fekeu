package scheduler

import (
	"log"
	"time"

	"gorm.io/gorm"

	"be03/internal/config"
	"be03/models"
)

// Scheduler handles periodic background jobs
type Scheduler struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewScheduler creates a new scheduler
func NewScheduler(cfg *config.Config, db *gorm.DB) *Scheduler {
	return &Scheduler{
		cfg: cfg,
		db:  db,
	}
}

// Start begins running periodic jobs
func (s *Scheduler) Start() {
	log.Println("[SCHEDULER] Starting background jobs...")

	retryInterval, _ := time.ParseDuration(s.cfg.RetryInterval)
	cleanupInterval, _ := time.ParseDuration(s.cfg.CleanupInterval)

	if retryInterval == 0 {
		retryInterval = 10 * time.Minute
	}
	if cleanupInterval == 0 {
		cleanupInterval = 1 * time.Hour
	}

	retryTicker := time.NewTicker(retryInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)

	defer retryTicker.Stop()
	defer cleanupTicker.Stop()

	// Run immediately on start
	s.retryFailedOCR()
	s.cleanup()

	for {
		select {
		case <-retryTicker.C:
			s.retryFailedOCR()
		case <-cleanupTicker.C:
			s.cleanup()
		}
	}
}

// StartWithRecovery runs the scheduler with automatic recovery on panic
func (s *Scheduler) StartWithRecovery() {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[SCHEDULER] PANIC recovered: %v", r)
				}
			}()
			s.Start()
		}()
		log.Printf("[SCHEDULER] Restarting in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}

func (s *Scheduler) retryFailedOCR() {
	log.Println("[SCHEDULER] Running OCR retry job...")

	var failedUploads []models.Upload
	result := s.db.Where("failed = ? AND created_at > ?", true, time.Now().Add(-24*time.Hour)).
		Limit(50).
		Find(&failedUploads)

	if result.Error != nil {
		log.Printf("[SCHEDULER] Failed to query failed uploads: %v", result.Error)
		return
	}

	if len(failedUploads) == 0 {
		log.Println("[SCHEDULER] No failed uploads to retry")
		return
	}

	log.Printf("[SCHEDULER] Found %d failed uploads to retry", len(failedUploads))

	// Reset failed status so watcher can pick them up
	for _, upload := range failedUploads {
		upload.Failed = false
		upload.FailedReason = ""
		if err := s.db.Save(&upload).Error; err != nil {
			log.Printf("[SCHEDULER] Failed to update upload %d: %v", upload.ID, err)
		} else {
			log.Printf("[SCHEDULER] Reset upload %d (%s) for retry", upload.ID, upload.FileName)
		}
	}
}

func (s *Scheduler) cleanup() {
	log.Println("[SCHEDULER] Running cleanup job...")

	// Delete old failed uploads (older than 7 days)
	result := s.db.Where("failed = ? AND created_at < ?", true, time.Now().Add(-7*24*time.Hour)).
		Delete(&models.Upload{})

	if result.Error != nil {
		log.Printf("[SCHEDULER] Cleanup failed: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("[SCHEDULER] Cleaned up %d old failed uploads", result.RowsAffected)
	} else {
		log.Println("[SCHEDULER] No old uploads to clean up")
	}

	// Clean up orphaned refresh tokens
	result = s.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshToken{})
	if result.Error == nil && result.RowsAffected > 0 {
		log.Printf("[SCHEDULER] Cleaned up %d expired refresh tokens", result.RowsAffected)
	}

	// Clean up old password reset records (if model exists)
	// Note: PasswordReset model may not exist in current schema
	// result = s.db.Where("created_at < ?", time.Now().Add(-24*time.Hour)).
	// 	Delete(&models.PasswordReset{})
	// if result.Error == nil && result.RowsAffected > 0 {
	// 	log.Printf("[SCHEDULER] Cleaned up %d old password reset records", result.RowsAffected)
	// }
}
