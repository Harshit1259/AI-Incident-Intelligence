# ──────────────────────────────────────────────────────────────────────────────
# AI Incident Platform — Root Makefile
#
# Targets:
#   make build            Build backend binary + frontend assets
#   make build-backend    Build Go backend binary only
#   make build-frontend   Build React frontend (npm run build)
#   make test             Run all backend tests
#   make lint             Run go vet on backend
#   make clean            Remove ALL generated artefacts (safe to run anytime)
#   make package          Build + assemble a distributable release tarball
#   make validate-release Verify a release tarball contains no forbidden files
#   make docker           Build the production Docker image
#   make dev              Start backend + frontend in dev mode (requires tmux)
#   make check-env        Validate that a .env file is present (local dev gate)
# ──────────────────────────────────────────────────────────────────────────────

SHELL := /bin/bash
.DEFAULT_GOAL := build

# ── Version ───────────────────────────────────────────────────────────────────
VERSION     ?= $(shell cat VERSION 2>/dev/null || echo "0.0.0-dev")
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# ── Paths ─────────────────────────────────────────────────────────────────────
BACKEND_DIR   := backend
FRONTEND_DIR  := frontend
AGENT_DIR     := neuroops-agent
DIST_DIR      := dist
RELEASE_NAME  := aiops-platform-$(VERSION)
BINARY_NAME   := aiops-server

# ── Go ────────────────────────────────────────────────────────────────────────
GO           ?= go
GO_REQUIRED  := 1.25.0
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.gitCommit=$(GIT_COMMIT) \
  -X main.buildTime=$(BUILD_TIME)

# ══════════════════════════════════════════════════════════════════════════════
# BUILD
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: build
build: build-backend build-frontend

.PHONY: check-go-version
check-go-version:
	@ACTUAL=$$($(GO) version 2>/dev/null | awk '{print $$3}' | sed 's/go//'); \
	if [ -z "$$ACTUAL" ]; then \
	  echo "✗  Go not found. Install from https://go.dev/dl/ or use the Dockerfile"; \
	  exit 1; \
	fi
	@echo "✓ Go $$($(GO) version | awk '{print $$3}')"

.PHONY: build-backend
build-backend:
	@echo "→ Building backend ($(VERSION) @ $(GIT_COMMIT)) ..."
	@mkdir -p $(DIST_DIR)/bin
	cd $(BACKEND_DIR) && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
	  -o ../$(DIST_DIR)/bin/$(BINARY_NAME) ./cmd/server
	@echo "✓ Backend binary: $(DIST_DIR)/bin/$(BINARY_NAME)"

.PHONY: build-frontend
build-frontend:
	@echo "→ Building frontend ..."
	cd $(FRONTEND_DIR) && npm ci --silent && npm run build
	@echo "✓ Frontend assets: $(FRONTEND_DIR)/dist/"

# ══════════════════════════════════════════════════════════════════════════════
# TEST & QUALITY
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: test
test:
	@echo "→ Running backend tests ..."
	cd $(BACKEND_DIR) && $(GO) test -race -cover ./...

.PHONY: test-short
test-short:
	cd $(BACKEND_DIR) && $(GO) test -short ./...

.PHONY: lint
lint:
	@echo "→ Running go vet ..."
	cd $(BACKEND_DIR) && $(GO) vet ./...
	@echo "✓ Lint clean"

.PHONY: check-env
check-env:
	@test -f .env || (echo "✗  .env not found — copy .env.example and fill in values"; exit 1)
	@echo "✓ .env present"

# ══════════════════════════════════════════════════════════════════════════════
# CLEAN
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: clean
clean:
	@echo "→ Cleaning build artefacts ..."
	rm -rf $(DIST_DIR)
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/.cache
	cd $(BACKEND_DIR) && $(GO) clean -cache
	@echo "✓ Clean"

.PHONY: clean-logs
clean-logs:
	find . -name "*.log" -not -path "./.git/*" -delete
	@echo "✓ Logs removed"

