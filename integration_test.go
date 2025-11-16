package main

import (
"context"
"os"
"path/filepath"
"testing"
"time"

"be03/internal/config"
"be03/internal/scheduler"
"be03/internal/watcher"
"be03/models"
"be03/persistence"
)

func TestIntegration_Config(t *testing.T) {
cfg := config.Load()

if cfg.DatabaseURL == "" {
not be empty")
}

if cfg.Port == "" {
not be empty")
}

if cfg.WatchDir == "" {
not be empty")
}
}

func TestIntegration_Database(t *testing.T) {
cfg := config.Load()
db, err := persistence.NewDB(cfg.DatabaseURL)
if err != nil {
connect to database: %v", err)
}

// Test migration
if err := db.AutoMigrate(&models.User{}, &models.Role{}, &models.Upload{}, &models.Catatan{}); err != nil {
%v", err)
}

// Test basic query
var count int64
db.Model(&models.User{}).Count(&count)
t.Logf("User count: %d", count)
}

func TestIntegration_Watcher(t *testing.T) {
cfg := config.Load()
db, err := persistence.NewDB(cfg.DatabaseURL)
if err != nil {
connect to database: %v", err)
}

// Create temporary watch directory
tmpDir := filepath.Join(os.TempDir(), "test_watch")
os.MkdirAll(tmpDir, 0755)
defer os.RemoveAll(tmpDir)

cfg.WatchDir = tmpDir

w, err := watcher.New(db, cfg)
if err != nil {
create watcher: %v", err)
}

ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

go func() {
e()
w.Stop()

t.Log("Watcher started and stopped successfully")
}

func TestIntegration_Scheduler(t *testing.T) {
cfg := config.Load()
db, err := persistence.NewDB(cfg.DatabaseURL)
if err != nil {
connect to database: %v", err)
}

s := scheduler.New(db, cfg)

ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

go func() {
e()
s.Stop()

t.Log("Scheduler started and stopped successfully")
}

func TestIntegration_FullStack(t *testing.T) {
cfg := config.Load()
db, err := persistence.NewDB(cfg.DatabaseURL)
if err != nil {
connect to database: %v", err)
}

// Create temporary watch directory
tmpDir := filepath.Join(os.TempDir(), "test_watch_full")
os.MkdirAll(tmpDir, 0755)
defer os.RemoveAll(tmpDir)

cfg.WatchDir = tmpDir

// Initialize components
w, err := watcher.New(db, cfg)
if err != nil {
create watcher: %v", err)
}

s := scheduler.New(db, cfg)

ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

// Start all components
go w.Start()
go s.Start()

<-ctx.Done()

// Stop all components
w.Stop()
s.Stop()

t.Log("Full stack integration test passed")
}
