# Multi-stage Dockerfile for building the be03 Go app and publishing to GHCR
# Usage in GH Actions: build the image and push to ghcr.io/${{ github.repository_owner }}/fekeu:tag

ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-bullseye AS builder
WORKDIR /src

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr \
    libleptonica-dev \
    libtesseract-dev \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# cache go modules
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org && go mod download

# copy rest of source
COPY . .

# build main application (enable CGO for gosseract)
RUN if [ -f ./main.go ]; then \
            CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /out/be03_app ./ ; \
        elif [ -d ./cmd ]; then \
            CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /out/be03_app ./cmd ; \
        else \
            echo "main entrypoint not found (main.go or ./cmd)" && exit 1; \
        fi

# watcher build (separate binary)
FROM builder AS watcher-builder
RUN if [ -d ./process ]; then \
            CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /out/be03_watcher ./process ; \
        elif [ -d ./cmd/watcher ]; then \
            CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /out/be03_watcher ./cmd/watcher ; \
        else \
            echo "watcher entrypoint not found (./process or ./cmd/watcher)" && exit 1; \
        fi

# runtime for main app
FROM debian:bullseye-slim AS runtime
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr libtesseract-dev ca-certificates && \
        rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/be03_app /usr/local/bin/be03_app
ENV SERVER_PORT=8081
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/be03_app"]

# runtime for watcher (final target name: "watcher")
FROM debian:bullseye-slim AS watcher
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr libtesseract-dev ca-certificates && \
        rm -rf /var/lib/apt/lists/*

COPY --from=watcher-builder /out/be03_watcher /usr/local/bin/be03_watcher
ENV WATCHER_PORT=9090
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/be03_watcher"]
