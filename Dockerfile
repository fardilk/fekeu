# ============================
# 1. BUILDER (Debian Bookworm)
# ============================
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Install exact OCR deps (Bookworm = Tesseract 5.x)
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr \
    tesseract-ocr-eng \
    libtesseract-dev \
    libleptonica-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy full source
COPY . .

# Build unified API binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /build/fotonota_api \
    ./cmd/api


# ============================
# 2. RUNTIME (Bookworm)
# ============================
FROM debian:bookworm-slim

WORKDIR /app

# Install ONLY required runtime OCR libs (matching builder version)
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr \
    tesseract-ocr-eng \
    libtesseract-dev \
    libleptonica-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy built binary
COPY --from=builder /build/fotonota_api /app/fotonota_api

# Prepare directories
RUN mkdir -p /app/public/keu \
             /app/public/processed \
             /app/public/failed \
             /app/uploads \
             /app/logs

RUN chmod +x /app/fotonota_api

EXPOSE 8080

CMD ["/app/fotonota_api"]
