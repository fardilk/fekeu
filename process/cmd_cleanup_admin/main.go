package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var adminID sql.NullInt64
	if err := db.QueryRow(`SELECT id FROM users WHERE username='admin' LIMIT 1`).Scan(&adminID); err != nil {
		log.Fatalf("find admin: %v", err)
	}
	if !adminID.Valid {
		fmt.Println("admin user not found; nothing to cleanup")
		return
	}
	// Nullify FKs first
	res1, err := db.Exec(`UPDATE uploads SET keuangan_id=NULL WHERE keuangan_id IN (SELECT id FROM catatan_keuangans WHERE user_id=$1)`, adminID.Int64)
	if err != nil {
		log.Fatalf("nullify uploads FK: %v", err)
	}
	n1, _ := res1.RowsAffected()
	// Delete admin catatan
	res2, err := db.Exec(`DELETE FROM catatan_keuangans WHERE user_id=$1`, adminID.Int64)
	if err != nil {
		log.Fatalf("delete admin catatan: %v", err)
	}
	n2, _ := res2.RowsAffected()
	fmt.Printf("cleanup done: uploads unlinked=%d, catatan deleted=%d\n", n1, n2)
}
