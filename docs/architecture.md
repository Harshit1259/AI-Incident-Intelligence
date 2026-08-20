# Architecture Overview

## System Design

```
Hosts (N agents)          Product Backend              Frontend
┌──────────────┐     ┌─────────────────────┐     ┌──────────────┐
│ NeuroOps     │     │                     │     │              │
│ Agent        │────>│ Ingestion Layer     │     │ Incident     │
│              │HTTP │   ↓                 │     │ Command      │
│ - Log tail   │     │ Normalization       │     │ Center       │
│ - Metrics    │     │   ↓                 │<───>│              │
│ - Traces     │     │ Correlation Engine  │REST │ Log Explorer │
└──────────────┘     │   ↓                 │     │              │
                     │ Knowledge Base (502)│     │ Agent Mgmt   │
External Tools       │   ↓                 │     │              │
┌──────────────┐     │ Incident Engine     │     │ Intelligence │
│ GitHub       │────>│   ↓                 │     │              │
│ PagerDuty    │     │ Business Impact     │     └──────────────┘
│ Datadog      │     │   ↓                 │
│ Prometheus   │     │ Verification        │
└──────────────┘     │                     │
                     │ PostgreSQL          │
                     └─────────────────────┘
```

## Data Flow

1. **Agent collects** logs from `/var/log/syslog`, metrics via gopsutil
2. **HTTP Forwarder** batches events (50/batch, 5s flush), POSTs to `/api/v1/ingest/agent`
3. **Ingestion service** stores all logs, pattern-matches errors
4. **Correlation engine** groups by fingerprint + time window → 1 incident
5. **Knowledge base** (502 entries) matches root cause, reasoning, resolution steps
6. **Incident engine** computes priority score, enriches with context logs/metrics
7. **Business impact engine** calculates actual/counterfactual/avoided loss
8. **Verification engine** checks if resolution worked (no new errors, metrics stable)

## Key Modules

| Module | Files | Purpose |
|--------|-------|---------|
| Ingestion | `handlers/agent_handler.go`, `services/agent_ingestion_service.go` | Accept agent data |
| Correlation | `services/correlation_service.go` | Dedup + group alerts → incidents |
| Knowledge Base | `services/knowledge_base.go`, `kb_*.go` | 502 pre-mapped incident patterns |
| Incident Detail | `services/incident_detail_service.go` | Build full incident view |
| Business Impact | `services/business_impact_service.go` | Financial loss calculation |
| Policy | `services/policy_service.go` | Execution guardrails |
| Verification | `services/verification_service.go` | Post-action health checks |
| Audit | `internal/audit/logger.go` | Immutable audit trail |

## Security

- JWT HS256 authentication
- PBKDF2-HMAC-SHA256 password hashing (10K iterations)
- RBAC: admin / operator / viewer roles
- Tenant isolation via `tenant_id` on all tables
- Panic recovery middleware
- No secrets in source code
