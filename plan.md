# Integrations & Fingerprint Plan (future work)

Status: **parked**. The current focus is open-source observability and
monitoring integrations only (see "Current scope" at the bottom). This file
keeps the full plan for every other integration so it can be picked up later.

Written 2026-09-12.

---

## 1. Why this plan exists

Two problems found while tracing the alert pipeline:

1. **Most catalog integrations cannot deliver alerts.** 36 integrations in
   `backend/internal/services/marketplace_catalog.go` point at the generic
   webhook `/api/v1/ingest/webhook`. That endpoint only accepts JSON already in
   NeuroOps' own `Event` shape and requires a top-level `title`. Checked
   against each integration's own catalog sample payload:
   - Accepted (2): Grafana (legacy format), Wiz.
   - Rejected with 400 "title is required" and sent to the DLQ (33): New Relic,
     Dynatrace, CloudWatch, Azure Monitor, GCP, Zabbix, Nagios, PRTG, Icinga,
     OpsGenie, Jenkins, ArgoCD, Terraform, Ansible, AWS EC2/ECS/RDS/Lambda/SQS/
     CloudTrail, OCI, GCP Pub/Sub, Kubernetes, Elastic APM, Honeycomb,
     Lightstep, Jaeger, Kibana, Splunk, Loki, Sumo Logic, Snyk, Lacework.
   - Rejected as invalid JSON (1): Azure Event Grid (sends an array).
2. **The marketplace "Send test alert" hides this.** `MarketplaceService`
   builds a ready-made `models.Event` and calls `ProcessEvent` directly,
   skipping the webhook parser — the test is green while real alerts fail.

And on fingerprints:

- Only Prometheus sends a fingerprint we use (Alertmanager's, which includes
  the pod name, so pod restarts create new incidents).
- Datadog and PagerDuty send stable IDs that we ignore; their fingerprint is
  computed from the title.
- OTel metrics/traces and custom mappers get a unique fingerprint (never
  deduplicated); OTel logs without trace context all get `"-"`.

---

## 2. Principle: which ID is the fingerprint

The fingerprint drives dedup, merge-into-open-incident, and "seen before"
memory. It must identify **the alert definition on a thing** and stay the
same every time that alert fires.

| Kind of ID | Examples | Use for |
|---|---|---|
| Definition ID — same every time it fires | Datadog monitor ID + host, CloudWatch alarm ARN, Zabbix trigger + host, OpsGenie alias | **fingerprint** |
| Occurrence ID — new each time it fires | PagerDuty incident ID, Dynatrace ProblemID, GCP incident_id, Azure alertId | `external_id` only (linking back to the tool) |

---

## 3. End-to-end flow after the change

```
Tool sends its NATIVE payload to the URL we gave it (URLs don't change)
  → handler resolves the source token → knows the tool (source type)
  → that tool's MAPPER builds our event:
       title · service · device · severity · firing/resolved
       identity fields (what makes it "this alert on this thing")
       external_id (the tool's occurrence ID)
  → save event → ProcessEvent
  → ① Fingerprint(event)  — ONE function for every source
  → ② mute → ③ dedup → ④–⑥ correlation → ⑦ incident   (unchanged)
```

Routing needs no URL change: marketplace "enable" creates the source with the
integration ID as its type (`marketplace_service.go`, `CreateSource(…,
integrationID)`), so the generic webhook can pick the mapper from the token's
source type and fall back to today's format when there is no mapper.

---

## 4. Part A — one fingerprint function

`backend/internal/ingest/fingerprint.go`:

1. Identity fields: from the mapper; else standard label keys (alert name,
   device/host/instance, service, environment); else the normalized title.
2. Normalize: lowercase/trim; strip pod hashes, UUIDs, IPs, ports, timestamps,
   measured values (`95%`, `230ms`); keep host names and StatefulSet ordinals.
3. Hash: sort, join with `0xff`, SHA-256, prefix `v1:`.

