# ─────────────────────────────────────────────────────────────────────────────
# NeurOps Agent — Makefile
# Copyright (c) NeurOps 2025. All rights reserved.
#
# Targets:
#   make build          Build agent and upgrader binaries for the host OS
#   make build-all      Cross-compile for Ubuntu 18/20/22/24
#   make test           Run all Go tests
#   make lint           Run golangci-lint
#   make package        Build + package into a distributable tarball
#   make clean          Remove all build artefacts
#   make install-sdk    Install the Python SDK into the embedded Python
#   make run            Run the agent locally (dev mode)
#   make docker         Build a Docker image for the agent
# ─────────────────────────────────────────────────────────────────────────────

SHELL       := /bin/bash
GO          ?= go
GOFLAGS     ?= -trimpath
GOOS        ?= linux
GOARCH      ?= amd64
CGO_ENABLED ?= 1

VERSION     := 1.0.0
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.gitCommit=$(GIT_COMMIT) \
  -X main.buildTime=$(BUILD_TIME)

AGENT_BIN    := neuroops-agent
UPGRADER_BIN := neuroops-upgrader
DIST_DIR     := dist
PACKAGE_NAME := neuroops-agent-$(VERSION)-linux-$(GOARCH)

PYTHON_EMBEDDED := /neuroops/python-embedded/bin/python3
PIP_EMBEDDED    := /neuroops/python-embedded/bin/pip3

# ── Default ───────────────────────────────────────────────────────────────────

.PHONY: all
all: build

# ── Build ─────────────────────────────────────────────────────────────────────

.PHONY: build
build: build-agent build-upgrader
	@echo "✓ Build complete: $(AGENT_BIN) $(UPGRADER_BIN)"

.PHONY: build-agent
build-agent:
	@echo "→ Building $(AGENT_BIN) ($(GOOS)/$(GOARCH)) ..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
	  -o $(AGENT_BIN) ./cmd/agent

.PHONY: build-upgrader
build-upgrader:
	@echo "→ Building $(UPGRADER_BIN) ($(GOOS)/$(GOARCH)) ..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
	  -o $(UPGRADER_BIN) ./cmd/upgrader

# ── Cross-compilation for each Ubuntu target ──────────────────────────────────

.PHONY: build-all
build-all: build-ubuntu-18 build-ubuntu-20 build-ubuntu-22 build-ubuntu-24

.PHONY: build-ubuntu-%
build-ubuntu-%:
	@echo "→ Building for Ubuntu $* ..."
	@mkdir -p ubuntu-$*
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
	  -o ubuntu-$*/$(AGENT_BIN) ./cmd/agent
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
	  $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
	  -o ubuntu-$*/$(UPGRADER_BIN) ./cmd/upgrader
	@echo "✓ Ubuntu $* binaries built"

# ── Test ──────────────────────────────────────────────────────────────────────

.PHONY: test
test:
	@echo "→ Running tests ..."
	$(GO) test -v -race -cover ./...

.PHONY: test-short
test-short:
	$(GO) test -short ./...

.PHONY: bench
bench:
	$(GO) test -bench=. -benchmem ./...

# ── Lint ──────────────────────────────────────────────────────────────────────

.PHONY: lint
lint:
	@command -v golangci-lint &>/dev/null || \
	  (echo "Installing golangci-lint ..."; \
	   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin)
	golangci-lint run ./...

.PHONY: vet
vet:
	$(GO) vet ./...

# ── SDK ───────────────────────────────────────────────────────────────────────

.PHONY: install-sdk
install-sdk:
	@echo "→ Installing NeurOps Python SDK ..."
	$(PIP_EMBEDDED) install -e sdk/python --quiet

.PHONY: build-go-sdk
build-go-sdk:
	@echo "→ Building Go SDK ..."
	cd sdk/go && $(GO) build ./...

# ── Plugin compilation ────────────────────────────────────────────────────────

