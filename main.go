package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	setupRoutes(r)

	r.Run(":8081")
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