Around it:
- `ProcessEvent` computes it for every event (not only when empty).
- ✅ `ToEvent` (`models/ingest_event.go`) no longer copies `ExternalID` into
  `Fingerprint` (done 2026-09-12).

---

## 5. Part B — identity per source

Field paths must be verified against each vendor's docs and a real payload
when the mapper is built.

| Source | Identity (→ fingerprint) | external_id |
|---|---|---|
| Prometheus | alert labels, pod names normalized | Alertmanager fingerprint |
| Grafana (unified alerting) | Alertmanager format — reuse Prometheus parser | Grafana fingerprint |
| Datadog | monitor ID (`alert_id`) + host | event ID |
| PagerDuty | PD service + dedup/incident key; else normalized title | PD incident ID |
| CloudWatch (via SNS) | alarm ARN | SNS MessageId |
| Azure Monitor | alert rule + target resource | `alertId` |
| GCP Monitoring | policy + condition + resource | `incident_id` |
| OpsGenie | `alias` | `alertId` |
| Nagios | host + service description | — |
| Zabbix | trigger ID + host | event ID |
| Dynatrace | problem title + impacted entity | ProblemID |
| New Relic | condition ID + entity | incident/issue ID |
| Kubernetes events | involved object (kind/ns/name) + reason | event UID |
| AWS EventBridge | `source` + `detail-type` + `resources` | event `id` |
| AWS RDS events | RDS event code + Source ID | — |
| Event Grid / Pub/Sub / OCI | event type + subject/resource | message ID |
| Splunk | search name + key result fields | `sid` |
| Wiz / Snyk / Lacework | issue/event definition + resource | issue/event ID |
| OTel metrics | metric name + service + host + datapoint attrs minus value | — |
| OTel logs / traces | service + span name or log template + error type | trace/span ID |
| Generic webhook | `dedup_key` if sent; else `labels.alertname` + host + service + env; else normalized title | sender `external_id` |
| Custom mapper | mapped service/resource/title-template fields, or stable `external_id` | mapped `external_id` |

---

## 6. Part C — mappers

- One file per tool: `backend/internal/ingest/mappers/<tool>.go`, registered
  in the existing `MapperRegistry`.
- Each fills: title, service, device (`resource`), severity, firing/resolved,
  identity fields, external_id.
- CloudWatch/SNS: handle `SubscriptionConfirmation`; only confirm when the
  SubscribeURL is a genuine AWS SNS host — never follow arbitrary URLs.
- "Send test alert": send the tool's real sample payload through the real
  mapper and pipeline; report parse errors.
- Catalog sample payloads: replace with each vendor's real format.
- CI/CD tools (Jenkins, ArgoCD, Terraform, Ansible) feed change correlation,
  not incidents.

---

## 7. Tests

- Fingerprint table tests + benchmark (pod suffix, UUID, IPv4/IPv6, ports,
  timestamps, job suffix, keep StatefulSet ordinal, keep numeric hosts,
  measured values).
- Per mapper, with real payloads: same alert twice → same fingerprint;
  different device → different fingerprint; fields mapped; resolved detected.
- Routing: integration token on the generic URL uses its mapper; plain
  webhook token keeps today's format.
- Live run on a throwaway database.

## 8. Rollout

New fingerprints carry `v1:`. Incidents open at deploy time won't match new
alerts by fingerprint (they still group by service within 5 minutes). "Seen
before" history restarts per alert. Nothing is deleted.

## 9. Related follow-ups (not in this plan)

- ✅ Close incidents on "resolved" messages — done for Prometheus/Grafana 2026-09-12.
- Extend alert mutes to these sources.
- ✅ Correlation: `tenant_id` added to the dedup/fingerprint/service queries
  and auto-resolve rule lookup (2026-09-13 — see "Fixes 2026-09-13").
- Correlation: replace the hardcoded `config.ServiceDependencies` map with the
  topology graph; record the real merge reason.
