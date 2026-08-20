# On-Premises Deployment Guide

## Build & Runtime Matrix

These are the **exact** versions the platform is built and tested against.
Any deviation is unsupported and will produce either a build-time error or undefined runtime behaviour.

| Layer | Pinned version | Where enforced |
|-------|----------------|----------------|
| Go toolchain | **1.25.0** | `go.mod` `toolchain` directive · `Dockerfile` `FROM golang:1.25-alpine` · `GOTOOLCHAIN=local` env · `.tool-versions` / `.mise.toml` · `make check-go-version` |
| Node.js | **20.18.0 (LTS)** | `.tool-versions` / `.mise.toml` |
| Base runtime image | **alpine:3.21** | `Dockerfile` runtime stage |
| PostgreSQL | **16** | `docker-compose.yml` `postgres:16-alpine` |

### Setting up your local toolchain

**Option A — mise (recommended)**
```bash
# Install mise: https://mise.jdx.dev
mise install          # reads .mise.toml, installs Go 1.25.0 + Node 20.18.0
```

**Option B — asdf**
```bash
asdf plugin add golang https://github.com/asdf-community/asdf-golang
asdf plugin add nodejs https://github.com/asdf-vm/asdf-nodejs
asdf install          # reads .tool-versions
```

**Option C — manual**

Download Go 1.25.0 from https://go.dev/dl/ and Node 20.18.0 from https://nodejs.org.

**Verify before building**
```bash
make check-go-version   # exits non-zero if local go != 1.25.0
go version              # go version go1.25.0 linux/amd64
node --version          # v20.18.0
```

---

## Prerequisites

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4 cores |
| RAM | 4 GB | 8 GB |
| Disk | 20 GB | 100 GB SSD |
| OS | Ubuntu 22.04 / RHEL 9 / Debian 12 | Ubuntu 22.04 LTS |
| Docker | 24+ | 24+ |
| Docker Compose | v2.20+ | v2.20+ |
| PostgreSQL | 15+ (if external) | 16 |

---

## Quick Start (Docker Compose)

### 1. Clone and configure

```bash
git clone <your-repo-url> ai-incident-platform
cd ai-incident-platform

# Copy template and fill in real values
cp .env.example .env
```

### 2. Set required secrets in `.env`

Open `.env` and set these — **never commit this file**:

```bash
# Generate a strong JWT secret
openssl rand -base64 32

# Set production mode
ENV=production

# Paste output above as JWT_SECRET
JWT_SECRET=<output from openssl>

# Database password
POSTGRES_PASSWORD=<strong-random-password>

# Your LLM key (OpenAI or Anthropic)
LLM_API_KEY=sk-proj-...
```

### 3. Start services

```bash
docker compose up -d
```

Services start order: PostgreSQL → Backend (waits for DB healthcheck).

### 4. Apply database migrations

```bash
# Migrations run automatically on backend startup via store.NewDB()
# Verify with:
docker compose logs backend | grep -i migrat
```

### 5. Verify

```bash
curl http://localhost:8080/api/v1/health
# Expected: {"status":"ok"}
```

---

## Production Hardening Checklist

### Security

- [ ] `ENV=production` set in `.env`
- [ ] `JWT_SECRET` is 32+ random characters (use `openssl rand -base64 32`)
- [ ] `AGENT_SECRET_KEY` is 32+ random characters (use `openssl rand -base64 32`)
- [ ] `AGENT_INGEST_TOKEN` is left empty (HMAC enforced in production; token only for dev)
- [ ] `POSTGRES_PASSWORD` is strong and unique
- [ ] `.env` file is NOT in git (verify: `git status .env` shows nothing)
- [ ] All real API keys rotated if previously committed to git
- [ ] `FRONTEND_ORIGIN` set to your actual domain (not `*`)
- [ ] TLS termination configured (nginx/Traefik in front of port 8080)
- [ ] Firewall: only expose ports 80/443 externally; 8080 internal only
- [ ] PostgreSQL port 5432 NOT exposed externally (docker-compose default binds to 127.0.0.1)

### Reverse Proxy (nginx example)

```nginx
server {
    listen 443 ssl;
    server_name incidents.yourdomain.com;

    ssl_certificate     /etc/ssl/certs/your.crt;
    ssl_certificate_key /etc/ssl/private/your.key;

    # Frontend (React SPA)
    location / {
        root /var/www/ai-incident-platform/dist;
        try_files $uri $uri/ /index.html;
    }

    # Backend API
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 90s;
    }
}
```

