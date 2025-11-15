##############################
# BUILD API
##############################

FROM golang:1.24 AS builder
WORKDIR /app

# Install deps for OCR (gosseract)
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
    -o /app/fotonota_api \
    ./cmd/api

##############################
# RUNTIME
##############################

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr libtesseract-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/fotonota_api /app/fotonota_api

EXPOSE 8080

CMD ["/app/fotonota_api"]