- ✅ Migration 21 (`typed_timestamps`) fresh-database failure — fixed 2026-09-12.
- Store Prometheus `runbook_url` (and other annotations) and show it as a link.

---

## Current scope (decided 2026-09-12)

Only these five integrations stay in the product, and will be made smooth
and seamless step by step:

1. **OpenTelemetry** — `/api/v1/otel/logs|metrics|traces`
2. **Prometheus** (Alertmanager) — `/api/v1/ingest/prometheus`
3. **Grafana** — Grafana alerting (Alertmanager-compatible webhook)
4. **Jaeger** — tracing backend (no alerting of its own; traces via OTLP)
5. **Zabbix** — webhook media type

Every other integration is removed from the product for now and listed below
so it can be re-added later using sections 1–8 of this plan.

### Known gaps in the five (to fix while making them smooth)

Fixed on 2026-09-12 (tests added for each; verified on a fresh database):

- ✅ Security: `X-Tenant-ID` header bypassed the source token on the
  Prometheus **and** OTel endpoints (`TenantFromRequestStrict` now trusts only
  JWT claims). Change-intelligence endpoints read the tenant from the header
  instead of the login — now use the JWT tenant.
- ✅ "Send test event" for a source always returned 401 (it replayed a
  scrubbed token); now runs the real Alertmanager parser directly.
- ✅ Fresh databases lacked 6 tables and 8 `action_audit` columns
  (db/migrations/007, 014, 016, 017, 019 were never run); added as built-in
  migration `apply_unrun_sql_files`.
- ✅ Migration 21 (`typed_timestamps`) failed on fresh databases.
- ✅ OTel: gzip bodies accepted (decompressed size capped at 8 MiB);
  protobuf gets a clear 415 telling users to set `encoding: json`.
- ✅ Grafana / older Alertmanagers: ingest endpoints accept
  `Authorization: Bearer <source token>` as well as `X-Source-Token`.
- ✅ OTel logs without trace context all got event ID `"-"` (overwriting each
  other across tenants); error spans in one trace collided; `ToEvent` copied
  the ID into the fingerprint so OTel/custom events never deduplicated.
- ✅ Prometheus/Grafana `runbook_url` is stored and shown as a link.
- ✅ Marketplace "Send test alert" now runs each integration's real sample
  payload through its real parser; catalog samples are tested against parsers.
- ✅ 413/415 errors returned code `INTERNAL_ERROR`.

- ✅ Auto-close on recovery (Prometheus/Grafana `status: resolved`): an
  incident closes automatically once every alert in it has recovered and
  stayed quiet for `AUTO_CLOSE_QUIET_PERIOD` (default 5m). Re-fire cancels;
  incidents holding alerts that cannot resolve (OTel, custom) never
  auto-close; a resolve for an alert in no open incident is ignored. Shown as
  a badge on the incident; history records actor `neuroops-auto-close`.
- ✅ Prometheus event IDs are now tenant-scoped (`am-<tenant>-<fingerprint>`)
  and the event upsert no longer rewrites `tenant_id` — two tenants with
  identical alert labels used to share (and take over) one event row.
- ✅ A re-notified or re-fired alert failed to update its incident (duplicate
  `incident_events` link rolled back the whole update, error discarded).

---

## Build plan — approved 2026-09-12 — ✅ steps 1–5 built and verified 2026-09-12

Verified end to end: sources API + screen (create/test/rotate/delete, token
shown once and filled into the setup), exporter-named service → namespace/
workload, custom-source routes 404, **real Zabbix 7.0.30** (import of our media
type → problem → incident, acknowledgement → timeline note, recovery →
auto-close; Zabbix delivery log all "sent"), and a **real OpenTelemetry
Collector 0.120 with default settings** (protobuf + gzip) delivering logs,
metrics and traces. Findings folded in: Zabbix needs a *dedicated* NeuroOps
user (it never sends update notifications to the user who made the update);
"Inaccessible user" shown as "a Zabbix user"; new sources show "waiting for
data" instead of "healthy".

