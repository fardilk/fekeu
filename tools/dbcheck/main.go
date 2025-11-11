package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	// Prefer DATABASE_URL (common platform var), fallback to DB_DSN
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DB_DSN")
	} else {
		// sanitize common platform URL variants (strip schema query param and ensure sslmode)
		if u, err := url.Parse(dsn); err == nil {
			q := u.Query()
			q.Del("schema")
			if q.Get("sslmode") == "" {
				q.Set("sslmode", "disable")
			}
			u.RawQuery = q.Encode()
			dsn = u.String()
		}
	}

	// If we have a postgres URL form convert to libpq key=value style so lib/pq doesn't pass unknown params
	if strings.HasPrefix(dsn, "postgres") {
		if u, err := url.Parse(dsn); err == nil {
			usr := ""
			pwd := ""
			if u.User != nil {
				usr = u.User.Username()
				pwd, _ = u.User.Password()
			}
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				port = "5432"
			}
			dbname := strings.TrimPrefix(u.Path, "/")
			q := u.Query()
			ssl := q.Get("sslmode")
			if ssl == "" {
				ssl = "disable"
			}
			dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, usr, pwd, dbname, ssl)
		}
	}
	if dsn == "" {
		log.Fatal("no DATABASE_URL or DB_DSN found in environment")
	}

	// Open with database/sql using lib/pq driver
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	// short timeout ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		fmt.Printf("CONNECT_FAIL: %v\n", err)
		os.Exit(2)
	}
	fmt.Println("CONNECT_OK")
}
