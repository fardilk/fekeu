package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"be03/pkg/ocr"

	_ "github.com/lib/pq"
)

var centsRE = regexp.MustCompile(`[.,]\d{2}$`)

func main() {
	user := flag.String("user", "fardiluser", "username to fix files for")
	dir := flag.String("dir", "public/keu", "base dir for files")
	flag.Parse()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT ck.id, ck.file_name FROM catatan_keuangans ck JOIN users u ON u.id=ck.user_id WHERE u.username=$1`, *user)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var fname string
		if err := rows.Scan(&id, &fname); err != nil {
			log.Printf("scan: %v", err)
			continue
		}
		full := filepath.Join(*dir, fname)
		amt, _, found, err := ocr.ExtractAmountFromImage(full)
		if err != nil {
			log.Printf("ocr %s: %v", full, err)
			continue
		}
		if amt <= 0 {
			log.Printf("no amt for id=%d file=%s", id, fname)
			continue
		}

		// normalize if found indicates cents
		if strings.TrimSpace(found) != "" && centsRE.MatchString(strings.TrimSpace(found)) {
			if amt%100 == 0 {
				log.Printf("normalizing for %s: %d -> %d (found=%s)", fname, amt, amt/100, found)
				amt = amt / 100
			}
		}

		if _, err := db.Exec(`UPDATE catatan_keuangans SET amount=$1, date=now() WHERE id=$2`, amt, id); err != nil {
			log.Printf("update id=%d: %v", id, err)
			continue
		}
		fmt.Printf("fixed id=%d file=%s amount=%d found=%q\n", id, fname, amt, found)
	}
}