(build order below)

### ✅ Step 1 — Sources screen (decision 6: yes)

No screen creates a source or shows its token today; every integration
needs one.

- Integrations → `<tool>` → **Connect**: name the source → **Create** →
  token shown once with **Copy** → the tool's config snippet with the token
  already filled in → **Send test alert** (runs the real parser).
- **Sources list**: name, integration, last event, health, **Rotate token**,
  **Delete**.
- Backend: rotate-token and delete endpoints (tenant-scoped, operator+);
  tokens never returned except at create/rotate.
- Tests: create/rotate/delete, token shown once, other tenant cannot see or
  rotate, test alert per integration.

### ✅ Step 2 — Exporter-named `service` label (decision 3c: auto-fix + hint)

Also: node-exporter alerts → `node/<node>`, blackbox probes → `probe/<target>`.

- When a Prometheus/Grafana alert's `service` is a known exporter name
  (`kube-state-metrics`, `node-exporter`, `blackbox-exporter`,
  `kube-prometheus-stack-*`, `prometheus-node-exporter`, `kubelet`,
  `cadvisor`), derive the service in order: `namespace/container` →
  `namespace/pod` (pod suffix removed) → `namespace` → `job`.
- Keep the original value as label `service.original`.
- Incident shows "Service taken from namespace/container because the service
  label named the exporter"; Prometheus setup page gets a tip.
- Tests: each fallback step; a real app `service` label is never replaced.

### ✅ Step 3 — Switch off custom sources (decision 7: nothing custom)

- Remove the route `/api/v1/ingest/custom/`, the schema-registry routes and
  its screen; keep the code; list them under "Removed integrations".
- Endpoint returns 404 (like the other removed ones).

### ✅ Step 4 — Zabbix (decision 4: yes; defaults below unless changed)

Files: `deploy/zabbix/neuroops-webhook.js` (script), `deploy/zabbix/build-media-type.py` → `neuroops-zabbix.yaml` (also served at `/integrations/neuroops-zabbix.yaml`), `backend/internal/handlers/zabbix_ingest.go`.

Flow: customer imports our webhook media type (`neuroops-zabbix.yaml`, a
JavaScript script) → sets URL + token → adds the media to a user → creates a
trigger action with problem, recovery and update operations → Zabbix POSTs
our JSON to `/api/v1/ingest/zabbix` with `Authorization: Bearer <token>`
and `X-Idempotency-Key: <event_id>-<event_value>-<update_status>`.

| Zabbix field | NeuroOps |
|---|---|
| `event_id` | event ID `zbx-<tenant>-<event_id>` (problem + recovery share it → auto-close) |
| `trigger_id` + `host` | fingerprint (definition identity) |
| `event_name` | title |
| `host` | device (mutes, correlation) |
| tag `service` → `host_group` → `host` | service |
| `severity` | Disaster→critical, High→high, Average→medium, Warning→low, Information/Not classified→info |
| `item_value` | value (mute ranges) |
| `event_value` 1/0 | firing / resolved → auto-close |
| `update_status` 1 | line on the incident timeline ("Acknowledged in Zabbix by …: …") |
| `zabbix_url` | "Open in Zabbix ↗" link |

- Supported: Zabbix 6.0 LTS and 7.0 LTS (5.x: paste the script by hand).
  Verify macro names against those docs while building.
- Zabbix's media-type **Test** button gets a friendly "test received" reply.
- Mutes extended to Zabbix alerts.
- Setup guide on the Integrations page with the media-type download.
- Tests: problem → incident; recovery → auto-close; retry → no duplicate;
  update → timeline line; bad token → 401; severity table; service order.
- Later (not now): two-way ack via the Zabbix API.

