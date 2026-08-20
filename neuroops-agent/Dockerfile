# ─────────────────────────────────────────────────────────────────────────────
# NeurOps Agent — Dockerfile
# Copyright (c) NeurOps 2025. All rights reserved.
#
# Multi-stage build:
#   Stage 1 (builder) — compiles Go binaries
#   Stage 2 (runner)  — minimal runtime image with embedded Python
# ─────────────────────────────────────────────────────────────────────────────

# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS builder

ARG VERSION=1.0.0
ARG GIT_COMMIT=unknown

WORKDIR /build

# Cache Go module downloads separately from source changes
COPY go.mod go.sum ./
RUN go mod download

# Build agent and upgrader
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT}" \
    -o neuroops-agent ./cmd/agent

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT}" \
    -o neuroops-upgrader ./cmd/upgrader

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM ubuntu:22.04 AS runner

LABEL maintainer="NeurOps Engineering <engg@neuroops.ai>"
LABEL version="${VERSION}"
LABEL description="NeurOps Intelligent Observability Agent"

# Install runtime dependencies (ZeroMQ, SSH, SNMP system libs)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libzmq5 \
    libsnmp-base \
    libssl3 \
    ca-certificates \
    tzdata \
    procps \
    net-tools \
    curl \
  && rm -rf /var/lib/apt/lists/*

# Create neuroops directory layout
RUN mkdir -p /neuroops/neuroops-agent/{logs,config,plugins/{metric,runbook,topology},tmp}

# Copy compiled binaries
COPY --from=builder /build/neuroops-agent    /neuroops/neuroops-agent/neuroops-agent
COPY --from=builder /build/neuroops-upgrader /neuroops/neuroops-agent/neuroops-upgrader

# Copy default configuration
COPY config/ /neuroops/neuroops-agent/config/
COPY plugins/ /neuroops/neuroops-agent/plugins/
COPY sdk/     /neuroops/neuroops-agent/sdk/

# Make binaries executable
RUN chmod +x /neuroops/neuroops-agent/neuroops-agent \
             /neuroops/neuroops-agent/neuroops-upgrader

WORKDIR /neuroops/neuroops-agent

# Health check via the embedded HTTP endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -sf http://localhost:8765/health || exit 1

# Expose health port (optional — product communication is outbound via ZMQ)
EXPOSE 8765

# Default command: run the agent
ENTRYPOINT ["/neuroops/neuroops-agent/neuroops-agent"]
CMD ["-config", "config/agent.json"]
