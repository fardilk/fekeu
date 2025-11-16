# Single Binary Migration Progress

## Overview
Migration from multi-binary architecture to unified single binary (fotonota_api).

## Completed Phases

### ✅ Phase 1: Folder Structure
Created clean internal module structure:
```
internal/
├── config/       # Centralized configuration
├── watcher/      # OCR file watcher (replaces process/process_keu.go)
├── scheduler/    # Background jobs (retry, cleanup)
├── services/     # Business logic
└── repository/   # Data access
```

### ✅ Phase 2: Unified Application (PARTIAL)
**Status:** Main application unified, watcher placeholder created

**Completed:**
- ✅ `internal/config/config.go` (155 lines) - Centralized config with env loading
- ✅ `cmd/api/main.go` - Unified entrypoint with 3 goroutines:
  - HTTP server (Chi router)
  - Watcher (with StartWithRecovery)
  - Scheduler (with StartWithRecovery)
- ✅ Graceful shutdown handling
- ✅ `internal/scheduler/scheduler.go` (139 lines) - Background jobs:
  - retryFailedOCR() - resets failed uploads every 10 min
  - cleanup() - removes old failed uploads (7 days), expired tokens
- ✅ `internal/watcher/watcher.go` (minimal) - Placeholder implementation

**Binary:** `bin/fotonota_api` (18MB)

**Compilation:** ✅ SUCCESS
```bash
go build -o bin/fotonota_api ./cmd/api
```

**In Progress:**
- ⏳ Complete watcher implementation with fsnotify + OCR processing

### Phase 3: Database Models
**Status:** Models already exist, adjusted scheduler to match schema

**Model Changes:**
- Upload model uses `Failed` (boolean) instead of `Status` (string)
- Upload model uses `KeuanganID` instead of `CatatanID`
- Upload model uses `ContentType` instead of `MimeType`

## Pending Phases

### Phase 4: Complete Watcher Implementation
**Priority:** HIGH

**TODO:**
1. Add fsnotify-based file watching
2. Implement OCR processing pipeline:
   - Image preprocessing
   - OCR engine integration
   - Amount extraction
   - CatatanKeuangan creation
3. Add preload state for deduplication
4. Test file upload → OCR → database flow

### Phase 5: Simplify Dockerfile
**Priority:** MEDIUM

**Current:** Multi-stage build with separate API + watcher binaries
**Target:** Single binary build

```dockerfile
# Simplified Dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o fotonota_api ./cmd/api

FROM alpine:latest
RUN apk add --no-cache tesseract-ocr liblept tesseract-ocr-data-eng
COPY --from=builder /app/fotonota_api /app/
WORKDIR /app
CMD ["/app/fotonota_api"]
```

### Phase 6: Simplify docker-compose.yml
**Priority:** MEDIUM

**Changes:**
- Remove watcher service
- Keep only single `api` service
- Update ports, volumes, environment

### Phase 7: Update GitHub Actions
**Priority:** MEDIUM

**Changes:**
- Remove watcher build steps
- Simplify to single image build
- Update deploy workflow

### Phase 8: E2E Tests
**Priority:** HIGH (User Requested)

**Tests needed:**
1. Unified binary startup
2. HTTP + watcher + scheduler running concurrently
3. File upload → OCR → database flow
4. Graceful shutdown

### Phase 9: Integration Tests
**Priority:** HIGH (User Requested)

**Tests needed:**
1. Watcher processing files
2. Scheduler retry logic
3. HTTP endpoints with running services
4. Database transactions

## Architecture Summary

### Single Binary Structure
```
fotonota_api (18MB)
├── HTTP Server (port 8080/8081)
│   ├── Auth endpoints
│   ├── Upload endpoints
│   └── CatatanKeuangan endpoints
├── Watcher Goroutine
│   ├── fsnotify file watching
│   ├── OCR processing
│   └── Auto-recovery on panic
└── Scheduler Goroutine
    ├── Retry failed OCR (10 min)
    ├── Cleanup old records (1 hour)
    └── Auto-recovery on panic
```

### Configuration (internal/config)
- Loads from `.env` file
- Supports environment variables
- Database URL normalization
- OCR settings (WatchDir, ProcessedDir)

### Auto-Recovery Pattern
Both watcher and scheduler use StartWithRecovery():
```go
func (w *Watcher) StartWithRecovery() {
    for {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    log.Printf("[WATCHER] PANIC recovered: %v", r)
                }
            }()
            w.Start()
        }()
        log.Printf("[WATCHER] Restarting in 2 seconds...")
        time.Sleep(2 * time.Second)
    }
}
```

## API Endpoints (for Flutter)

### Production VPS
- **URL:** `http://103.172.204.34:8081`
- **Login:** `POST /login`
  ```json
  {
    "username": "fardil",
    "password": "admin123"
  }
  ```

### Local Development
- **URL:** `http://localhost:8080`

## Next Steps

1. **Complete Watcher Implementation** (HIGH)
   - Copy OCR logic from process/process_keu.go
   - Implement file watching with fsnotify
   - Test with actual image files

2. **Create E2E Tests** (HIGH)
   - Test unified binary startup
   - Test concurrent goroutines
   - Test file processing flow

3. **Create Integration Tests** (HIGH)
   - Test watcher module
   - Test scheduler module
   - Test HTTP endpoints

4. **Simplify Deployment** (MEDIUM)
   - Update Dockerfile
   - Update docker-compose.yml
   - Update GitHub Actions

## Files Modified

### Created
- `internal/config/config.go` (155 lines)
- `internal/watcher/watcher.go` (minimal, 44 lines)
- `internal/scheduler/scheduler.go` (139 lines)

### Modified
- `cmd/api/main.go` - Unified with 3 goroutines, removed old helper functions
- `internal/http/API_DOCUMENTATION.md` - Added VPS IP (103.172.204.34:8081)

### Deprecated (to be removed after full migration)
- `process/process_keu.go` (772 lines) - Will be replaced by internal/watcher
- Old watcher binary builds

## Build Commands

```bash
# Build unified binary
go build -o bin/fotonota_api ./cmd/api

# Run with migrate
./bin/fotonota_api migrate

# Run server
./bin/fotonota_api

# Test watcher module
go build ./internal/watcher

# Test scheduler module
go build ./internal/scheduler

# Test config module
go build ./internal/config
```

## Migration Benefits

1. **Single Binary** - Easier deployment and version management
2. **Shared Memory** - Watcher and HTTP server share database connections
3. **Simplified Logs** - All logs in one place
4. **Auto-Recovery** - Both services restart automatically on panic
5. **Graceful Shutdown** - Clean shutdown of all goroutines
6. **Centralized Config** - Single source of configuration

## Status: Phase 2 - In Progress

**Current State:** Unified binary compiles and runs with HTTP + scheduler. Watcher is placeholder.
**Next Task:** Implement complete watcher with OCR processing.