### ✅ Step 5 — OTLP protobuf over HTTP (decision 5)

Added `go.opentelemetry.io/proto/otlp` and `google.golang.org/protobuf`
(only the *Data message packages — no gRPC in the dependency graph). Protobuf
bodies are converted to OTLP JSON (enums as numbers, trace/span IDs hex) and
use the existing mappers; protobuf requests get a protobuf-style 200 reply.
OTLP over **gRPC (port 4317) is still not supported** — add when customers ask.

### ✅ Step 6 — Grafana (approved 2026-09-13, built and verified 2026-09-13)

Researched on a real Grafana 13.2.1 (payload captures are the test fixtures in
`backend/internal/handlers/testdata/grafana/`). Decisions as approved:

- **G1** Own endpoint `POST /api/v1/ingest/grafana` (`grafana_ingest.go`).
  Grafana sources set up earlier against `/ingest/prometheus` keep working:
  a `grafana`-typed token there gets the Grafana mapping.
- **G2** Service: `service` label (exporter names derived, as for
  Prometheus) → `grafana_folder` → rule name.
- **G3** `DatasourceNoData` / `DatasourceError` become incidents titled
  "No data: <rule>" / "Query error: <rule>", label
  `grafana.monitoring_gap`, severity **`unknown`** (per user, not low). The
  rule's own summary is not used — it would claim a problem nobody saw.
  `unknown` is a new severity: lowest weight when an incident's severity is
  chosen (a real alert wins), priority score like medium so gaps aren't buried,
  allowed in the incident filter, neutral pill in the UI.
- **G4** Grafana's contact-point **Test** button (alertname `TestAlert`, no
  ruleUID) creates a **"[Test] Grafana contact point test — <source>"**
  incident (per user: lets them check the whole path). Each press is its own
  event; the alert is marked recovered at once so the incident closes itself
  after the auto-close quiet period (Grafana never sends "resolved" for a
  test). Test alerts are never muted. NeuroOps' own Test button sends the same
  payload.
- **G5** No `severity` label → medium.
- **G6** HMAC signing: later.
- Event ID `gf-<tenant>-<orgId>-<fingerprint>` (firing and resolved share it).
  Value: first non-threshold entry of `valueString` (rounded to 2 dp), else a
  single `values` entry, else a `value` annotation → `grafana.value`, used by
  mute value ranges. Links stored: `grafana.rule_url` (generatorURL),
  `grafana.dashboard_url`, `grafana.panel_url`, plus `grafana.rule_uid`,
  `grafana.org_id`, runbook. Incident UI: "Open rule in Grafana ↗",
  "Dashboard ↗", value, "test" and "monitoring gap" pills.
- Mutes cover Grafana alerts (alert name, device from host/instance labels,
  value range).
- Setup guide: UI steps (Grafana 11/12/13 field names), provisioning YAML with
  the token filled in, `root_url` tip (links otherwise point to
  localhost:3000), notification-policy tip.
- E2E on Grafana 13.2.1: provisioned contact point from the guide's YAML →
  Test → test incident, auto-closed; CPU rule fires → critical incident with
  value, runbook and links; resolves → auto-closed; 2-series disk rule → one
  incident, two alerts; no-data rule → "No data" incident, severity unknown.

### ✅ Fixes 2026-09-13 (after the Grafana step)

- **Tenant isolation in correlation (bug).** `FingerprintSeenInWindow`,
  `FindOpenIncidentByFingerprint`, `FindOpenIncidentForService(s)` and
  `FindMatchingRules` ignored the tenant: a customer's alert could be dropped
  as a "duplicate" of another customer's, merged into their incident, or
  auto-resolved by their rules. All now take the tenant. Test:
  `internal/store/correlation_db_test.go` (real Postgres via
  `TEST_POSTGRES_DSN`; fails on the old queries).