.PHONY: build-plugins
build-plugins:
	@echo "→ Building example Go plugins ..."
	@for dir in plugins/examples/*/; do \
	  if [ -f "$$dir/plugin.go" ]; then \
	    echo "  Building $$dir ..."; \
	    cd "$$dir" && \
	    GOPATH=/neuroops/gopath GOROOT=/neuroops/go \
	    /neuroops/go/bin/go build $(GOFLAGS) -o plugin . && \
	    cd - > /dev/null; \
	  fi; \
	done
	@echo "✓ Example plugins built"

# ── Package ───────────────────────────────────────────────────────────────────

.PHONY: package
package: build build-all
	@echo "→ Creating distribution package ..."
	@mkdir -p $(DIST_DIR)/$(PACKAGE_NAME)

	# Main binaries
	cp $(AGENT_BIN)    $(DIST_DIR)/$(PACKAGE_NAME)/
	cp $(UPGRADER_BIN) $(DIST_DIR)/$(PACKAGE_NAME)/

	# Per-Ubuntu binaries
	@for v in 18 20 22 24; do \
	  mkdir -p $(DIST_DIR)/$(PACKAGE_NAME)/ubuntu-$$v; \
	  cp ubuntu-$$v/$(AGENT_BIN)    $(DIST_DIR)/$(PACKAGE_NAME)/ubuntu-$$v/ 2>/dev/null || true; \
	  cp ubuntu-$$v/$(UPGRADER_BIN) $(DIST_DIR)/$(PACKAGE_NAME)/ubuntu-$$v/ 2>/dev/null || true; \
	done

	# Config, services, scripts
	cp -r config   $(DIST_DIR)/$(PACKAGE_NAME)/
	cp -r services $(DIST_DIR)/$(PACKAGE_NAME)/
	cp -r scripts  $(DIST_DIR)/$(PACKAGE_NAME)/
	cp -r plugins  $(DIST_DIR)/$(PACKAGE_NAME)/
	cp -r sdk      $(DIST_DIR)/$(PACKAGE_NAME)/

	# Install script at root for convenience
	cp scripts/install.sh $(DIST_DIR)/$(PACKAGE_NAME)/install.sh
	chmod +x $(DIST_DIR)/$(PACKAGE_NAME)/install.sh

	# Create tarball
	cd $(DIST_DIR) && tar -czf $(PACKAGE_NAME).tar.gz $(PACKAGE_NAME)/

	@echo "✓ Package: $(DIST_DIR)/$(PACKAGE_NAME).tar.gz"

# ── Docker ────────────────────────────────────────────────────────────────────

.PHONY: docker
docker:
	@echo "→ Building Docker image ..."
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  -t neuroops/agent:$(VERSION) \
	  -t neuroops/agent:latest \
	  .
	@echo "✓ Docker image: neuroops/agent:$(VERSION)"

# ── Development helpers ───────────────────────────────────────────────────────

.PHONY: run
run: build
	@echo "→ Starting agent in development mode ..."
	./$(AGENT_BIN) -config config/agent.json

.PHONY: run-upgrader
run-upgrader: build-upgrader
	./$(UPGRADER_BIN) -config config/agent.json

.PHONY: health
health:
	@curl -s http://localhost:8765/health/detail | python3 -m json.tool

# ── Clean ─────────────────────────────────────────────────────────────────────

.PHONY: clean
clean:
	@echo "→ Cleaning ..."
	rm -f $(AGENT_BIN) $(UPGRADER_BIN)
	rm -rf $(DIST_DIR)
	rm -rf ubuntu-{18,20,22,24}
	rm -rf plugins/examples/*/plugin
	$(GO) clean -cache
	@echo "✓ Clean"

.PHONY: clean-logs
clean-logs:
	rm -rf logs/*.log

# ── Version ───────────────────────────────────────────────────────────────────

.PHONY: version
version:
	@echo "NeurOps Agent v$(VERSION) (git=$(GIT_COMMIT) built=$(BUILD_TIME))"
