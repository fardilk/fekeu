package report

import (
	"database/sql"
	"fmt"
	"log"
	"os"
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

// RunReport prints a month-bounded report for username (month in YYYY-MM) and
// optionally lists matching catatan_keuangan rows.
func RunReport(username, month string, list bool) {
	gdb := mustDBFromEnv()

	var user models.User
	if err := gdb.Where("username = ?", username).First(&user).Error; err != nil {
		log.Fatalf("user not found: %v", err)
	}

	t, err := time.Parse("2006-01", month)
	if err != nil {
		log.Fatalf("invalid month format, expected YYYY-MM: %v", err)
	}
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	var total sql.NullFloat64
	var cnt int64
	if err := gdb.Raw(`SELECT COALESCE(SUM(amount),0) AS total, COUNT(*) AS cnt FROM catatan_keuangans WHERE user_id = ? AND date >= ? AND date < ?`, user.ID, start, end).Row().Scan(&total, &cnt); err != nil {
		log.Fatalf("query failed: %v", err)
	}

	fmt.Printf("Report for user=%s month=%s (UTC):\n", user.Username, month)
	fmt.Printf("  records=%d total_amount=%.2f\n", cnt, total.Float64)

	if list {
		var rows []models.CatatanKeuangan
		if err := gdb.Where("user_id = ? AND date >= ? AND date < ?", user.ID, start, end).Order("id").Find(&rows).Error; err != nil {
			log.Fatalf("fetch rows failed: %v", err)
		}
		for _, r := range rows {
			fmt.Printf("%d|%s|%d|%s|%s\n", r.ID, r.FileName, r.Amount, r.Date.Format(time.RFC3339), r.CreatedAt.Format(time.RFC3339))
		}
	}
}
