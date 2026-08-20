<div align="center">

# NeuroOps

**AI incident intelligence for SRE teams.**
Cut alert noise by an order of magnitude, get an evidence-based root cause in seconds, and close the loop with policy-gated auto-remediation.

[![CI](https://github.com/Harshit1259/AI-Incident-Intelligence/actions/workflows/ci.yml/badge.svg)](https://github.com/Harshit1259/AI-Incident-Intelligence/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

---

## The problem

A mid-size platform team receives thousands of alerts a day from Prometheus, Datadog, PagerDuty and CI. Most are duplicates, and most of the rest are downstream symptoms of one underlying failure. On-call engineers spend their night triaging noise, and the signal that mattered arrives buried in it.

NeuroOps ingests every alert through one normalized pipeline, deduplicates and correlates them into a small number of real incidents, explains what broke and why, and — where policy allows — fixes it and verifies the fix worked.

```
   1,000 raw alerts  →  fingerprint dedup  →  time-window correlation  →  12 incidents
```

---

## Quickstart

```bash
git clone https://github.com/Harshit1259/AI-Incident-Intelligence.git
cd AI-Incident-Intelligence
./quickstart.sh
```

The script generates a `.env` with random secrets, builds the frontend, brings up PostgreSQL + backend + nginx via Docker Compose, and opens `http://localhost`.

Then register an account, and fire a test alert:

```bash
curl -s -X POST http://localhost:8080/api/v1/ingest/webhook \
  -H 'Content-Type: application/json' \
  -d '{"source":"test","service":"payments-api","severity":"critical","title":"High error rate"}'
```

An incident appears on the dashboard immediately. Send the same alert four more times — you still have one incident, with an event count of one. That's the dedup window doing its job.

> **AI features are optional.** Without `LLM_API_KEY`, root-cause analysis falls back to a built-in knowledge base of ~500 failure patterns and then to rule-based templates. The product has no dead screens with the LLM turned off.

<details>
<summary><b>Running without Docker</b></summary>

```bash
cp .env.example .env          # fill in POSTGRES_DSN and JWT_SECRET

docker run -d --name aiops-db -p 5432:5432 \
  -e POSTGRES_USER=aiops_user -e POSTGRES_PASSWORD=yourpassword \
  -e POSTGRES_DB=aiops postgres:16-alpine

cd backend && go build -o server ./cmd/server && ./server   # migrations run on startup
cd frontend && npm install && npm run dev                   # http://localhost:5173
```

</details>

---

## Architecture

```
  Alert sources                Go backend (:8080)                 React SPA
┌────────────────┐      ┌──────────────────────────────┐      ┌──────────────┐
│ Prometheus     │      │  Ingest ─ tenant resolution   │      │ Incident     │
│ Datadog        │─────▶│         ─ idempotency         │      │ command      │
│ PagerDuty      │ HTTP │         ─ normalization       │      │ centre       │
│ GitHub/GitLab  │      │            ↓                  │◀────▶│              │
│ OpenTelemetry  │      │  Correlation engine           │ REST │ Log explorer │
│ NeuroOps agent │      │    fingerprint · dedup · merge│      │ Fleet view   │
└────────────────┘      │            ↓                  │      │ SLO · ROI    │
                        │  AI layer                     │      └──────────────┘
                        │    KB → LLM → templates       │
                        │            ↓                  │
                        │  Policy engine → remediation  │
                        │            ↓                  │            nginx
                        │  Verification → auto-close    │      serves SPA and
                        │                               │      proxies /api/*
                        │          PostgreSQL 16        │
                        └──────────────────────────────┘
```

Every ingest path — webhook, agent, or OTLP — converges on a single function, `CorrelationService.ProcessEvent`, so noise reduction is enforced in exactly one place. All correlation state lives in PostgreSQL rather than process memory, which keeps the backend horizontally scalable.

---

## How correlation works

Three gates stand between a raw alert and a new incident.

**1 · Fingerprint.** Each event is hashed as `sha256(source | service | severity | normalizedTitle)`, truncated to 16 hex characters. The title normalizer strips instance-specific tokens first — pod name suffixes, UUIDs, IPv4 addresses, numeric IDs — so that `pod checkout-7f9d-x2k CPU high` and `pod checkout-3a1b-9mz CPU high` produce the same fingerprint.

**2 · Deduplication.** If that fingerprint was already seen within a five-minute window, the event is suppressed. A 500-alert storm becomes zero additional work.

**3 · Correlation.** The engine looks for an open incident to merge into — first by exact fingerprint at any age, so a recurring anomaly rejoins its original incident; then by service and *dependent* services within the correlation window, which is what collapses a database failure and the four APIs timing out behind it into a single incident.

Merging escalates severity but never downgrades it, and recomputes a 0–100 priority score from severity, event count, risk, recurrence and blast radius.

---

## Features

| | |
|---|---|
| **Alert correlation** | Fingerprint dedup + time-window and dependency-graph grouping |
| **AI root cause** | Structured RCA with confidence, evidence claims, and a `falsified_by` field per claim |
| **Incident copilot** | Chat against any incident's assembled context |
| **Auto-remediation** | Eight-check policy engine, Slack approval gates, automatic rollback |
| **Verification** | Before/after snapshots; auto-close only when proof items improve |
| **NeuroOps agent** | One-line install; streams metrics, logs, traces and topology from any host |
| **Predictive incidents** | Weighted linear regression forecasts threshold breaches before they fire |
| **SLO tracking** | Error budgets, burn rate, projected depletion |
| **Business impact** | Actual vs. counterfactual loss modelling with confidence bounds |
| **Noise reduction score** | The funnel from raw alerts to confirmed incidents, in hours and dollars |
| **On-call scheduling** | Rotations, follow-the-sun, overrides |
| **Post-mortems** | AI-drafted from the incident timeline |
| **Multi-tenancy** | Lifecycle isolation, per-tenant rate limits and field-level encryption |
| **Public status page** | Served at `/status` |

### Integrations

Paste the webhook URL from **Setup Guide → Connect Your First Source** into any tool.

| Tool | Endpoint |
|---|---|
| Generic / any tool | `POST /api/v1/ingest/webhook` |
| Prometheus Alertmanager | `POST /api/v1/ingest/prometheus` |
| PagerDuty | `POST /api/v1/ingest/pagerduty` |
| Datadog | `POST /api/v1/ingest/datadog` |
| GitHub (deployments, PRs) | `POST /api/v1/ingest/github` |
| GitLab | `POST /api/v1/ingest/gitlab` |
| OpenTelemetry | `POST /api/v1/ingest/otel` |
| NeuroOps agent | `POST /api/v1/ingest/agent` (HMAC-signed) |

---

## Engineering notes

The decisions worth explaining, and what each one costs.

**One external backend dependency.** The Go backend imports `github.com/lib/pq` and nothing else. JWT (HS256), PBKDF2-HMAC-SHA256, AES-256-GCM, HMAC request signing, the LLM client and the token-bucket rate limiter are all written against the standard library. For a system that stores customer credentials and executes commands on customer hosts, a small auditable surface was worth the implementation cost. The trade-off is real: hand-rolled crypto is a liability you take on knowingly, and a team with a security review process should prefer `x/crypto` and a maintained JWT library.

**No web framework, no ORM.** `net/http` with `ServeMux`, middleware as plain function composition, and hand-written SQL in one store per aggregate. Costs conventions and type-safe queries; buys explicit control over the request path and the query plan.

**Migrations as an ordered Go slice.** Every statement is `IF NOT EXISTS`, columns are never dropped or renamed, and the list is append-only — so migrations are idempotent and rolling back to a previous binary is always safe. Costs version tracking and down-migrations; a versioned tool with a `schema_migrations` table is the right upgrade past this scale.

**Provider-agnostic AI with three data modes.** `cloud` calls a public API with PII scrubbed from the prompt first; `private` calls a self-hosted OpenAI-compatible endpoint so no data leaves your infrastructure; `offline` disables LLM calls entirely. The `/ai/status` endpoint reports call counts and a SHA-256 prefix of the last prompt — enough to prove which prompt ran without storing its content.

**Edition gating in the router.** A `uint8` bitmask parsed from `PLATFORM_EDITIONS` decides which route groups get registered at startup, so a Core-only deployment does not have enterprise endpoints in its mux at all. Packaging enforced at the routing layer rather than a flag checked per request.

**Four independent brakes on automation.** Remediation only triggers on high and critical incidents; auto-execution additionally requires the policy to permit it *and* incident confidence ≥ 90; blast-radius checks cap concurrent actions and the number of services an incident may span; and verification rolls back automatically when confidence drops by 20 points or more. When no policy matches, the default is manual — fail-closed.

---

## Project structure

```
backend/
  cmd/server/main.go       composition root — wires every store, service and handler
  internal/
    routes/                route registration, grouped by feature
    middleware/            JWT, RBAC, tenancy, rate limiting, recovery, tracing
    handlers/              HTTP layer — decode, delegate, encode
    services/              business logic
    store/                 SQL, one store per aggregate
    models/                plain structs, no ORM tags
    platform/              cache, crypto, queue, ratelimit, edition, logger, trace
    ingest/                per-vendor payload normalizers
    llm/                   provider-agnostic LLM client
frontend/src/
  components/              47 panels and views
  api/                     one fetch wrapper with transparent 401 recovery
neuroops-agent/            separate Go module — collectors, plugin engine, transport
docs/                      architecture, ingestion, incidents, install
deploy/                    nginx config
```

The dependency direction is strictly `handler → service → store`. Handlers never touch SQL; stores never contain business rules.

---

## The NeuroOps agent

A standalone Go binary that streams host telemetry into the platform.

```bash
curl -fsSL http://localhost:8080/install.sh | TENANT_ID=<your-tenant> sh
```

It collects metrics via `gopsutil`, tails log files with position tracking, proxies OTLP traces, and runs plugins written in either Go or Python against a two-method contract (`Discover`, `Collect`) with SSH, SNMP and HTTP clients provided. A ZeroMQ SUB socket lets the platform push commands down to the host — `metric.poll`, `plugin.run`, `config.reload`, `agent.upgrade` — which is what makes closed-loop remediation possible without an inbound firewall hole.

Every ingest request is HMAC-SHA256 signed over a canonical string that includes the request body hash, a Unix timestamp and a 128-bit nonce. The timestamp window is ±5 minutes and nonces are tracked for 10 minutes, so replay is bounded on both axes.

---

## Configuration

| Variable | Required | Default | Description |
|---|:---:|---|---|
| `POSTGRES_DSN` | ● | — | PostgreSQL connection string |
| `JWT_SECRET` | ● | dev default | 32+ character random string for signing JWTs |
| `LLM_API_KEY` | | — | Anthropic or OpenAI key; enables AI RCA and copilot |
| `LLM_MODEL` | | `claude-sonnet-4-6` | Model identifier |
| `LLM_PROVIDER` | | auto-detected | `anthropic` \| `openai` \| `local` |
| `LLM_DATA_MODE` | | `cloud` | `cloud` \| `private` \| `offline` |
| `MASTER_ENCRYPTION_KEY` | | — | 64 hex chars; enables per-tenant field encryption |
| `AGENT_SECRET_KEY` | | derived | AES-256 key protecting per-agent signing secrets |
| `PLATFORM_EDITIONS` | | all | `core,enterprise,agent,saasops` |
| `SLACK_BOT_TOKEN` | | — | Enables Slack war-room auto-creation |
| `GITHUB_WEBHOOK_SECRET` | | — | HMAC secret for GitHub webhook verification |

See [`.env.example`](.env.example) for the full list.

---

## Development

```bash
make build          # backend + frontend
make test           # go test -race -cover ./...
make lint           # go vet + eslint
make docker         # build the container image

cd backend  && go run ./cmd/server
cd frontend && npm run dev
```

CI runs `go vet`, a cross-compiled build and `go test -race -cover ./...` on the backend, plus `npm ci`, lint and build on the frontend.

Tests concentrate on the algorithmic core — fingerprinting and correlation, JWT and RBAC middleware, tenant isolation, the rate limiter, edition gating, the knowledge base, and the predictive and business-impact models — because those are the places where a bug is silent and expensive. Store-layer integration tests against a real PostgreSQL instance are the most valuable thing still missing.

---

## Status and scope

This is a personal project built to explore what a modern AIOps platform actually requires end to end, and it is complete enough to run and demonstrate. It is not production-hardened, and a few things are known to need work before it would be:

- The correlation check-then-act is not atomic, so concurrent events for the same service can each create an incident. The fix is a partial unique index on `(tenant_id, fingerprint)` filtered to open statuses, or a Postgres advisory lock around the read-modify-write.
- Incident detail is fetched by ID without a tenant filter, and the correlation write path is not tenant-scoped. Both need `tenant_id` threaded through.
- Password hashing uses PBKDF2 at 10,000 iterations, below current OWASP guidance; `argon2id` is the right replacement.
- The service dependency map is hardcoded. The topology tables that should back it already exist and are populated by the agent — they are simply not wired in yet.

Contributions and issues are welcome.

---

## License

MIT — see [LICENSE](LICENSE).
