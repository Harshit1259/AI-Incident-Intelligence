# How Ingestion Works

## Signal Sources

The platform accepts signals from:

| Source | Endpoint | Format |
|--------|----------|--------|
| NeuroOps Agent | `POST /api/v1/ingest/agent` | Agent event batch |
| Generic Webhook | `POST /api/v1/ingest/webhook` | JSON event |
| Prometheus | `POST /api/v1/ingest/prometheus` | Alertmanager payload |
| GitHub | `POST /api/v1/ingest/github` | Push/deploy/release |
| GitLab | `POST /api/v1/ingest/gitlab` | Push/deploy hooks |
| PagerDuty | `POST /api/v1/ingest/pagerduty` | PD webhook v3 |
| Datadog | `POST /api/v1/ingest/datadog` | Monitor webhook |

## Agent Event Format

```json
{
  "event.type": "log",
  "event.source": "10.72.223.95",
  "event.category": "error",
  "log.source": "/var/log/syslog",
  "log.tag": "syslog",
  "log.message": "ERROR: payments-api: connection refused to database",
  "agent.id": "uuid",
  "object.ip": "10.72.223.95",
  "timestamp": 1712345678
}
```

## Processing Pipeline

### Step 1: Store
Every log is stored in `log_entries` table regardless of severity.

### Step 2: Pattern Match
`matchLogSeverity()` checks if the log is a real error:
- **Critical**: kernel panic, OOM kill, fatal error
- **High**: segfault, connection refused, auth failure
- **Medium**: timeout, service failed, too many open files
- **Skipped**: systemd, apparmor, desktop services, agent internals

### Step 3: Service Extraction
`extractServiceFromLog()` extracts the service name:
- Looks for `FATAL: service-name:` or `ERROR: service-name:` patterns
- Falls back to syslog process name
- Falls back to hostname

### Step 4: Event Creation
Creates an internal Event and passes to correlation engine.

## Agent Stop/Start

When an agent is stopped via the UI:
- `agent.status = 'stopped'` in the database
- All incoming events from that agent are rejected (not stored, not processed)
- `last_seen_at` is still updated (so you know the agent process is alive)
- Click Start to resume collection
