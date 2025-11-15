package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	httpRouter "be03/internal/http"
	"be03/internal/persistence"
)

func main() {
	// Auto-load .env if present
	loadDotEnv()

	// Normalize DATABASE_URL to DB_DSN
	normalizeDatabaseURL()

	// Support migrate command
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		db, err := persistence.NewDB()
		if err != nil {
			log.Fatal("migration failed:", err)
		}
		log.Println("migration and seeding completed")
		_ = db
		return
	}

	// Initialize database
	db, err := persistence.NewDB()
	if err != nil {
		log.Fatal("failed to initialize database:", err)
	}

	// Build router
	router := httpRouter.NewRouter(db)

	// Start background watcher
	go startWatcherProcess()

	// Start server
	port := os.Getenv("PORT")
	if strings.TrimSpace(port) == "" {
		port = os.Getenv("SERVER_PORT")
	}
	if strings.TrimSpace(port) == "" {
		port = "8080"
	}

	addr := ":" + port
	log.Printf("Starting HTTP server on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal("server error:", err)
	}
}

func normalizeDatabaseURL() {
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		if os.Getenv("DB_DSN") == "" {
			if u, err := url.Parse(dbURL); err == nil {
				q := u.Query()
				q.Del("schema")
				if q.Get("sslmode") == "" {
					q.Set("sslmode", "disable")
				}
				u.RawQuery = q.Encode()
				_ = os.Setenv("DB_DSN", u.String())
			} else {
				_ = os.Setenv("DB_DSN", dbURL)
			}
		}
	}

	if dsn := os.Getenv("DB_DSN"); strings.HasPrefix(dsn, "postgres") {
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
			libpq := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, usr, pwd, dbname, ssl)
			_ = os.Setenv("DB_DSN", libpq)
		}
	}
}

func startWatcherProcess() {
	_ = os.MkdirAll("logs", 0755)
	logfile := filepath.Join("logs", "watcher.log")
	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("failed to open watcher log: %v", err)
		return
	}
	cmd := exec.Command("go", "run", "process/process_keu.go", "-dir", "public/keu", "-watch")
	cmd.Env = os.Environ()
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		log.Printf("failed to start watcher process: %v", err)
		_ = f.Close()
		return
	}
	log.Printf("started watcher process pid=%d, logging to %s", cmd.Process.Pid, logfile)
}

func loadDotEnv() {
	path := ".env"
	if _, err := os.Stat(path); err != nil {
		return
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.IndexByte(line, '='); eq > 0 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, val)
			}
		}
	}
}
