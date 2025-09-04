package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ...existing code...

var jwtSecret []byte // loaded from env JWT_SECRET (fallback to dev default)

func main() {
	// Auto-load ./.env if present (no external dependency) before reading vars
	loadDotEnv()
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-insecure-secret-change" // development fallback
	}
	jwtSecret = []byte(secret)

	// Support a lightweight migrate command: `./be03_app migrate`
	// It runs AutoMigrate and seeding then exits. Useful for CI or manual DB setup.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		initDB()
		fmt.Println("migration and seeding completed")
		return
	}

	initDB()

	r := gin.Default()

	// Register CORS middleware early so all routes covered
	r.Use(corsMiddleware())

	setupRoutes(r)

	// Start file watcher in background so `go run .` also runs the watcher.
	go startWatcherProcess()

	r.Run(":8081")
}

// startWatcherProcess launches the existing process watcher as a child process
// using `go run`. Output is redirected to logs/watcher.log. This keeps the
// implementation minimal and avoids refactoring the watcher into a library.
func startWatcherProcess() {
	// Ensure logs directory exists
	_ = os.MkdirAll("logs", 0755)
	logfile := filepath.Join("logs", "watcher.log")
	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("failed to open watcher log: %v", err)
		return
	}
	cmd := exec.Command("go", "run", "process/process_keu.go", "-dir", "public/keu", "-watch")
	// inherit environment so DB_DSN and other env vars propagate
	cmd.Env = os.Environ()
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		log.Printf("failed to start watcher process: %v", err)
		_ = f.Close()
		return
	}
	log.Printf("started watcher process pid=%d, logging to %s", cmd.Process.Pid, logfile)
	// do not wait here; child runs independently and logs to file
}

// corsMiddleware allows cross-origin requests from configured origins (comma separated in ALLOWED_ORIGINS).
// If ALLOWED_ORIGINS is empty, it falls back to common local dev ports.
// Example .env: ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001
func corsMiddleware() gin.HandlerFunc {
	// Read and parse allowed origins once (hot-reload not required for dev convenience)
	raw := os.Getenv("ALLOWED_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		// include Vite default 5173 plus common React ports
		raw = "http://localhost:5173,http://localhost:3000,http://localhost:3001,http://localhost:3002,http://localhost:3003"
	}
	parts := strings.Split(raw, ",")
	allowed := make(map[string]struct{}, len(parts))
	cleanedList := make([]string, 0, len(parts))
	for _, p := range parts {
		o := strings.TrimSpace(p)
		if o == "" {
			continue
		}
		allowed[o] = struct{}{}
		cleanedList = append(cleanedList, o)
	}
	allowMethods := "GET,POST,PUT,PATCH,DELETE,OPTIONS"
	allowHeaders := "Authorization,Content-Type,Accept,Origin,X-Requested-With"
	maxAge := fmt.Sprintf("%d", int((12*time.Hour)/time.Second))
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", allowMethods)
				c.Header("Access-Control-Allow-Headers", allowHeaders)
				c.Header("Access-Control-Max-Age", maxAge)
			}
		}
		// Handle preflight quickly
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// loadDotEnv loads key=value pairs from a local .env file into the environment
// without overwriting variables that are already set. Lines starting with # are ignored.
func loadDotEnv() {
	path := ".env"
	if _, err := os.Stat(path); err != nil {
		return // no .env file
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
		// split on first '='
		if eq := strings.IndexByte(line, '='); eq > 0 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, val)
			}
		}
	}
}