- **Recovery keeps the firing value** (user decision): a resolved message no
  longer overwrites `annotation.value` / `grafana.value` /
  `zabbix.item_value` / `otel.metric.value` (`models.AlertValueLabels`,
  merged in the event upsert).
- **Self-service sign-up (bug):** only the first company in the system could
  get an admin; later sign-ups got 403 or a tenant with no admin. The first
  user of a brand-new tenant is now its admin (sign-up into an existing
  tenant is still refused).
- **Dashboard showed platform-wide numbers to every tenant:** "Events
  ingested" (never incremented — always 0) now sums the tenant's sources;
  "Active agents" uses the tenant's agents; cross-tenant totals removed from
  "Platform internals". Sidebar showed "Operator" for everyone — now the real
  role.
- **docker-compose** did not pass `AUTO_CLOSE_QUIET_PERIOD` to the backend.
- **Lint clean:** set-state-in-effect in Dashboard/IncidentWorkspace (loaders
  now set state in `.then` callbacks; the incident list/detail also ignore
  stale responses), `no-unused-vars` for destructured components.
- **Verified end to end with the real tools:** Prometheus 3.5 +
  Alertmanager 0.28 (two tenants, identical alerts → separate incidents; two
  fire/resolve/auto-close cycles), Grafana 11.6 / 12.2 / 13.2 (provisioning
  YAML from the guide, Test button, firing, no-data), Zabbix 6.0.48 and 7.0
  (media type import, problem, ack note, recovery → auto-close, firing value
  kept), OTel Collector 0.120 (protobuf + gzip), and the docker-compose stack
  through nginx with all of them.

### On hold (per user, 2026-09-12)
- **Jaeger**: Jaeger sends nothing itself — today's "Jaeger" entry is the OTel
  traces setup under another name. Recommendation: fold trace ingestion into
  the OTel integration, and make Jaeger a link-out integration (customer
  enters the Jaeger UI URL; trace-based alerts get "Open trace in Jaeger ↗").
- **Trace alerts (8)**: recommendation is NeuroOps as a problem detector (not
  a trace store): keep error alerts now and show the real failure count;
  recommend a Collector filter so only failed/slow traces are sent; then add
  slow-request alerts; later error-rate/latency summaries.
- **Fingerprint quality**: our own fingerprint from cleaned labels (the
  `Fingerprint(Alert)` drill). ✅ Written 2026-09-13 in
  `backend/internal/fingerprint/` (30 table cases + value table + benchmark;
  pod suffixes parsed by hand — regex backtracking was 62% of the cost).
  **Not wired in yet.** Open decisions before wiring: which sources use it
  (those without their own fingerprint: OTel, Zabbix already has
  trigger+host), and whether IPs in `instance` should merge different hosts
  (today `10.0.3.17` and `10.0.3.18` give the same fingerprint, per the drill).

---

## Removed integrations — to re-add later

### Integrations Hub page (`frontend/src/components/IntegrationsHub.jsx`)

| Entry | What it did | Backend it relied on |
|---|---|---|
| GitHub | Deploy/push/release events → "What changed" on incidents | `/api/v1/ingest/github`, `GitHubWebhookHandler`, `ChangeLinkerService`, env `GITHUB_WEBHOOK_SECRET` |
| GitLab | Same as GitHub | `/api/v1/ingest/gitlab`, `GitLabWebhookHandler`, `ChangeLinkerService`, env `GITLAB_WEBHOOK_TOKEN` |
| PagerDuty | Import PD incidents (webhook v3) as alerts | `/api/v1/ingest/pagerduty`, `PagerDutyWebhookHandler`, env `PAGERDUTY_WEBHOOK_SECRET` |
| Datadog | Monitor alerts via Datadog webhook | `/api/v1/ingest/datadog`, `DatadogWebhookHandler`, env `DATADOG_WEBHOOK_SECRET`, `DATADOG_API_KEY` |
| Slack | `/aiops` slash command, Ack/Resolve buttons, auto channel per critical incident | `/api/v1/slack/command`, `/api/v1/slack/interaction`, `SlackHandler`, `SlackService`, env `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET` |
| Generic webhook | Any tool posting NeuroOps-shaped JSON | `/api/v1/ingest/webhook`, `IngestHandler.GenericWebhook` |
| Status page (listed in the hub) | Public status API | `/api/v1/status` (a product feature, not a third-party integration) |