# ══════════════════════════════════════════════════════════════════════════════
# PACKAGE — assemble a distributable release tarball
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: package
package: clean build
	@echo "→ Assembling release package $(RELEASE_NAME) ..."
	@mkdir -p $(DIST_DIR)/$(RELEASE_NAME)

	# Backend binary
	cp $(DIST_DIR)/bin/$(BINARY_NAME) $(DIST_DIR)/$(RELEASE_NAME)/

	# Frontend static assets
	cp -r $(FRONTEND_DIR)/dist $(DIST_DIR)/$(RELEASE_NAME)/frontend

	# Database migrations (source only — no secrets)
	cp -r $(BACKEND_DIR)/db/migrations $(DIST_DIR)/$(RELEASE_NAME)/migrations

	# Deployment artefacts
	cp docker-compose.yml   $(DIST_DIR)/$(RELEASE_NAME)/
	cp Dockerfile           $(DIST_DIR)/$(RELEASE_NAME)/
	cp .env.example         $(DIST_DIR)/$(RELEASE_NAME)/
	cp DEPLOYMENT.md        $(DIST_DIR)/$(RELEASE_NAME)/

	# Write release manifest
	@echo "Writing RELEASE_MANIFEST.json ..."
	@cat > $(DIST_DIR)/$(RELEASE_NAME)/RELEASE_MANIFEST.json <<EOF
	{
	  "product": "ai-incident-platform",
	  "version": "$(VERSION)",
	  "git_commit": "$(GIT_COMMIT)",
	  "build_time": "$(BUILD_TIME)",
	  "components": {
	    "backend": "$(BINARY_NAME)",
	    "frontend": "frontend/",
	    "migrations": "migrations/"
	  }
	}
	EOF

	# Create tarball
	cd $(DIST_DIR) && tar -czf $(RELEASE_NAME).tar.gz $(RELEASE_NAME)/

	# Validate before declaring success
	@$(MAKE) validate-release RELEASE_TARBALL=$(DIST_DIR)/$(RELEASE_NAME).tar.gz

	@echo ""
	@echo "✓ Release package: $(DIST_DIR)/$(RELEASE_NAME).tar.gz"

# ══════════════════════════════════════════════════════════════════════════════
# VALIDATE-RELEASE — CI gate: ensure no forbidden files in a release tarball
# ══════════════════════════════════════════════════════════════════════════════

RELEASE_TARBALL ?= $(DIST_DIR)/$(RELEASE_NAME).tar.gz

.PHONY: validate-release
validate-release:
	@echo "→ Validating release tarball: $(RELEASE_TARBALL) ..."
	@test -f $(RELEASE_TARBALL) || (echo "✗  Tarball not found: $(RELEASE_TARBALL)"; exit 1)
	@bash scripts/validate-package.sh $(RELEASE_TARBALL)
	@echo "✓ Release validation passed"

# ══════════════════════════════════════════════════════════════════════════════
# DOCKER
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: docker
docker:
	@echo "→ Building Docker image aiops-platform:$(VERSION) ..."
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  -t aiops-platform:$(VERSION) \
	  -t aiops-platform:latest \
	  .
	@echo "✓ Docker image: aiops-platform:$(VERSION)"

# ══════════════════════════════════════════════════════════════════════════════
# AGENT (delegates to neuroops-agent/Makefile)
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: build-agent
build-agent:
	$(MAKE) -C $(AGENT_DIR) build

.PHONY: package-agent
package-agent:
	$(MAKE) -C $(AGENT_DIR) package

.PHONY: clean-agent
clean-agent:
	$(MAKE) -C $(AGENT_DIR) clean

# ══════════════════════════════════════════════════════════════════════════════
# HELP
# ══════════════════════════════════════════════════════════════════════════════

.PHONY: help
help:
	@echo ""
	@echo "AI Incident Platform — Make Targets"
	@echo "────────────────────────────────────"
	@echo "  build            Build backend + frontend"
	@echo "  build-backend    Build Go binary only"
	@echo "  build-frontend   Build React assets only"
	@echo "  test             Run all tests"
	@echo "  lint             Run go vet"
	@echo "  clean            Remove all build artefacts"
	@echo "  package          Build + assemble release tarball"
	@echo "  validate-release Validate tarball for forbidden files"
	@echo "  docker           Build production Docker image"
	@echo "  check-env        Verify .env exists (local dev gate)"
	@echo "  check-go-version Assert local Go matches go.mod requirement ($(GO_REQUIRED))"
	@echo "  help             Show this help"
	@echo ""
