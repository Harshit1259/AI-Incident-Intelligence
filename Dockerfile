# ── Build stage ───────────────────────────────────────────────────────────────
# Keep this image tag in sync with the `go` / `toolchain` lines in backend/go.mod.
# To upgrade Go: update all three places atomically.
FROM golang:1.25-alpine AS builder

# GOTOOLCHAIN=local tells Go not to attempt downloading a different toolchain;
# the exact version is already provided by the base image.
ENV GOTOOLCHAIN=local

RUN apk add --no-cache git

WORKDIR /app
COPY backend/ ./

# Accept optional build-metadata injected by `make docker` or CI.
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
      -ldflags "-s -w \
        -X main.version=${VERSION} \
        -X main.gitCommit=${GIT_COMMIT} \
        -X main.buildTime=${BUILD_TIME}" \
      -o /aiops-backend ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────────
# Pinned to a specific Alpine patch; bump periodically for security updates.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /aiops-backend /usr/local/bin/aiops-backend

EXPOSE 8080
ENV HTTP_PORT=8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

CMD ["aiops-backend"]
