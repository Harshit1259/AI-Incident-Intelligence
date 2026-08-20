# Installation Guide

## Prerequisites

- Go 1.22+
- PostgreSQL 14+
- Node.js 20+ (for frontend dev)

## Quick Start

### 1. Database

```bash
sudo -u postgres createuser -P aiops_user    # password: aiops_pass
sudo -u postgres createdb -O aiops_user aiops
```

### 2. Backend

```bash
cd backend
export POSTGRES_DSN="host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable"
go run cmd/server/main.go
```

The backend auto-runs all migrations on startup. No manual schema setup needed.

### 3. Frontend

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173

### 4. Agent

Install the NeuroOps agent on each host you want to monitor:

```bash
cd neuroops-agent
go build -o neuroops-agent ./cmd/agent/main.go
sudo cp neuroops-agent /usr/local/bin/
```

Configure `config/agent.json`:
```json
{
  "agent": {
    "http.forwarder.enabled": true,
    "http.forwarder.url": "http://<backend-host>:8080",
    "log.agent.enabled": true,
    "metric.agent.enabled": true
  }
}
```

Start: `./neuroops-agent -config config/agent.json`

### Docker Deployment

```bash
docker-compose up -d
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_DSN` | Yes | localhost | PostgreSQL connection string |
| `HTTP_PORT` | No | 8080 | Backend port |
| `FRONTEND_ORIGIN` | No | http://localhost:5173 | CORS origin |
| `JWT_SECRET` | No | auto-generated | 32+ char secret for JWT signing |
| `ANTHROPIC_API_KEY` | No | - | Enables AI-powered RCA (optional) |
| `SLACK_BOT_TOKEN` | No | - | Slack integration |
| `SLACK_SIGNING_SECRET` | No | - | Slack webhook verification |
