package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"be03/internal/config"
	httpRouter "be03/internal/http"
	"be03/internal/persistence"
	"be03/internal/scheduler"
	"be03/internal/watcher"
)

func main() {
	log.Println("🚀 Starting FotoNota Unified Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Support migrate command
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		db, err := persistence.NewDB()
		if err != nil {
			log.Fatal("migration failed:", err)
		}
		log.Println("✅ Migration and seeding completed")
		_ = db
		return
	}

	// Initialize database
	db, err := persistence.NewDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("✅ Database initialized")

	// Build HTTP router
	router := httpRouter.NewRouter(db)

	// Start background watcher (with auto-recovery)
	watcherInstance, err := watcher.New(db, cfg)
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	go watcherInstance.StartWithRecovery()
	log.Println("✅ Watcher started")

	// Start scheduler (with auto-recovery)
	schedulerInstance := scheduler.NewScheduler(cfg, db)
	go schedulerInstance.StartWithRecovery()
	log.Println("✅ Scheduler started")

	// Start HTTP server
	addr := ":" + cfg.Port
	log.Printf("✅ Starting HTTP server on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down gracefully...")
		server.Close()
	}()

	// Start server (blocking)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Server error:", err)
	}

	log.Println("👋 Server stopped")
}
