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
        else \
            echo "main.go not found" && exit 1; \
        fi

FROM debian:bullseye-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr libtesseract-dev ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/be03_app /usr/local/bin/be03_app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/be03_app"]
