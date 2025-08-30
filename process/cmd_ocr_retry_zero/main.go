package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"be03/pkg/ocr"

	"github.com/disintegration/imaging"
	_ "github.com/lib/pq"
)

func main() {
	profile := flag.String("profile", "fardiluser", "username/profile to retry")
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

	rows, err := db.Query(`SELECT ck.id, ck.file_name, up.store_path FROM catatan_keuangans ck JOIN users u ON u.id=ck.user_id LEFT JOIN profiles p ON p.user_id=u.id LEFT JOIN uploads up ON up.file_name=ck.file_name AND up.profile_id=p.id WHERE u.username=$1 AND ck.amount=0`, *profile)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var fname string
		var store sql.NullString
		if err := rows.Scan(&id, &fname, &store); err != nil {
			log.Printf("scan: %v", err)
			continue
		}
		path := fname
		if store.Valid && store.String != "" {
			path = store.String
		} else {
			path = *dir + "/" + fname
		}

		// aggressive preprocessing: open, sharpen, increase contrast, save temp
		img, err := imaging.Open(path)
		if err != nil {
			log.Printf("open %s: %v", path, err)
			continue
		}
		proc := imaging.Sharpen(img, 2.0)
		proc = imaging.AdjustContrast(proc, 30)
		tmp := path + ".retry.png"
		if err := imaging.Save(proc, tmp); err != nil {
			log.Printf("save tmp %s: %v", tmp, err)
			continue
		}

		amt, conf, found, err := ocr.ExtractAmountFromImage(tmp)
		_ = os.Remove(tmp)
		if err != nil {
			log.Printf("ocr %s: %v", path, err)
			continue
		}
		if amt == 0 {
			log.Printf("no amount found for id=%d file=%s (found=%q conf=%.2f)", id, fname, found, conf)
			continue
		}

		// apply update
		if _, err := db.Exec(`UPDATE catatan_keuangans SET amount=$1 WHERE id=$2`, amt, id); err != nil {
			log.Printf("update id=%d: %v", id, err)
			continue
		}
		fmt.Printf("updated id=%d file=%s amount=%d conf=%.2f found=%q\n", id, fname, amt, conf, found)
	}
}
