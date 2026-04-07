package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"ai-incident-platform/backend/internal/config"

	_ "github.com/lib/pq"
)

func NewDB(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// runMigrations applies all schema changes in order.
// Every statement uses IF NOT EXISTS / IF EXISTS so it is safe to run on
// every startup — idempotent by design.
func runMigrations(db *sql.DB) error {
	log.Println("db: running migrations…")

	for i, migration := range migrations {
		if _, err := db.Exec(migration.sql); err != nil {
			return fmt.Errorf("migration %02d (%s): %w", i+1, migration.name, err)
		}
		log.Printf("db: migration %02d (%s) ok", i+1, migration.name)
	}

	log.Printf("db: all %d migrations applied", len(migrations))
	return nil
}

type migration struct {
	name string
	sql  string
}

// migrations is the ordered list of all schema changes.
// Rules:
//   - Use ADD COLUMN IF NOT EXISTS, CREATE TABLE IF NOT EXISTS, CREATE INDEX IF NOT EXISTS
//   - Never DROP or rename columns (backwards-safe)
//   - Keep sorted oldest → newest
var migrations = []migration{
	// ─── 000: Core tables ────────────────────────────────────────────────────
	{
		name: "core_tables",
		sql: `
CREATE TABLE IF NOT EXISTS events (
    id          TEXT PRIMARY KEY,
    source      TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL DEFAULT '',
    service     TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS incidents (
    id                      TEXT PRIMARY KEY,
    service                 TEXT NOT NULL,
    severity                TEXT NOT NULL,
    status                  TEXT NOT NULL DEFAULT 'open',
    first_event_time        TEXT NOT NULL DEFAULT '',
    last_event_time         TEXT NOT NULL DEFAULT '',
    title                   TEXT NOT NULL DEFAULT '',
    correlation_pattern     TEXT NOT NULL DEFAULT '',
    correlation_score       INTEGER NOT NULL DEFAULT 0,
    correlation_reason      TEXT NOT NULL DEFAULT '',
    confidence              INTEGER NOT NULL DEFAULT 0,
    risk_score              INTEGER NOT NULL DEFAULT 0,
    event_count             INTEGER NOT NULL DEFAULT 1,
    root_cause_summary      TEXT NOT NULL DEFAULT '',
    root_cause_type         TEXT NOT NULL DEFAULT '',
    reasoning_json          TEXT NOT NULL DEFAULT '[]',
    what_changed_type       TEXT NOT NULL DEFAULT '',
    what_changed_service    TEXT NOT NULL DEFAULT '',
    what_changed_version    TEXT NOT NULL DEFAULT '',
    what_changed_description TEXT NOT NULL DEFAULT '',
    what_changed_timestamp  TEXT NOT NULL DEFAULT '',
    impacted_services_json  TEXT NOT NULL DEFAULT '[]',
    impact_count            INTEGER NOT NULL DEFAULT 0,
    seen_before             BOOLEAN NOT NULL DEFAULT false,
    recurring_count         INTEGER NOT NULL DEFAULT 0,
    similar_incident_id     TEXT NOT NULL DEFAULT '',
    last_seen_at            TEXT NOT NULL DEFAULT '',
    fingerprint             TEXT NOT NULL DEFAULT '',
    tenant_id               TEXT NOT NULL DEFAULT 'default'
);

CREATE TABLE IF NOT EXISTS incident_events (
    incident_id TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    PRIMARY KEY (incident_id, event_id)
);

CREATE TABLE IF NOT EXISTS changes (
    id          SERIAL PRIMARY KEY,
    service     TEXT NOT NULL,
    type        TEXT NOT NULL,
    version     TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    timestamp   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS incident_status_history (
    id              SERIAL PRIMARY KEY,
    incident_id     TEXT NOT NULL,
    previous_status TEXT NOT NULL DEFAULT '',
    new_status      TEXT NOT NULL,
    note            TEXT NOT NULL DEFAULT '',
    changed_by      TEXT NOT NULL DEFAULT 'system',
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS action_audit (
    id          SERIAL PRIMARY KEY,
    incident_id TEXT NOT NULL DEFAULT '',
    action_id   TEXT NOT NULL DEFAULT '',
    label       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    executed_by TEXT NOT NULL DEFAULT 'system',
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    result      TEXT NOT NULL DEFAULT ''
);
`,
	},

	// ─── 001: Backfill columns on existing tables BEFORE creating indexes.
	//         ADD COLUMN IF NOT EXISTS is idempotent — safe to re-run every startup.
	{
		name: "backfill_columns",
		sql: `
ALTER TABLE events    ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE events    ADD COLUMN IF NOT EXISTS title       TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS tenant_id   TEXT NOT NULL DEFAULT 'default';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS correlation_pattern      TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS correlation_score        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS correlation_reason       TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS confidence               INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS risk_score               INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS event_count              INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS root_cause_summary       TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS root_cause_type          TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS reasoning_json           TEXT NOT NULL DEFAULT '[]';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_type        TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_service     TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_version     TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_description TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_timestamp   TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS impacted_services_json   TEXT NOT NULL DEFAULT '[]';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS impact_count             INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS seen_before              BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS recurring_count          INTEGER NOT NULL DEFAULT 0;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS similar_incident_id      TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS last_seen_at             TEXT NOT NULL DEFAULT '';
`,
	},

	// ─── 002: Indexes — all columns are guaranteed to exist by migration 001 ─
	{
		name: "core_indexes",
		sql: `
CREATE INDEX IF NOT EXISTS idx_incidents_status             ON incidents (status);
CREATE INDEX IF NOT EXISTS idx_incidents_severity           ON incidents (severity);
CREATE INDEX IF NOT EXISTS idx_incidents_service            ON incidents (service);
CREATE INDEX IF NOT EXISTS idx_incidents_last_event_time    ON incidents (last_event_time DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_status_severity_ts ON incidents (status, severity, last_event_time DESC);
CREATE INDEX IF NOT EXISTS idx_incident_events_incident_id  ON incident_events (incident_id);
CREATE INDEX IF NOT EXISTS idx_events_fingerprint           ON events (fingerprint);
CREATE INDEX IF NOT EXISTS idx_events_fingerprint_ts        ON events (fingerprint, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_service_ts            ON events (service, timestamp);
CREATE INDEX IF NOT EXISTS idx_incidents_service_status_ts  ON incidents (service, status, last_event_time);
CREATE INDEX IF NOT EXISTS idx_incidents_tenant_id          ON incidents (tenant_id);
CREATE INDEX IF NOT EXISTS idx_incidents_fingerprint        ON incidents (fingerprint);
CREATE INDEX IF NOT EXISTS idx_changes_service_timestamp    ON changes (service, timestamp DESC);
`,
	},

	// ─── 003: Tenants + Users ────────────────────────────────────────────────
	{
		name: "tenants_and_users",
		sql: `
CREATE TABLE IF NOT EXISTS tenants (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO tenants (id, name, slug)
VALUES ('default', 'Default Tenant', 'default')
ON CONFLICT DO NOTHING;

-- Drop the users table if it has the wrong id type (SERIAL/integer).
-- No user was ever successfully inserted so this is always safe.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'users'
          AND column_name = 'id'
          AND data_type IN ('integer', 'bigint', 'smallint')
    ) THEN
        DROP TABLE users;
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL DEFAULT 'default',
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'operator',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email     ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users (tenant_id);
`,
	},

	// ─── 004: Phase 2 tables ─────────────────────────────────────────────────
	{
		name: "phase2_tables",
		sql: `
CREATE TABLE IF NOT EXISTS post_mortems (
    id                  TEXT PRIMARY KEY,
    incident_id         TEXT NOT NULL,
    tenant_id           TEXT NOT NULL DEFAULT 'default',
    title               TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'draft',
    executive_summary   TEXT NOT NULL DEFAULT '',
    timeline_narrative  TEXT NOT NULL DEFAULT '',
    root_cause          TEXT NOT NULL DEFAULT '',
    impact              TEXT NOT NULL DEFAULT '',
    resolution          TEXT NOT NULL DEFAULT '',
    action_items_json   TEXT NOT NULL DEFAULT '[]',
    lessons             TEXT NOT NULL DEFAULT '',
    generated_by        TEXT NOT NULL DEFAULT 'llm',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_post_mortems_incident_id ON post_mortems (incident_id);
CREATE INDEX IF NOT EXISTS idx_post_mortems_tenant_id ON post_mortems (tenant_id);
`,
	},

	// ─── 005: Phase 2 columns ────────────────────────────────────────────────
	{
		name: "phase2_columns",
		sql: `
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS slack_channel_id TEXT NOT NULL DEFAULT '';
`,
	},

	// ─── 006: Phase 3 — SLO tracking + On-call scheduling ────────────────────
	{
		name: "phase3_slo_oncall",
		sql: `
CREATE TABLE IF NOT EXISTS slo_definitions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    target_percent  REAL NOT NULL DEFAULT 99.9,
    window_days     INTEGER NOT NULL DEFAULT 30,
    metric_type     TEXT NOT NULL DEFAULT 'availability',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS slo_measurements (
    id              SERIAL PRIMARY KEY,
    slo_id          TEXT NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    total_requests  BIGINT NOT NULL DEFAULT 0,
    good_requests   BIGINT NOT NULL DEFAULT 0,
    bad_minutes     INTEGER NOT NULL DEFAULT 0,
    error_budget_remaining REAL NOT NULL DEFAULT 100.0
);

CREATE INDEX IF NOT EXISTS idx_slo_definitions_service ON slo_definitions (service);
CREATE INDEX IF NOT EXISTS idx_slo_definitions_tenant ON slo_definitions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_slo_measurements_slo_id ON slo_measurements (slo_id);
CREATE INDEX IF NOT EXISTS idx_slo_measurements_ts ON slo_measurements (slo_id, timestamp DESC);

CREATE TABLE IF NOT EXISTS oncall_schedules (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    team_name       TEXT NOT NULL,
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    rotation_type   TEXT NOT NULL DEFAULT 'weekly',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oncall_members (
    id              SERIAL PRIMARY KEY,
    schedule_id     TEXT NOT NULL,
    user_name       TEXT NOT NULL,
    user_email      TEXT NOT NULL DEFAULT '',
    position        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS oncall_overrides (
    id              SERIAL PRIMARY KEY,
    schedule_id     TEXT NOT NULL,
    original_user   TEXT NOT NULL DEFAULT '',
    override_user   TEXT NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    reason          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_oncall_schedules_tenant ON oncall_schedules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_oncall_members_schedule ON oncall_members (schedule_id);
CREATE INDEX IF NOT EXISTS idx_oncall_overrides_schedule ON oncall_overrides (schedule_id);
`,
	},

	// ─── 007: Phase 3 — Metrics + Anomaly alerts ─────────────────────────────
	{
		name: "phase3_metrics",
		sql: `
CREATE TABLE IF NOT EXISTS anomaly_alerts (
    id              SERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL,
    metric_name     TEXT NOT NULL,
    current_value   REAL NOT NULL DEFAULT 0,
    threshold       REAL NOT NULL DEFAULT 0,
    trend           TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL DEFAULT 'warning',
    message         TEXT NOT NULL DEFAULT '',
    fired_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged    BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS incident_metrics (
    id              SERIAL PRIMARY KEY,
    incident_id     TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    detected_at     TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    ttd_seconds     INTEGER NOT NULL DEFAULT 0,
    tta_seconds     INTEGER NOT NULL DEFAULT 0,
    ttr_seconds     INTEGER NOT NULL DEFAULT 0,
    responder       TEXT NOT NULL DEFAULT '',
    is_autoresolved BOOLEAN NOT NULL DEFAULT false,
    toil_minutes    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_anomaly_alerts_service ON anomaly_alerts (service);
CREATE INDEX IF NOT EXISTS idx_anomaly_alerts_tenant ON anomaly_alerts (tenant_id);
CREATE INDEX IF NOT EXISTS idx_incident_metrics_incident ON incident_metrics (incident_id);
CREATE INDEX IF NOT EXISTS idx_incident_metrics_tenant ON incident_metrics (tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_incident_metrics_incident_unique ON incident_metrics (incident_id);
`,
	},

	// ─── 008: Phase 4 — Billing + Onboarding ────────────────────────────────
	{
		name: "phase4_billing",
		sql: `
CREATE TABLE IF NOT EXISTS billing_plans (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    tier            TEXT NOT NULL DEFAULT 'starter',
    price_cents     INTEGER NOT NULL DEFAULT 0,
    max_incidents   INTEGER NOT NULL DEFAULT 100,
    max_users       INTEGER NOT NULL DEFAULT 3,
    max_services    INTEGER NOT NULL DEFAULT 5,
    features_json   TEXT NOT NULL DEFAULT '[]',
    active          BOOLEAN NOT NULL DEFAULT true
);

INSERT INTO billing_plans (id, name, tier, price_cents, max_incidents, max_users, max_services, features_json) VALUES
    ('plan_starter', 'Starter', 'starter', 0, 100, 3, 5, '["correlation","dedup","basic_rca","status_page"]'),
    ('plan_growth', 'Growth', 'growth', 50000, 1000, 10, 25, '["correlation","dedup","ai_rca","copilot","slo_tracking","oncall","integrations","postmortem"]'),
    ('plan_scale', 'Scale', 'scale', 150000, -1, -1, -1, '["correlation","dedup","ai_rca","copilot","slo_tracking","oncall","integrations","postmortem","anomaly_detection","roi_dashboard","digest","priority_support"]')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS tenant_subscriptions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL UNIQUE,
    plan_id         TEXT NOT NULL DEFAULT 'plan_starter',
    status          TEXT NOT NULL DEFAULT 'active',
    stripe_customer_id TEXT NOT NULL DEFAULT '',
    stripe_sub_id   TEXT NOT NULL DEFAULT '',
    current_period_start TIMESTAMPTZ,
    current_period_end   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS onboarding_progress (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL UNIQUE,
    step            TEXT NOT NULL DEFAULT 'signup',
    completed_steps TEXT NOT NULL DEFAULT '[]',
    first_source_connected BOOLEAN NOT NULL DEFAULT false,
    first_incident_created BOOLEAN NOT NULL DEFAULT false,
    first_rca_generated    BOOLEAN NOT NULL DEFAULT false,
    aha_moment_reached     BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_subscriptions_tenant ON tenant_subscriptions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_onboarding_progress_tenant ON onboarding_progress (tenant_id);
`,
	},

	// ─── 009: Phase 4 — Correlation merge columns ────────────────────────────
	{
		name: "phase4_correlation",
		sql: `
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS parent_incident_id TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS merged_incident_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS is_merged BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_incidents_parent ON incidents (parent_incident_id);
`,
	},
}