### Persistent Data

PostgreSQL data is stored in the `pgdata` Docker volume. Back it up regularly:

```bash
# Backup
docker exec $(docker compose ps -q db) pg_dump -U aiops_user aiops | gzip > backup-$(date +%Y%m%d).sql.gz

# Restore
gunzip -c backup-20240101.sql.gz | docker exec -i $(docker compose ps -q db) psql -U aiops_user aiops
```

---

## Frontend Build (for nginx serving)

```bash
cd frontend
npm install
VITE_API_BASE_URL=https://incidents.yourdomain.com/api/v1 npm run build
# Output in frontend/dist/ — copy to your web server
```

---

## NeuroOps Agent (optional — for remote command execution)

The agent runs on each monitored host and sends metrics/logs to the platform.

```bash
# On each monitored host:
cd neuroops-agent
./neuroops-agent --server https://incidents.yourdomain.com --agent-id $(hostname)
```

The agent listens on port 8765 by default. The backend discovers agents by their registered `host_ip`.

---

## Environment Variables Reference

| Variable | Required | Description |
|----------|----------|-------------|
| `ENV` | No | Set to `production` to enforce JWT_SECRET |
| `HTTP_PORT` | No | Backend port (default: 8080) |
| `POSTGRES_DSN` | Yes | PostgreSQL connection string |
| `POSTGRES_PASSWORD` | Yes (compose) | DB password for docker-compose |
| `JWT_SECRET` | **Yes in prod** | 32+ char secret for JWT signing |
| `LLM_API_KEY` | No | OpenAI/Anthropic key for AI features |
| `LLM_MODEL` | No | Model name (default: gpt-4o) |
| `FRONTEND_ORIGIN` | Yes | Allowed CORS origin |
| `SLACK_BOT_TOKEN` | No | Slack integration |
| `SLACK_SIGNING_SECRET` | No | Slack webhook verification |
| `GITHUB_WEBHOOK_SECRET` | No | GitHub webhook HMAC secret |
| `GITLAB_WEBHOOK_TOKEN` | No | GitLab webhook token |
| `DATADOG_API_KEY` | No | Datadog integration |
| `SRE_HOURLY_COST` | No | Used for ROI calculations (default: 150) |
| `AGENT_INGEST_ENABLED` | No | Enable agent metric ingestion (default: true) |
| `AGENT_INGEST_TOKEN` | No | Legacy dev-mode token; ignored in production (no HMAC) |
| `AGENT_SECRET_KEY` | **Yes in prod** | 32+ byte random key used to AES-encrypt per-agent secrets at rest — generate: `openssl rand -base64 32` |

---

## Testing the ingest endpoint

Agent ingest now requires per-agent HMAC-SHA256 request signing.
Use the helper script below (requires `openssl` and `xxd`).

```bash
AGENT_ID="<your-agent-id>"
AGENT_SECRET="<agt_… secret issued at registration>"
BODY='{"events":[{"event.type":"metric","cpu":45.2}]}'
TIMESTAMP=$(date -u +%s)
NONCE=$(openssl rand -hex 16)
BODY_HASH=$(printf '%s' "$BODY" | openssl dgst -sha256 | awk '{print $2}')
SIGNING_STRING="POST\n${AGENT_ID}\n${TIMESTAMP}\n${NONCE}\n${BODY_HASH}"
SIGNATURE=$(printf "$SIGNING_STRING" | openssl dgst -sha256 -hmac "$AGENT_SECRET" | awk '{print $2}')

curl -X POST http://localhost:8080/api/v1/ingest/agent \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: $AGENT_ID" \
  -H "X-Timestamp: $TIMESTAMP" \
  -H "X-Nonce: $NONCE" \
  -H "X-Signature: $SIGNATURE" \
  -d "$BODY"
```

Expected: `{"alerts":0,"processed":1}`

> **Dev mode shortcut** (local only, never in production):
> Set `AGENT_INGEST_TOKEN` in `.env` and pass `X-Agent-Token: <token>` instead of the HMAC headers.
> This path is disabled automatically when `ENV=production`.

---

## Monitoring

The platform exposes a metrics endpoint (no auth required — for Prometheus/Grafana):

```
GET /api/v1/platform/metrics
```

Health check:

```
GET /api/v1/health
```
