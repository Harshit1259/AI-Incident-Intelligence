#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# NeuroOps AI Incident Platform — One-Command Quick Start
# Usage:  ./quickstart.sh
# ──────────────────────────────────────────────────────────────────────────────
set -euo pipefail

RED='\033[0;31m'; GRN='\033[0;32m'; YLW='\033[0;33m'; BLU='\033[0;34m'
BOLD='\033[1m'; NC='\033[0m'

info()  { echo -e "${BLU}${BOLD}  →${NC}  $*"; }
ok()    { echo -e "${GRN}${BOLD}  ✓${NC}  $*"; }
warn()  { echo -e "${YLW}${BOLD}  !${NC}  $*"; }
fatal() { echo -e "${RED}${BOLD}  ✗${NC}  $*"; exit 1; }

BANNER="
 ███╗   ██╗███████╗██╗   ██╗██████╗  ██████╗  ██████╗ ██████╗ ███████╗
 ████╗  ██║██╔════╝██║   ██║██╔══██╗██╔═══██╗██╔═══██╗██╔══██╗██╔════╝
 ██╔██╗ ██║█████╗  ██║   ██║██████╔╝██║   ██║██║   ██║██████╔╝███████╗
 ██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██║   ██║██║   ██║██╔═══╝ ╚════██║
 ██║ ╚████║███████╗╚██████╔╝██║  ██║╚██████╔╝╚██████╔╝██║     ███████║
 ╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝     ╚══════╝
              AI Incident Platform — Quick Start
"
echo -e "${BLU}${BOLD}${BANNER}${NC}"

# ── Dependency checks ─────────────────────────────────────────────────────────
info "Checking dependencies..."

command -v docker    >/dev/null 2>&1 || fatal "Docker not found. Install from https://docs.docker.com/get-docker/"
command -v node      >/dev/null 2>&1 || fatal "Node.js not found. Install from https://nodejs.org/ (v18+)"
command -v npm       >/dev/null 2>&1 || fatal "npm not found. Install Node.js from https://nodejs.org/"

# docker compose v2 or v1
if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  fatal "Docker Compose not found. Install Docker Desktop or 'docker compose' plugin."
fi

ok "docker, node, npm — all present"

# ── Generate .env ─────────────────────────────────────────────────────────────
if [ ! -f ".env" ]; then
  info "Generating .env with secure random secrets..."
  cp .env.example .env

  # Generate secure random values
  JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
  PG_PASSWORD=$(openssl rand -hex 16  2>/dev/null || head -c 16 /dev/urandom | xxd -p)
  AGENT_KEY=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)

  # Replace placeholder values in .env
  sed -i.bak \
    -e "s|JWT_SECRET=CHANGE_ME_USE_openssl_rand_-base64_32|JWT_SECRET=${JWT_SECRET}|g" \
    -e "s|password=CHANGE_ME|password=${PG_PASSWORD}|g" \
    -e "s|AGENT_SECRET_KEY=CHANGE_ME_USE_openssl_rand_-base64_32|AGENT_SECRET_KEY=${AGENT_KEY}|g" \
    .env
  rm -f .env.bak

  # Export for docker-compose
  export POSTGRES_PASSWORD="${PG_PASSWORD}"
  ok ".env created with generated secrets"
else
  ok ".env already exists — skipping generation"
  # Source it so POSTGRES_PASSWORD is available for docker-compose
  set -a; source .env; set +a
fi

# Make sure POSTGRES_PASSWORD is exported (docker-compose needs it)
PG_PASS=$(grep 'password=' .env | grep -oP 'password=\K[^ ]+' | head -1 || echo "aiops_dev")
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-${PG_PASS}}"

# ── Build frontend ────────────────────────────────────────────────────────────
info "Building frontend..."
cd frontend
if [ ! -d "node_modules" ]; then
  npm install --silent
fi
npm run build --silent
cd ..
ok "Frontend built → frontend/dist/"

# ── Start services ────────────────────────────────────────────────────────────
info "Starting NeuroOps Platform (DB + Backend + Frontend)..."
$COMPOSE up -d --build

# ── Wait for backend health ───────────────────────────────────────────────────
info "Waiting for backend to be healthy..."
MAX_WAIT=60
WAITED=0
until curl -sf http://localhost:8080/api/v1/health >/dev/null 2>&1; do
  sleep 2
  WAITED=$((WAITED + 2))
  if [ $WAITED -ge $MAX_WAIT ]; then
    warn "Backend didn't respond in ${MAX_WAIT}s. Check logs: $COMPOSE logs backend"
    break
  fi
done

if curl -sf http://localhost:8080/api/v1/health >/dev/null 2>&1; then
  ok "Backend healthy"
fi

# ── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GRN}${BOLD}══════════════════════════════════════════════════════════${NC}"
echo -e "${GRN}${BOLD}  NeuroOps is running!${NC}"
echo -e "${GRN}${BOLD}══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  Platform URL  : ${BOLD}http://localhost${NC}  (or http://localhost:8080)"
echo -e "  API Base      : ${BOLD}http://localhost:8080/api/v1${NC}"
echo ""
echo -e "  ${BOLD}What to do next:${NC}"
echo -e "    1. Open ${BOLD}http://localhost${NC} in your browser"
echo -e "    2. Click ${BOLD}Create Account${NC} to register (first user = admin)"
echo -e "    3. Follow the ${BOLD}Setup Guide${NC} — paste a webhook URL and see your first incident"
echo ""
echo -e "  ${BOLD}Useful commands:${NC}"
echo -e "    $COMPOSE logs -f backend     # tail backend logs"
echo -e "    $COMPOSE logs -f             # tail all logs"
echo -e "    $COMPOSE down               # stop everything"
echo -e "    $COMPOSE down -v            # stop + wipe database"
echo ""
echo -e "  ${BOLD}Test an alert (no tool needed) — Prometheus Alertmanager format:${NC}"
echo -e "    curl -s -X POST http://localhost:8080/api/v1/ingest/prometheus \\"
echo -e "      -H 'Content-Type: application/json' -H 'X-Source-Token: <your source token>' \\"
echo -e "      -d '{\"alerts\":[{\"status\":\"firing\",\"labels\":{\"alertname\":\"HighErrorRate\",\"service\":\"api\",\"severity\":\"critical\"},\"annotations\":{\"summary\":\"High error rate\"}}]}'"
echo ""

# Open browser automatically
if command -v xdg-open >/dev/null 2>&1; then
  xdg-open "http://localhost" >/dev/null 2>&1 &
elif command -v open >/dev/null 2>&1; then
  open "http://localhost" >/dev/null 2>&1 &
fi