### Marketplace catalog (`backend/internal/services/marketplace_catalog.go`)

Monitoring: New Relic, Dynatrace, AWS CloudWatch, Azure Monitor, GCP
Operations Suite, Nagios, PRTG Network Monitor, Icinga 2, Datadog.

Ticketing: Jira, ServiceNow, Zendesk, Freshservice, Linear, GitHub Issues.

Communication / on-call: Slack, Microsoft Teams, PagerDuty, OpsGenie, Splunk
On-Call (VictorOps), Cisco Webex, Discord, WhatsApp Business.

CI/CD: GitHub Actions, GitLab CI/CD, Jenkins, ArgoCD, Terraform Cloud,
Ansible / AWX.

Cloud: AWS EC2, AWS ECS, AWS RDS, AWS Lambda, AWS SQS, AWS CloudTrail, Azure
Event Grid, GCP Pub/Sub Push, Oracle Cloud (OCI), Kubernetes Events.

APM: Elastic APM, Honeycomb, ServiceNow Lightstep.

Logging: Elastic / Kibana, Splunk, Grafana Loki, Sumo Logic.

Security: Wiz, Snyk, Lacework.

(Kept from the catalog: Prometheus, Grafana, Zabbix, Jaeger. OpenTelemetry
was never in the catalog and is added.)

### What was removed from the code (2026-09-12, option A: hide and disconnect)

- Routes: `/api/v1/ingest/webhook`, `/api/v1/ingest/datadog`,
  `/api/v1/ingest/pagerduty`, `/api/v1/ingest/github`,
  `/api/v1/ingest/gitlab`, `/api/v1/slack/command`,
  `/api/v1/slack/interaction`.
- Handlers deleted (recover from git history):
  `handlers/datadog_webhook_handler.go`, `pagerduty_webhook_handler.go`,
  `github_webhook_handler.go`, `gitlab_webhook_handler.go`,
  `slack_handler.go`, and `IngestHandler.GenericWebhook`.
- Config keys removed: `GITHUB_WEBHOOK_SECRET`, `GITLAB_WEBHOOK_TOKEN`,
  `DATADOG_API_KEY`, `DATADOG_WEBHOOK_SECRET`, `PAGERDUTY_WEBHOOK_SECRET`
  (config, schema, `.env.example`, `docker-compose.yml`).
- Catalog trimmed to five entries; Integrations page and onboarding wizard
  show only the supported endpoints.
- Custom sources switched off: routes `/api/v1/ingest/custom/{type}` and
  `/api/schema-registry` removed (handlers, `CustomMapper` and the unused
  `SchemaRegistryPanel.jsx` kept).
- Kept but switched off (no feed or not configured): `SlackService`,
  `WhatsAppService`, `TeamsService`, `JiraService`, `ServiceNowService`,
  `ChangeLinkerService`, and the marketplace's outbound connection probes.

### Outbound services wired into other features

These are integrations too, but other features call them directly:

| Service | Used by |
|---|---|
| `SlackService` | `CorrelationService` (channel per critical incident), `RemediationOrchestrator` (approval messages), `WorkflowService` (status comms), Slack handler |
| `WhatsAppService` + `WhatsAppHandler` | `CorrelationService` (critical incident alerts), WhatsApp panel |
| `TeamsService`, `JiraService`, `ServiceNowService` | `WorkflowService` (incident tickets and stakeholder updates), marketplace connection tests |
| `ChangeLinkerService` | GitHub/GitLab webhooks → "What changed" panel and change correlation |
