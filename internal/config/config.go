package config

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// Server
	Port       string
	ServerPort string

	// Database
	DBDSN string

	// JWT
	JWTSecret string

	// Watcher
	WatchDir     string
	ProcessedDir string

	// OCR
	UploadBase string

	// Scheduler
	RetryInterval   string // e.g., "10m"
	CleanupInterval string // e.g., "1h"
}

// Load reads configuration from environment variables and .env file
func Load() (*Config, error) {
	// Load .env file if present
	loadDotEnv()

	// Normalize DATABASE_URL to DB_DSN
	normalizeDatabaseURL()

	cfg := &Config{
		Port:            getEnv("PORT", ""),
		ServerPort:      getEnv("SERVER_PORT", "8080"),
		DBDSN:           getEnv("DB_DSN", ""),
		JWTSecret:       getEnv("JWT_SECRET", "your-secret-key"), // TODO: Make required in production
		WatchDir:        getEnv("WATCH_DIR", "public/keu"),
		ProcessedDir:    getEnv("PROCESSED_DIR", "public/processed"),
		UploadBase:      getEnv("UPLOAD_BASE", "uploads"),
		RetryInterval:   getEnv("RETRY_INTERVAL", "10m"),
		CleanupInterval: getEnv("CLEANUP_INTERVAL", "1h"),
	}

	// Use PORT if set, otherwise fallback to ServerPort
	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = cfg.ServerPort
	}

	// Validate required fields
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN or DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
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
			log.Printf("Normalized DB_DSN to libpq format")
		}
	}
}
