##############################
# BUILDER FOR API
##############################
FROM golang:1.24-bullseye AS builder
WORKDIR /app

# Install OCR deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr \
    libleptonica-dev \
    libtesseract-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build API binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /out/fotonota_api \
    ./cmd/api


##############################
# BUILDER FOR WATCHER
##############################
FROM golang:1.24-bullseye AS watcher-builder
WORKDIR /app

# Install OCR deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr \
    libleptonica-dev \
    libtesseract-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build WATCHER binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /out/fotonota_watcher \
    ./cmd/watcher


##############################
# RUNTIME (API)
##############################
FROM ubuntu:22.04 AS runtime
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr libtesseract-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/fotonota_api /app/fotonota_api

EXPOSE 8080
ENTRYPOINT ["/app/fotonota_api"]


##############################
# RUNTIME (WATCHER)
##############################
FROM ubuntu:22.04 AS watcher
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr libtesseract-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=watcher-builder /out/fotonota_watcher /app/fotonota_watcher

ENTRYPOINT ["/app/fotonota_watcher"]
