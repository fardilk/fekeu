# Single-stage build for unified binary
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Install Tesseract OCR
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    tesseract-ocr-eng \
    libtesseract-dev \
    libleptonica-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build single unified binary
RUN CGO_ENABLED=1 GOOS=linux go build -o fotonota_api ./cmd/api

# Runtime stage
FROM ubuntu:22.04

WORKDIR /app

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    tesseract-ocr-eng \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from builder
COPY --from=builder /build/fotonota_api .

# Create necessary directories
RUN mkdir -p public/keu public/processed public/failed uploads logs

# Set permissions
RUN chmod +x fotonota_api

# Expose port
EXPOSE 8080

# Run unified binary
CMD ["./fotonota_api"]
