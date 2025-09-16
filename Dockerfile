# Multi-stage Dockerfile for building the be03 Go app and publishing to GHCR
# Usage in GH Actions: build the image and push to ghcr.io/${{ github.repository_owner }}/fekeu:tag

ARG GO_VERSION=1.20
FROM golang:${GO_VERSION}-bullseye AS builder
WORKDIR /src

# cache go modules
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download

# copy rest of source
COPY . .

# build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags='-s -w' -o /out/be03_app ./cmd/create_user || true

# If the repo's main app is at top-level, build that too as fallback
RUN if [ -f ./main.go ]; then \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o /out/be03_app ./ ; \
    fi

FROM gcr.io/distroless/static-debian11
COPY --from=builder /out/be03_app /usr/local/bin/be03_app
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/be03_app"]
