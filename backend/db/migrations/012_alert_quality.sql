-- Alert quality governance tables
-- Safe to re-run: all DDL is idempotent.

-- Alert rule registry — auto-populated from ingest events, enriched manually.
-- id = fingerprint of the alert rule.
CREATE TABLE IF NOT EXISTS alert_rules (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    name            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',    -- prometheus | datadog | pagerduty | custom
    service         TEXT NOT NULL DEFAULT '',
    team            TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL DEFAULT '',
    condition_expr  TEXT NOT NULL DEFAULT '',    -- raw alert expression if available
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_fired_at   TIMESTAMPTZ,
    fire_count_7d   INTEGER NOT NULL DEFAULT 0,
    fire_count_30d  INTEGER NOT NULL DEFAULT 0,
    fp_count        INTEGER NOT NULL DEFAULT 0,
    noise_count     INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant   ON alert_rules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_service  ON alert_rules (tenant_id, service);
CREATE INDEX IF NOT EXISTS idx_alert_rules_last_fired ON alert_rules (tenant_id, last_fired_at);
