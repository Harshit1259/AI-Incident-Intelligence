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
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

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
    id                   SERIAL PRIMARY KEY,
    service              TEXT    NOT NULL,
    type                 TEXT    NOT NULL,
    version              TEXT    NOT NULL DEFAULT '',
    description          TEXT    NOT NULL DEFAULT '',
    timestamp            TEXT    NOT NULL,
    tenant_id            TEXT    NOT NULL DEFAULT 'default',
    environment          TEXT    NOT NULL DEFAULT 'production',
    commit_sha           TEXT    NOT NULL DEFAULT '',
    author               TEXT    NOT NULL DEFAULT '',
    pr_number            TEXT    NOT NULL DEFAULT '',
    changed_files_count  INTEGER NOT NULL DEFAULT 0,
    change_source        TEXT    NOT NULL DEFAULT 'webhook',
    metadata_json        TEXT    NOT NULL DEFAULT '{}',
    correlation_score    INTEGER NOT NULL DEFAULT 0,
    linked_incident_id   TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS feature_flag_changes (
    id                  SERIAL PRIMARY KEY,
    tenant_id           TEXT    NOT NULL DEFAULT 'default',
    flag_name           TEXT    NOT NULL,
    flag_key            TEXT    NOT NULL DEFAULT '',
    environment         TEXT    NOT NULL DEFAULT 'production',
    old_value           TEXT    NOT NULL DEFAULT '',
    new_value           TEXT    NOT NULL DEFAULT '',
    changed_by          TEXT    NOT NULL DEFAULT '',
    affected_pct        INTEGER NOT NULL DEFAULT 100,
    timestamp           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    linked_incident_id  TEXT    NOT NULL DEFAULT '',
    correlation_score   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS config_baselines (
    id              SERIAL PRIMARY KEY,
    tenant_id       TEXT    NOT NULL DEFAULT 'default',
    service         TEXT    NOT NULL,
    config_key      TEXT    NOT NULL,
    baseline_value  TEXT    NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, service, config_key)
);

CREATE TABLE IF NOT EXISTS config_drift_events (
    id                  SERIAL PRIMARY KEY,
    tenant_id           TEXT    NOT NULL DEFAULT 'default',
    service             TEXT    NOT NULL,
    config_key          TEXT    NOT NULL,
    baseline_value      TEXT    NOT NULL DEFAULT '',
    current_value       TEXT    NOT NULL DEFAULT '',
    drift_severity      TEXT    NOT NULL DEFAULT 'low',
    detected_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    linked_incident_id  TEXT    NOT NULL DEFAULT ''
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

	// ─── 010: NeuroOps agents ─────────────────────────────────────────────────
	{
		name: "agents",
		sql: `
CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    name            TEXT NOT NULL DEFAULT '',
    host_ip         TEXT NOT NULL DEFAULT '',
    os_type         TEXT NOT NULL DEFAULT '',
    version         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata_json   TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS agent_metrics (
    id              SERIAL PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    metric_type     TEXT NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data_json       TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents (tenant_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents (status);
CREATE INDEX IF NOT EXISTS idx_agent_metrics_agent ON agent_metrics (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_metrics_ts ON agent_metrics (agent_id, timestamp DESC);
`,
	},

	// ─── 011: Gap features — business impact, feedback, auto-resolve, runbooks, deps, whatsapp ─
	{
		name: "gap_features",
		sql: `
CREATE TABLE IF NOT EXISTS business_impact_reports (
    id              TEXT PRIMARY KEY,
    incident_id     TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    executive_summary TEXT NOT NULL DEFAULT '',
    revenue_impact    TEXT NOT NULL DEFAULT '',
    user_impact       TEXT NOT NULL DEFAULT '',
    sla_impact        TEXT NOT NULL DEFAULT '',
    business_risk     TEXT NOT NULL DEFAULT 'low',
    estimated_cost_usd REAL NOT NULL DEFAULT 0,
    generated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_feedback (
    id              SERIAL PRIMARY KEY,
    event_id        TEXT NOT NULL,
    incident_id     TEXT NOT NULL DEFAULT '',
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    feedback        TEXT NOT NULL DEFAULT 'useful',
    fingerprint     TEXT NOT NULL DEFAULT '',
    reason          TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auto_resolve_rules (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    name            TEXT NOT NULL,
    pattern         TEXT NOT NULL DEFAULT '',
    service         TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL DEFAULT '',
    action          TEXT NOT NULL DEFAULT 'resolve',
    cooldown_minutes INTEGER NOT NULL DEFAULT 30,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    times_fired     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS runbooks (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    tags            TEXT NOT NULL DEFAULT '',
    severity_match  TEXT NOT NULL DEFAULT '',
    pattern_match   TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dependency_catalog (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL,
    dependency_name TEXT NOT NULL,
    dependency_type TEXT NOT NULL DEFAULT 'internal',
    vendor          TEXT NOT NULL DEFAULT '',
    health_url      TEXT NOT NULL DEFAULT '',
    status_page_url TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS whatsapp_configs (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    phone_number_id TEXT NOT NULL DEFAULT '',
    access_token    TEXT NOT NULL DEFAULT '',
    verify_token    TEXT NOT NULL DEFAULT '',
    notify_on       TEXT NOT NULL DEFAULT 'critical',
    recipients_json TEXT NOT NULL DEFAULT '[]',
    enabled         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_biz_impact_incident ON business_impact_reports (incident_id);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_fingerprint ON alert_feedback (fingerprint);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_tenant ON alert_feedback (tenant_id);
CREATE INDEX IF NOT EXISTS idx_auto_resolve_rules_tenant ON auto_resolve_rules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_runbooks_service ON runbooks (service);
CREATE INDEX IF NOT EXISTS idx_runbooks_tenant ON runbooks (tenant_id);
CREATE INDEX IF NOT EXISTS idx_dependency_catalog_service ON dependency_catalog (service);
CREATE INDEX IF NOT EXISTS idx_whatsapp_configs_tenant ON whatsapp_configs (tenant_id);
`,
	},

	// ─── Alert Quality Governance ─────────────────────────────────────────────
	{
		name: "alert_quality",
		sql: `
CREATE TABLE IF NOT EXISTS alert_rules (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    name            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',
    service         TEXT NOT NULL DEFAULT '',
    team            TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL DEFAULT '',
    condition_expr  TEXT NOT NULL DEFAULT '',
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_fired_at   TIMESTAMPTZ,
    fire_count_7d   INTEGER NOT NULL DEFAULT 0,
    fire_count_30d  INTEGER NOT NULL DEFAULT 0,
    fp_count        INTEGER NOT NULL DEFAULT 0,
    noise_count     INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant    ON alert_rules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_service   ON alert_rules (tenant_id, service);
CREATE INDEX IF NOT EXISTS idx_alert_rules_last_fired ON alert_rules (tenant_id, last_fired_at);
`,
	},

	// ─── 012: Log Explorer ──────────────────────────────────────────────────
	{
		name: "log_explorer",
		sql: `
CREATE TABLE IF NOT EXISTS log_entries (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    agent_id        TEXT NOT NULL DEFAULT '',
    host_ip         TEXT NOT NULL DEFAULT '',
    log_source      TEXT NOT NULL DEFAULT '',
    log_tag         TEXT NOT NULL DEFAULT '',
    event_type      TEXT NOT NULL DEFAULT 'log',
    event_category  TEXT NOT NULL DEFAULT 'info',
    message         TEXT NOT NULL DEFAULT '',
    raw_json        TEXT NOT NULL DEFAULT '{}',
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_log_entries_tenant_ts ON log_entries (tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_entries_agent ON log_entries (agent_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_entries_host ON log_entries (host_ip, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_entries_category ON log_entries (event_category, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_entries_tag ON log_entries (log_tag);
`,
	},

	// ─── 013: Business Impact Engine ────────────────────────────────────────
	{
		name: "business_impact_engine",
		sql: `
DROP TABLE IF EXISTS business_impact_reports;

CREATE TABLE IF NOT EXISTS biz_service_profiles (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL,
    tier            TEXT NOT NULL DEFAULT 'TIER_2',
    cost_model_type TEXT NOT NULL DEFAULT 'mixed',
    hourly_revenue         REAL NOT NULL DEFAULT 0,
    transactions_per_hour  REAL NOT NULL DEFAULT 0,
    avg_order_value        REAL NOT NULL DEFAULT 0,
    users_per_hour         REAL NOT NULL DEFAULT 0,
    employee_cost_per_hour REAL NOT NULL DEFAULT 25,
    sla_penalty_per_minute REAL NOT NULL DEFAULT 0,
    sla_threshold_minutes  INTEGER NOT NULL DEFAULT 15,
    business_hour_multiplier REAL NOT NULL DEFAULT 1.0,
    peak_multiplier        REAL NOT NULL DEFAULT 1.0,
    infra_cost_per_hour    REAL NOT NULL DEFAULT 0,
    confidence_mode        TEXT NOT NULL DEFAULT 'balanced',
    currency               TEXT NOT NULL DEFAULT 'USD',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS biz_baselines (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    service         TEXT NOT NULL DEFAULT '',
    incident_type   TEXT NOT NULL DEFAULT '',
    avg_mttr_minutes    REAL NOT NULL DEFAULT 60,
    median_mttr_minutes REAL NOT NULL DEFAULT 45,
    p90_mttr_minutes    REAL NOT NULL DEFAULT 90,
    sample_size     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS biz_financial_estimates (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    incident_id     TEXT NOT NULL,
    service         TEXT NOT NULL DEFAULT '',
    actual_loss          REAL NOT NULL DEFAULT 0,
    counterfactual_loss  REAL NOT NULL DEFAULT 0,
    avoided_loss         REAL NOT NULL DEFAULT 0,
    confidence_level     TEXT NOT NULL DEFAULT 'LOW',
    confidence_score     REAL NOT NULL DEFAULT 0.4,
    method_used          TEXT NOT NULL DEFAULT 'static_fallback',
    currency             TEXT NOT NULL DEFAULT 'USD',
    breakdown_json       TEXT NOT NULL DEFAULT '{}',
    explanation          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS biz_monthly_rollups (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    year            INTEGER NOT NULL,
    month           INTEGER NOT NULL,
    total_actual_loss         REAL NOT NULL DEFAULT 0,
    total_counterfactual_loss REAL NOT NULL DEFAULT 0,
    total_avoided_loss        REAL NOT NULL DEFAULT 0,
    confidence_weighted_loss  REAL NOT NULL DEFAULT 0,
    currency                  TEXT NOT NULL DEFAULT 'USD',
    top_incidents_json        TEXT NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_biz_profiles_service ON biz_service_profiles (service);
CREATE INDEX IF NOT EXISTS idx_biz_profiles_tenant ON biz_service_profiles (tenant_id);
CREATE INDEX IF NOT EXISTS idx_biz_baselines_service ON biz_baselines (service, incident_type);
CREATE INDEX IF NOT EXISTS idx_biz_estimates_incident ON biz_financial_estimates (incident_id);
CREATE INDEX IF NOT EXISTS idx_biz_estimates_tenant ON biz_financial_estimates (tenant_id);
CREATE INDEX IF NOT EXISTS idx_biz_rollups_tenant ON biz_monthly_rollups (tenant_id, year, month);
`,
	},

	// ─── Priority score column ──────────────────────────────────────────
	{
		name: "priority_score",
		sql: `
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS priority_score INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_incidents_priority ON incidents (priority_score DESC);
`,
	},

	// ─── Source Registry ────────────────────────────────────────────────
	{
		name: "source_registry",
		sql: `
CREATE TABLE IF NOT EXISTS source_registry (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL DEFAULT 'default',
    name        TEXT NOT NULL,
    source_type TEXT NOT NULL DEFAULT 'webhook',
    endpoint    TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',
    last_event  TIMESTAMPTZ,
    event_count INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_source_registry_tenant ON source_registry (tenant_id);
`,
	},

	// ─── Tenant configuration layer ─────────────────────────────────────
	{
		name: "tenant_config_layer",
		sql: `
-- Extend tenants table with business context columns.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS deployment_mode  TEXT NOT NULL DEFAULT 'cloud';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS industry_type    TEXT NOT NULL DEFAULT 'general';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS default_currency TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS timezone         TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS estimation_mode  TEXT NOT NULL DEFAULT 'balanced';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Service catalog: per-tenant registry of all services.
CREATE TABLE IF NOT EXISTS service_catalog (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL DEFAULT 'default',
    service_name      TEXT NOT NULL,
    environment       TEXT NOT NULL DEFAULT 'prod',
    business_unit     TEXT NOT NULL DEFAULT '',
    owner             TEXT NOT NULL DEFAULT '',
    is_customer_facing BOOLEAN NOT NULL DEFAULT false,
    tier              TEXT NOT NULL DEFAULT 'TIER_2',
    region            TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_service_catalog_tenant      ON service_catalog (tenant_id);
CREATE INDEX IF NOT EXISTS idx_service_catalog_name        ON service_catalog (service_name);
CREATE INDEX IF NOT EXISTS idx_service_catalog_tenant_name ON service_catalog (tenant_id, service_name);

-- Tenant settings: per-tenant multiplier and preference overrides.
CREATE TABLE IF NOT EXISTS tenant_settings (
    id                          TEXT PRIMARY KEY,
    tenant_id                   TEXT NOT NULL UNIQUE,
    severity_multipliers_json   TEXT NOT NULL DEFAULT '{}',
    tier_multipliers_json       TEXT NOT NULL DEFAULT '{}',
    confidence_mode             TEXT NOT NULL DEFAULT 'balanced',
    monthly_report_prefs_json   TEXT NOT NULL DEFAULT '{}',
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_settings_tenant ON tenant_settings (tenant_id);
`,
	},

	// ─── Enterprise features ────────────────────────────────────────────
	{
		name: "enterprise_features",
		sql: `
CREATE TABLE IF NOT EXISTS execution_policies (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    name            TEXT NOT NULL,
    service         TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL DEFAULT '',
    execution_mode  TEXT NOT NULL DEFAULT 'manual',
    max_retries     INTEGER NOT NULL DEFAULT 2,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    timeout_seconds INTEGER NOT NULL DEFAULT 90,
    max_per_hour    INTEGER NOT NULL DEFAULT 10,
    blackout_start  TEXT NOT NULL DEFAULT '',
    blackout_end    TEXT NOT NULL DEFAULT '',
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS action_executions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    incident_id     TEXT NOT NULL,
    action_id       TEXT NOT NULL,
    action_label    TEXT NOT NULL DEFAULT '',
    action_desc     TEXT NOT NULL DEFAULT '',
    chain_position  INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'planned',
    execution_mode  TEXT NOT NULL DEFAULT 'manual',
    result_output   TEXT NOT NULL DEFAULT '',
    error_message   TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    verified        BOOLEAN NOT NULL DEFAULT false,
    verification_result TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS verification_records (
    id                    TEXT PRIMARY KEY,
    tenant_id             TEXT    NOT NULL DEFAULT 'default',
    incident_id           TEXT    NOT NULL,
    execution_id          TEXT    NOT NULL DEFAULT '',
    strategy              TEXT    NOT NULL DEFAULT 'metric_check',
    status                TEXT    NOT NULL DEFAULT 'pending',
    checks_json           TEXT    NOT NULL DEFAULT '[]',
    result                TEXT    NOT NULL DEFAULT 'unknown',
    before_snapshot_json  TEXT    NOT NULL DEFAULT '{}',
    after_snapshot_json   TEXT    NOT NULL DEFAULT '{}',
    confidence_before     INTEGER NOT NULL DEFAULT 0,
    confidence_after      INTEGER NOT NULL DEFAULT 0,
    proof_items_json      TEXT    NOT NULL DEFAULT '[]',
    rollback_triggered    BOOLEAN NOT NULL DEFAULT FALSE,
    auto_close_eligible   BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenant_configs (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL UNIQUE,
    settings_json   TEXT NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS platform_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    actor           TEXT NOT NULL DEFAULT 'system',
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL DEFAULT '',
    resource_id     TEXT NOT NULL DEFAULT '',
    details_json    TEXT NOT NULL DEFAULT '{}',
    ip_address      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'operator';

-- Phase 3: incident memory — resolution history per fingerprint/service
CREATE TABLE IF NOT EXISTS incident_resolutions (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT 'default',
    fingerprint     TEXT NOT NULL DEFAULT '',
    service         TEXT NOT NULL DEFAULT '',
    incident_id     TEXT NOT NULL UNIQUE,
    ttr_seconds     INTEGER NOT NULL DEFAULT 0,
    actions_taken   TEXT NOT NULL DEFAULT '[]',
    resolution_note TEXT NOT NULL DEFAULT '',
    resolved_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_incident_resolutions_fingerprint ON incident_resolutions (tenant_id, fingerprint);
CREATE INDEX IF NOT EXISTS idx_incident_resolutions_service ON incident_resolutions (tenant_id, service);

-- Phase 2: approval workflow and rollback tracking on action executions
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS requires_approval  BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS approval_status    TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS approved_by        TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS approved_at        TIMESTAMPTZ;
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS rejected_by        TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS rejection_reason   TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS rollback_ref       TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS rollback_reason    TEXT    NOT NULL DEFAULT '';
ALTER TABLE action_executions ADD COLUMN IF NOT EXISTS rolled_back_by     TEXT    NOT NULL DEFAULT '';

-- Phase 2: change linkage confidence score
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_confidence INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_exec_policies_tenant ON execution_policies (tenant_id);
CREATE INDEX IF NOT EXISTS idx_action_executions_incident ON action_executions (incident_id);
CREATE INDEX IF NOT EXISTS idx_action_executions_status ON action_executions (status);
CREATE INDEX IF NOT EXISTS idx_verification_records_incident ON verification_records (incident_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_tenant ON platform_audit_log (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tenant_configs_tenant ON tenant_configs (tenant_id);
`,
	},

	// ─── Tenant isolation hardening ─────────────────────────────────────
	{
		name: "tenant_isolation_hardening",
		sql: `
-- Events: add tenant_id so every event is scoped to an owning tenant.
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
CREATE INDEX IF NOT EXISTS idx_events_tenant_id ON events (tenant_id);

-- Source Registry: add ingest_token for public webhook tenant resolution.
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS ingest_token TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_registry_ingest_token
    ON source_registry (ingest_token) WHERE ingest_token != '';

-- Backfill existing events to the default tenant.
UPDATE events SET tenant_id = 'default' WHERE tenant_id = '' OR tenant_id IS NULL;
`,
	},

	// ─── Typed timestamps: convert TEXT to TIMESTAMPTZ ───────────────────
	// incidents.first_event_time and last_event_time were stored as RFC3339
	// text. Converting to TIMESTAMPTZ enables real date arithmetic, correct
	// time-zone-aware sorting, and proper index behavior.
	//
	// what_changed_timestamp and last_seen_at become nullable TIMESTAMPTZ
	// (empty string → NULL) so Go can use *time.Time with nil == "not set".
	//
	// changes.timestamp had the same TEXT problem; fixed here too.
	//
	// Safe to re-run: ALTER COLUMN with an identical USING clause is
	// idempotent on an already-typed column in PostgreSQL.
	{
		name: "typed_timestamps",
		sql: `
-- incidents: required timestamps (NOT NULL)
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'incidents'
      AND column_name = 'first_event_time'
      AND data_type   = 'text'
  ) THEN
    -- The TEXT column's '' default cannot be cast to TIMESTAMPTZ, which made
    -- this migration fail on every fresh database. Drop it first.
    ALTER TABLE incidents ALTER COLUMN first_event_time DROP DEFAULT;
    ALTER TABLE incidents
      ALTER COLUMN first_event_time TYPE TIMESTAMPTZ
        USING CASE WHEN first_event_time = '' THEN NOW()
                   ELSE first_event_time::TIMESTAMPTZ END,
      ALTER COLUMN first_event_time SET DEFAULT NOW();

    -- The TEXT column's '' default cannot be cast to TIMESTAMPTZ, which made
    -- this migration fail on every fresh database. Drop it first.
    ALTER TABLE incidents ALTER COLUMN last_event_time DROP DEFAULT;
    ALTER TABLE incidents
      ALTER COLUMN last_event_time TYPE TIMESTAMPTZ
        USING CASE WHEN last_event_time = '' THEN NOW()
                   ELSE last_event_time::TIMESTAMPTZ END,
      ALTER COLUMN last_event_time SET DEFAULT NOW();
  END IF;
END$$;

-- incidents: optional timestamps (nullable)
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'incidents'
      AND column_name = 'what_changed_timestamp'
      AND data_type   = 'text'
  ) THEN
    ALTER TABLE incidents
      ALTER COLUMN what_changed_timestamp DROP NOT NULL,
      ALTER COLUMN what_changed_timestamp DROP DEFAULT;
    ALTER TABLE incidents
      ALTER COLUMN what_changed_timestamp TYPE TIMESTAMPTZ
        USING NULLIF(what_changed_timestamp, '')::TIMESTAMPTZ;

    ALTER TABLE incidents
      ALTER COLUMN last_seen_at DROP NOT NULL,
      ALTER COLUMN last_seen_at DROP DEFAULT;
    ALTER TABLE incidents
      ALTER COLUMN last_seen_at TYPE TIMESTAMPTZ
        USING NULLIF(last_seen_at, '')::TIMESTAMPTZ;
  END IF;
END$$;

-- changes: timestamp column (NOT NULL)
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'changes'
      AND column_name = 'timestamp'
      AND data_type   = 'text'
  ) THEN
    ALTER TABLE changes
      ALTER COLUMN timestamp TYPE TIMESTAMPTZ
        USING timestamp::TIMESTAMPTZ;
  END IF;
END$$;

-- Drop the text-based indexes on first/last_event_time and recreate them
-- so the planner can use them for temporal range scans.
DROP INDEX IF EXISTS idx_incidents_last_event_time;
DROP INDEX IF EXISTS idx_incidents_service_status_ts;
DROP INDEX IF EXISTS idx_incidents_status_severity_ts;
CREATE INDEX IF NOT EXISTS idx_incidents_last_event_time    ON incidents (last_event_time DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_service_status_ts  ON incidents (service, status, last_event_time DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_status_severity_ts ON incidents (status, severity, last_event_time DESC);
`,
	},

	// ─── Agent cryptographic identity ────────────────────────────────────
	{
		name: "agent_cryptographic_identity",
		sql: `
-- Per-agent AES-256-GCM encrypted HMAC signing secret.
-- Never stored in plaintext; decrypted server-side only during request verification.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS secret_enc      TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS enrolled_via    TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS last_rotated_at TIMESTAMPTZ;

-- Single-use bootstrap tokens that gate agent registration.
-- token_hash = hex(SHA-256(raw_token)); the raw token is returned once on creation.
CREATE TABLE IF NOT EXISTS agent_enrollment_tokens (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL DEFAULT 'default',
    label            TEXT NOT NULL DEFAULT '',
    token_hash       TEXT NOT NULL,
    used_at          TIMESTAMPTZ,
    used_by_agent_id TEXT NOT NULL DEFAULT '',
    expires_at       TIMESTAMPTZ NOT NULL,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_tenant ON agent_enrollment_tokens (tenant_id);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_hash   ON agent_enrollment_tokens (token_hash);
`,
	},

	// ─── Webhook hardening ────────────────────────────────────────────────
	// Adds infrastructure for rate limiting, idempotency, dead-letter queuing,
	// and richer integration health telemetry.
	{
		name: "webhook_hardening",
		sql: `
-- Richer health columns for source_registry.
-- last_error stores the most recent error message; last_error_at stores when it occurred;
-- error_count is a rolling counter reset on RecordSuccess.
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS last_error    TEXT    NOT NULL DEFAULT '';
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMPTZ;
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS error_count   INTEGER NOT NULL DEFAULT 0;

-- Idempotency key table.
-- Stores X-Idempotency-Key values per tenant for 24 hours.
-- If the same key arrives twice within the TTL the cached response is returned.
CREATE TABLE IF NOT EXISTS webhook_idempotency_keys (
    key        TEXT        NOT NULL,
    tenant_id  TEXT        NOT NULL DEFAULT 'default',
    source_id  TEXT        NOT NULL DEFAULT '',
    response   TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (key, tenant_id)
);
CREATE INDEX IF NOT EXISTS idx_idempotency_expires ON webhook_idempotency_keys (expires_at);

-- Dead-letter queue.
-- Payloads that passed auth/schema validation but failed during event processing
-- are written here instead of being silently dropped.
CREATE TABLE IF NOT EXISTS webhook_dead_letters (
    id            BIGSERIAL   PRIMARY KEY,
    tenant_id     TEXT        NOT NULL DEFAULT 'default',
    source_id     TEXT        NOT NULL DEFAULT '',
    source_type   TEXT        NOT NULL DEFAULT '',
    endpoint      TEXT        NOT NULL DEFAULT '',
    raw_payload   TEXT        NOT NULL DEFAULT '',
    error         TEXT        NOT NULL DEFAULT '',
    retry_count   INTEGER     NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_retry_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dlq_tenant ON webhook_dead_letters (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlq_source ON webhook_dead_letters (source_id);
`,
	},

	// ─── Enterprise Automation Policy Engine ─────────────────────────────
	// Adds environment-aware matching, priority, business-hours enforcement,
	// blast-radius controls, circuit-breaker, retry strategy, simulation/dry-run
	// modes, rollback hooks, approval workflow extensions, immutable execution
	// trail, and per-policy circuit-breaker state tracking.
	{
		name: "automation_policy_engine",
		sql: `
-- Extend execution_policies with all enterprise-grade controls.
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS environment                   TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS priority                      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS business_hours_start          TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS business_hours_end            TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS business_hours_timezone       TEXT    NOT NULL DEFAULT 'UTC';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS enforce_business_hours        BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS max_concurrent_actions        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS max_impacted_services         INTEGER NOT NULL DEFAULT 0;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS circuit_breaker_enabled       BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS circuit_breaker_threshold     INTEGER NOT NULL DEFAULT 5;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS circuit_breaker_window_seconds INTEGER NOT NULL DEFAULT 3600;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS circuit_breaker_cooldown_minutes INTEGER NOT NULL DEFAULT 30;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS retry_backoff_seconds         INTEGER NOT NULL DEFAULT 30;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS retry_strategy                TEXT    NOT NULL DEFAULT 'exponential';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS simulation_mode               BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS dry_run_mode                  BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS rollback_webhook_url          TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS rollback_script_ref           TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS approval_groups               TEXT    NOT NULL DEFAULT '';
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS approval_timeout_minutes      INTEGER NOT NULL DEFAULT 60;
ALTER TABLE execution_policies ADD COLUMN IF NOT EXISTS updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Immutable policy execution trail.
-- Every policy evaluation writes one row here. Rows must never be updated or deleted.
CREATE TABLE IF NOT EXISTS policy_execution_trail (
    id             TEXT        PRIMARY KEY,
    tenant_id      TEXT        NOT NULL DEFAULT 'default',
    policy_id      TEXT        NOT NULL DEFAULT '',
    policy_name    TEXT        NOT NULL DEFAULT '',
    incident_id    TEXT        NOT NULL DEFAULT '',
    action_id      TEXT        NOT NULL DEFAULT '',
    service        TEXT        NOT NULL DEFAULT '',
    environment    TEXT        NOT NULL DEFAULT '',
    severity       TEXT        NOT NULL DEFAULT '',
    decision       TEXT        NOT NULL DEFAULT 'allowed',
    denial_code    TEXT        NOT NULL DEFAULT '',
    execution_mode TEXT        NOT NULL DEFAULT '',
    reason         TEXT        NOT NULL DEFAULT '',
    is_simulation  BOOLEAN     NOT NULL DEFAULT false,
    is_dry_run     BOOLEAN     NOT NULL DEFAULT false,
    actor          TEXT        NOT NULL DEFAULT 'system',
    context_json   TEXT        NOT NULL DEFAULT '{}',
    evaluated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_policy_trail_tenant   ON policy_execution_trail (tenant_id, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_policy_trail_policy   ON policy_execution_trail (policy_id, evaluated_at DESC);
CREATE INDEX IF NOT EXISTS idx_policy_trail_incident ON policy_execution_trail (incident_id);
CREATE INDEX IF NOT EXISTS idx_policy_trail_decision ON policy_execution_trail (tenant_id, decision, evaluated_at DESC);

-- Per-policy circuit-breaker state.
CREATE TABLE IF NOT EXISTS policy_circuit_breaker_states (
    policy_id            TEXT        PRIMARY KEY,
    tenant_id            TEXT        NOT NULL DEFAULT 'default',
    is_open              BOOLEAN     NOT NULL DEFAULT false,
    consecutive_failures INTEGER     NOT NULL DEFAULT 0,
    last_failure_at      TIMESTAMPTZ,
    opened_at            TIMESTAMPTZ,
    will_reset_at        TIMESTAMPTZ,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_circuit_breaker_tenant ON policy_circuit_breaker_states (tenant_id);
`,
	},
	// otel_native_data_model — Feature 7
	// Adds OTel-native fields to events, creates schema_mappings table for custom sources.
	{
		name: "otel_native_data_model",
		sql: `
-- OTel-native fields on events table.
ALTER TABLE events ADD COLUMN IF NOT EXISTS trace_id      TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS span_id       TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS ingest_schema TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS attrs_json    TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id     TEXT NOT NULL DEFAULT 'default';
ALTER TABLE events ADD COLUMN IF NOT EXISTS external_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS resource      TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS environment   TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS fingerprint   TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS title         TEXT NOT NULL DEFAULT '';

-- Indexes for OTel trace/span correlation.
CREATE INDEX IF NOT EXISTS idx_events_trace_id      ON events (trace_id) WHERE trace_id <> '';
CREATE INDEX IF NOT EXISTS idx_events_ingest_schema ON events (tenant_id, ingest_schema, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_tenant_ts     ON events (tenant_id, timestamp DESC);

-- Custom source schema registry.
CREATE TABLE IF NOT EXISTS schema_mappings (
    id                   TEXT        PRIMARY KEY,
    tenant_id            TEXT        NOT NULL DEFAULT 'default',
    source_type          TEXT        NOT NULL,
    name                 TEXT        NOT NULL DEFAULT '',
    enabled              BOOLEAN     NOT NULL DEFAULT true,
    field_map            TEXT        NOT NULL DEFAULT '{}',
    severity_map         TEXT        NOT NULL DEFAULT '{}',
    default_signal_type  TEXT        NOT NULL DEFAULT 'alert',
    default_severity     TEXT        NOT NULL DEFAULT 'medium',
    title_template       TEXT        NOT NULL DEFAULT '',
    sample_payload       TEXT        NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_type)
);
CREATE INDEX IF NOT EXISTS idx_schema_mappings_tenant ON schema_mappings (tenant_id, enabled);
`,
	},
	// team_workflow_primitives — Feature 8
	// Incident commander, stakeholder templates, ticket sync, status communications.
	{
		name: "team_workflow_primitives",
		sql: `
-- Incident commander role assignment.
CREATE TABLE IF NOT EXISTS incident_commanders (
    id           TEXT        PRIMARY KEY,
    tenant_id    TEXT        NOT NULL DEFAULT 'default',
    incident_id  TEXT        NOT NULL,
    user_id      TEXT        NOT NULL DEFAULT '',
    user_name    TEXT        NOT NULL DEFAULT '',
    user_email   TEXT        NOT NULL DEFAULT '',
    assigned_by  TEXT        NOT NULL DEFAULT '',
    notes        TEXT        NOT NULL DEFAULT '',
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    relieved_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_commanders_incident ON incident_commanders (tenant_id, incident_id, relieved_at);

-- Stakeholder comms templates with variable substitution.
CREATE TABLE IF NOT EXISTS stakeholder_templates (
    id         TEXT        PRIMARY KEY,
    tenant_id  TEXT        NOT NULL DEFAULT 'default',
    name       TEXT        NOT NULL,
    channel    TEXT        NOT NULL DEFAULT 'slack',
    subject    TEXT        NOT NULL DEFAULT '',
    body       TEXT        NOT NULL DEFAULT '',
    is_default BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_templates_tenant ON stakeholder_templates (tenant_id, channel);

-- External ticket links (JIRA, ServiceNow).
CREATE TABLE IF NOT EXISTS ticket_syncs (
    id            TEXT        PRIMARY KEY,
    tenant_id     TEXT        NOT NULL DEFAULT 'default',
    incident_id   TEXT        NOT NULL,
    provider      TEXT        NOT NULL,
    ticket_key    TEXT        NOT NULL DEFAULT '',
    ticket_url    TEXT        NOT NULL DEFAULT '',
    ticket_status TEXT        NOT NULL DEFAULT '',
    created_by    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (incident_id, provider)
);
CREATE INDEX IF NOT EXISTS idx_ticket_syncs_incident ON ticket_syncs (tenant_id, incident_id);

-- Customer-facing status communications.
CREATE TABLE IF NOT EXISTS status_communications (
    id                TEXT        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL DEFAULT 'default',
    incident_id       TEXT        NOT NULL,
    stage             TEXT        NOT NULL DEFAULT 'investigating',
    title             TEXT        NOT NULL DEFAULT '',
    body              TEXT        NOT NULL DEFAULT '',
    affected_services TEXT        NOT NULL DEFAULT '[]',
    published_by      TEXT        NOT NULL DEFAULT '',
    published_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_status_comms_incident ON status_communications (tenant_id, incident_id, published_at DESC);
`,
	},
	// ── Feature B1: SaaS multi-tenancy ───────────────────────────────────────────
	{
		name: "saas_multitenant",
		sql: `
-- Extend tenants with SaaS plan, lifecycle state, and per-tenant limits.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS plan               TEXT    NOT NULL DEFAULT 'trial';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS state              TEXT    NOT NULL DEFAULT 'active';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS owner_email        TEXT    NOT NULL DEFAULT '';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS suspended_at       TIMESTAMPTZ;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS disabled_at        TIMESTAMPTZ;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS ingest_rate_min    INTEGER;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS query_rate_min     INTEGER;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS max_incidents      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS max_events         INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS encryption_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS audit_retain_days  INTEGER NOT NULL DEFAULT 90;

-- Rate-limit event log: tracks when a tenant was throttled and by which category.
CREATE TABLE IF NOT EXISTS tenant_rate_limit_events (
    id         BIGSERIAL    PRIMARY KEY,
    tenant_id  TEXT         NOT NULL,
    category   TEXT         NOT NULL DEFAULT 'ingest',
    limited_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    count      INTEGER      NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_rate_limit_events ON tenant_rate_limit_events (tenant_id, limited_at DESC);
`,
	},
	// ── Feature 9: AI domain memory ──────────────────────────────────────────────
	{
		name: "incident_domain_memory",
		sql: `
-- Remediation patterns: success-tracked, reusable fixes keyed by service+error_signature.
CREATE TABLE IF NOT EXISTS remediation_patterns (
    id                   TEXT        PRIMARY KEY,
    tenant_id            TEXT        NOT NULL DEFAULT 'default',
    service              TEXT        NOT NULL DEFAULT '',
    error_signature      TEXT        NOT NULL DEFAULT '',
    root_cause_category  TEXT        NOT NULL DEFAULT '',
    remediation_summary  TEXT        NOT NULL DEFAULT '',
    remediation_steps    TEXT        NOT NULL DEFAULT '[]',
    success_count        INTEGER     NOT NULL DEFAULT 1,
    failure_count        INTEGER     NOT NULL DEFAULT 0,
    avg_resolution_mins  REAL        NOT NULL DEFAULT 0,
    last_used_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_remediation_patterns_service ON remediation_patterns (tenant_id, service);

-- Deploy signatures: known-bad deploy patterns that historically triggered incidents.
CREATE TABLE IF NOT EXISTS deploy_signatures (
    id                TEXT        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL DEFAULT 'default',
    service           TEXT        NOT NULL DEFAULT '',
    signature_name    TEXT        NOT NULL DEFAULT '',
    description       TEXT        NOT NULL DEFAULT '',
    indicators        TEXT        NOT NULL DEFAULT '[]',
    impacted_services TEXT        NOT NULL DEFAULT '[]',
    typical_severity  TEXT        NOT NULL DEFAULT 'medium',
    occurrence_count  INTEGER     NOT NULL DEFAULT 1,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_deploy_signatures_service ON deploy_signatures (tenant_id, service);

-- Runbook preferences: team-specific workflow preferences per service.
CREATE TABLE IF NOT EXISTS runbook_preferences (
    id               TEXT        PRIMARY KEY,
    tenant_id        TEXT        NOT NULL DEFAULT 'default',
    team_name        TEXT        NOT NULL DEFAULT '',
    service          TEXT        NOT NULL DEFAULT '',
    preference_key   TEXT        NOT NULL DEFAULT '',
    preference_value TEXT        NOT NULL DEFAULT '',
    context          TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, team_name, service, preference_key)
);
CREATE INDEX IF NOT EXISTS idx_runbook_prefs_team_service ON runbook_preferences (tenant_id, team_name, service);
`,
	},

	// ── Multi-tenancy hardening: per-table isolation gaps ────────────────────────
	{
		name: "multitenant_isolation_hardening",
		sql: `
-- incident_status_history: add tenant_id so history records are always
-- scoped to their owning tenant. Prevents cross-tenant reads when incident
-- IDs are guessable or re-used across tenants.
ALTER TABLE incident_status_history ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
CREATE INDEX IF NOT EXISTS idx_incident_status_history_tenant ON incident_status_history (tenant_id, incident_id);

-- Backfill existing rows to default tenant.
UPDATE incident_status_history SET tenant_id = 'default' WHERE tenant_id = '' OR tenant_id IS NULL;

-- source_registry: add token_hash column for encrypted-token lookup.
-- Stores SHA-256(ingest_token) so we can lookup by hash when the stored
-- token is encrypted (field-level encryption via AES-256-GCM).
-- Falls back to plaintext lookup for pre-existing rows where hash is empty.
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS ingest_token_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_source_registry_token_hash
    ON source_registry (ingest_token_hash) WHERE ingest_token_hash <> '';

-- action_audit: add tenant_id for per-tenant scoping, then index.
ALTER TABLE action_audit ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
CREATE INDEX IF NOT EXISTS idx_action_audit_tenant_incident
    ON action_audit (tenant_id, incident_id, executed_at DESC);
`,
	},

	// ── Identity & Enterprise Access ──────────────────────────────────────────
	{
		name: "identity_enterprise_access",
		sql: `
-- Extend users table with SSO + SCIM fields.
ALTER TABLE users ADD COLUMN IF NOT EXISTS sso_user_id       TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS sso_provider_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name      TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS scim_external_id  TEXT NOT NULL DEFAULT '';

-- SSO provider config (OIDC + SAML) per tenant.
CREATE TABLE IF NOT EXISTS sso_providers (
    id                    TEXT PRIMARY KEY,
    tenant_id             TEXT NOT NULL,
    provider_type         TEXT NOT NULL DEFAULT 'oidc',
    name                  TEXT NOT NULL,
    enabled               BOOLEAN NOT NULL DEFAULT true,
    client_id             TEXT NOT NULL DEFAULT '',
    client_secret_enc     TEXT NOT NULL DEFAULT '',
    discovery_url         TEXT NOT NULL DEFAULT '',
    scopes                TEXT NOT NULL DEFAULT 'openid email profile',
    idp_entity_id         TEXT NOT NULL DEFAULT '',
    idp_sso_url           TEXT NOT NULL DEFAULT '',
    idp_cert_enc          TEXT NOT NULL DEFAULT '',
    sp_entity_id          TEXT NOT NULL DEFAULT '',
    acs_url               TEXT NOT NULL DEFAULT '',
    attribute_mapping_json TEXT NOT NULL DEFAULT '{}',
    auto_provision        BOOLEAN NOT NULL DEFAULT true,
    default_role          TEXT NOT NULL DEFAULT 'viewer',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sso_providers_tenant ON sso_providers (tenant_id);

-- Short-lived OIDC PKCE state (TTL ~5 min).
CREATE TABLE IF NOT EXISTS sso_state (
    state         TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    provider_id   TEXT NOT NULL,
    code_verifier TEXT NOT NULL DEFAULT '',
    redirect_to   TEXT NOT NULL DEFAULT '',
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Verified email domains → tenant routing.
CREATE TABLE IF NOT EXISTS domain_verifications (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    domain             TEXT NOT NULL UNIQUE,
    verified           BOOLEAN NOT NULL DEFAULT false,
    verification_token TEXT NOT NULL DEFAULT '',
    dns_txt_record     TEXT NOT NULL DEFAULT '',
    verified_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_domain_verifications_tenant ON domain_verifications (tenant_id);

-- Projects within a tenant/org.
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_projects_tenant ON projects (tenant_id);

-- Workspaces within projects.
CREATE TABLE IF NOT EXISTS workspaces (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_workspaces_project ON workspaces (tenant_id, project_id);

-- Custom roles (tenant-defined, beyond admin/operator/viewer).
CREATE TABLE IF NOT EXISTS custom_roles (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    is_system        BOOLEAN NOT NULL DEFAULT false,
    permissions_json TEXT NOT NULL DEFAULT '[]',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_custom_roles_tenant ON custom_roles (tenant_id);

-- Role assignments: user → role at org/project/workspace scope.
CREATE TABLE IF NOT EXISTS role_assignments (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    role_id     TEXT NOT NULL,
    scope_type  TEXT NOT NULL DEFAULT 'org',
    scope_id    TEXT NOT NULL DEFAULT '',
    assigned_by TEXT NOT NULL DEFAULT '',
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_role_assignments_user ON role_assignments (tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_role_assignments_role ON role_assignments (tenant_id, role_id);

-- Attribute policies for ABAC.
CREATE TABLE IF NOT EXISTS attribute_policies (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    role_id        TEXT NOT NULL,
    resource_type  TEXT NOT NULL,
    action         TEXT NOT NULL,
    conditions_json TEXT NOT NULL DEFAULT '{}',
    effect         TEXT NOT NULL DEFAULT 'allow',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_attribute_policies_tenant ON attribute_policies (tenant_id, role_id);

-- Service accounts (machine identities).
CREATE TABLE IF NOT EXISTS service_accounts (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'operator',
    custom_role_id TEXT NOT NULL DEFAULT '',
    enabled        BOOLEAN NOT NULL DEFAULT true,
    created_by     TEXT NOT NULL DEFAULT '',
    last_used_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_service_accounts_tenant ON service_accounts (tenant_id);

-- API keys (for both users and service accounts).
CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL UNIQUE,
    key_prefix  TEXT NOT NULL DEFAULT '',
    owner_type  TEXT NOT NULL DEFAULT 'user',
    owner_id    TEXT NOT NULL,
    scopes_json TEXT NOT NULL DEFAULT '[]',
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    revoked     BOOLEAN NOT NULL DEFAULT false,
    revoked_at  TIMESTAMPTZ,
    revoked_by  TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys (tenant_id, owner_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash   ON api_keys (key_hash);

-- SCIM groups (mapped from IdP groups/roles).
CREATE TABLE IF NOT EXISTS scim_groups (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    display_name TEXT NOT NULL,
    external_id  TEXT NOT NULL DEFAULT '',
    members_json TEXT NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scim_groups_tenant ON scim_groups (tenant_id);

-- SCIM sync log for auditing provisioning operations.
CREATE TABLE IF NOT EXISTS scim_sync_log (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    operation   TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'success',
    details     TEXT NOT NULL DEFAULT '',
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scim_sync_log_tenant ON scim_sync_log (tenant_id, synced_at DESC);
`,
	},

	// ── Topology & Dependency Intelligence ───────────────────────────────────────
	{
		name: "topology_intelligence",
		sql: `
CREATE TABLE IF NOT EXISTS topology_nodes (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL DEFAULT 'default',
    node_type          TEXT NOT NULL DEFAULT 'service',
    name               TEXT NOT NULL,
    display_name       TEXT NOT NULL DEFAULT '',
    tier               TEXT NOT NULL DEFAULT 'internal',
    owner_team         TEXT NOT NULL DEFAULT '',
    is_customer_facing BOOLEAN NOT NULL DEFAULT false,
    version            TEXT NOT NULL DEFAULT '',
    environment        TEXT NOT NULL DEFAULT 'production',
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    auto_discovered    BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_nodes_tenant_name ON topology_nodes (tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_tenant ON topology_nodes (tenant_id);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_type   ON topology_nodes (tenant_id, node_type);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_team   ON topology_nodes (tenant_id, owner_team);

CREATE TABLE IF NOT EXISTS topology_edges (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL DEFAULT 'default',
    from_node_id       TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    to_node_id         TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    relation           TEXT NOT NULL DEFAULT 'depends_on',
    weight             REAL NOT NULL DEFAULT 1.0,
    confidence         INTEGER NOT NULL DEFAULT 50,
    propagation_factor REAL NOT NULL DEFAULT 0.7,
    latency_ms_p99     REAL NOT NULL DEFAULT 0,
    error_rate_pct     REAL NOT NULL DEFAULT 0,
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    auto_discovered    BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_edges_unique ON topology_edges (tenant_id, from_node_id, to_node_id, relation);
CREATE INDEX IF NOT EXISTS idx_topology_edges_from ON topology_edges (tenant_id, from_node_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_to   ON topology_edges (tenant_id, to_node_id);

CREATE TABLE IF NOT EXISTS topology_node_health (
    id             BIGSERIAL PRIMARY KEY,
    node_id        TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    tenant_id      TEXT NOT NULL DEFAULT 'default',
    status         TEXT NOT NULL DEFAULT 'unknown',
    cpu_pct        REAL NOT NULL DEFAULT 0,
    mem_pct        REAL NOT NULL DEFAULT 0,
    error_rate_pct REAL NOT NULL DEFAULT 0,
    latency_ms_p99 REAL NOT NULL DEFAULT 0,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_topology_node_health_node   ON topology_node_health (node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_topology_node_health_tenant ON topology_node_health (tenant_id, recorded_at DESC);
`,
	},

	// ─── Status page subscriptions ───────────────────────────────────────
	// StatusStore.Subscribe/Unsubscribe have always referenced this table,
	// but no migration created it — so every subscribe and unsubscribe on the
	// public status page failed with a 500. The unique constraint is required
	// by Subscribe's ON CONFLICT (tenant_id, channel, target) clause, which
	// re-issues a token instead of creating duplicate rows for the same target.
	{
		name: "status_page_subscriptions",
		sql: `
CREATE TABLE IF NOT EXISTS status_subscriptions (
    id         BIGSERIAL   PRIMARY KEY,
    tenant_id  TEXT        NOT NULL DEFAULT 'default',
    channel    TEXT        NOT NULL,
    target     TEXT        NOT NULL,
    token      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, channel, target)
);
-- Unsubscribe looks up purely by token, so it needs its own unique index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_status_subscriptions_token  ON status_subscriptions (token);
CREATE INDEX        IF NOT EXISTS idx_status_subscriptions_tenant ON status_subscriptions (tenant_id, channel);
`,
	},

	// ─── Event labels ────────────────────────────────────────────────────
	// Labels (team, region, host, alertname, agent.id …) reached the Event
	// model on every ingest path but were dropped at SaveEvent — the events
	// table had no column for them. They were used in-flight to derive
	// service/severity/title and then thrown away, so nothing downstream
	// could ever filter or group by them.
	//
	// Stored as JSON rather than a side table: labels are read whole, never
	// queried individually, and a per-label row would multiply the hottest
	// table in the system by the label count.
	{
		name: "event_labels",
		sql: `
ALTER TABLE events ADD COLUMN IF NOT EXISTS labels_json TEXT NOT NULL DEFAULT '';
`,
	},

	// ─── Tenant data region ──────────────────────────────────────────────
	// TenantStore reads and writes tenants.data_region in seven places
	// (Create, List, Get, SetDataRegion, GetDataRegion) but no migration ever
	// created the column, so every one of those queries failed with
	// `column "data_region" does not exist`. That took out the whole tenant
	// admin surface — listing, creating and inspecting tenants — as well as
	// the data-residency checks that read it.
	//
	// "us" matches the COALESCE default the queries already assume, so
	// existing tenants keep the behaviour they had before the column existed.
	{
		name: "tenant_data_region",
		sql: `
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS data_region TEXT NOT NULL DEFAULT 'us';
CREATE INDEX IF NOT EXISTS idx_tenants_data_region ON tenants (data_region);
`,
	},

	// ─── SQL files that were never applied ───────────────────────────────
	// These db/migrations/*.sql files were written but runMigrations never
	// executed them (the release bundle copies them, nothing runs them). On a
	// fresh database that left action_audit without its security columns and
	// predictive incidents, customer health score, the marketplace and push
	// subscriptions without their tables, so those features failed with
	// "relation … does not exist". Every statement is IF NOT EXISTS, so
	// databases where someone ran the files by hand are unaffected.
	{
		name: "apply_unrun_sql_files",
		sql: `
-- from db/migrations/007_action_audit_security.sql
-- 007_action_audit_security.sql
-- Enriches the action_audit table with security and compliance metadata.
--
-- Before this migration, action_audit had no actor attribution, no source IP,
-- no record of what command was actually forwarded, and no execution mode.
-- This made compliance audits and incident investigations difficult.
--
-- After this migration, every action execution record carries:
--   - actor_id     : the JWT sub (user ID) who triggered the action
--   - tenant_id    : which tenant the action belongs to
--   - source_ip    : caller's IP address for security attribution
--   - command      : the exact command string forwarded to the agent (or empty)
--   - action_type  : diagnostic | investigation | remediation | coordination | verification
--   - risk_level   : low | medium | high
--   - execution_mode: observe | act (see security.ExecutionMode)
--   - policy_reason: why the policy engine allowed or blocked the command

ALTER TABLE action_audit
    ADD COLUMN IF NOT EXISTS actor_id       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tenant_id      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_ip      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS command        TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS action_type    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS risk_level     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS policy_reason  TEXT NOT NULL DEFAULT '';

-- Indexes for compliance queries: "show all actions by actor" / "show all high-risk acts"
CREATE INDEX IF NOT EXISTS idx_action_audit_actor_id
    ON action_audit (actor_id);

CREATE INDEX IF NOT EXISTS idx_action_audit_tenant_id
    ON action_audit (tenant_id);

CREATE INDEX IF NOT EXISTS idx_action_audit_execution_mode
    ON action_audit (tenant_id, execution_mode, executed_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_audit_action_type
    ON action_audit (incident_id, action_type);

-- from db/migrations/014_predictive_incidents.sql
-- 014: Predictive Incidents — alert before the alert fires
-- Two tables:
--   metric_series       — rolling time-series buffer for trend computation
--   predictive_incidents — predictions created when a trend will breach SLO

CREATE TABLE IF NOT EXISTS metric_series (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    service     TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_metric_series_lookup
    ON metric_series (tenant_id, service, metric_name, recorded_at DESC);

-- Automatically purge data points older than 2 hours to keep the table small.
-- The background worker calls DELETE older than 2h every 30 minutes.
-- Nothing else is needed at the schema level.

CREATE TABLE IF NOT EXISTS predictive_incidents (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    service              TEXT NOT NULL,
    metric_name          TEXT NOT NULL,
    current_value        DOUBLE PRECISION NOT NULL DEFAULT 0,
    slo_threshold        DOUBLE PRECISION NOT NULL DEFAULT 0,
    trend_slope          DOUBLE PRECISION NOT NULL DEFAULT 0,
    breach_probability   DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_interval  DOUBLE PRECISION NOT NULL DEFAULT 95,
    predicted_breach_min INTEGER NOT NULL DEFAULT 0,
    predicted_breach_max INTEGER NOT NULL DEFAULT 0,
    status               TEXT NOT NULL DEFAULT 'open',
    message              TEXT NOT NULL DEFAULT '',
    data_points_used     INTEGER NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_predictive_incidents_tenant
    ON predictive_incidents (tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_predictive_incidents_service
    ON predictive_incidents (tenant_id, service, metric_name, status);

-- from db/migrations/016_health_score.sql
-- SaaS Feature 4: Customer Health Score & Churn Prevention
-- Two tables: snapshot cache for computed scores, and CSM action log.

-- Snapshot cache: stores the most-recent health score computation per tenant.
-- Re-computed on every GET request; older rows are retained for trend analysis.
CREATE TABLE IF NOT EXISTS tenant_health_snapshots (
    id          BIGSERIAL    PRIMARY KEY,
    tenant_id   TEXT         NOT NULL,
    score       INT          NOT NULL DEFAULT 0,
    churn_risk  TEXT         NOT NULL DEFAULT 'medium',
    signals_json TEXT        NOT NULL DEFAULT '{}',
    actions_json TEXT        NOT NULL DEFAULT '[]',
    computed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_health_snapshots_tenant_computed
    ON tenant_health_snapshots (tenant_id, computed_at DESC);

-- CSM action log: records every customer-success action taken on a tenant.
CREATE TABLE IF NOT EXISTS tenant_health_actions (
    id          BIGSERIAL    PRIMARY KEY,
    tenant_id   TEXT         NOT NULL,
    actor       TEXT         NOT NULL DEFAULT '',
    action_type TEXT         NOT NULL,  -- 'email' | 'call' | 'note' | 'task'
    notes       TEXT         NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_health_actions_tenant_id
    ON tenant_health_actions (tenant_id, created_at DESC);

-- from db/migrations/017_marketplace.sql
-- SaaS Feature 5: Marketplace / Integration Hub
-- Per-tenant integration enable state, auth config, routing rules, and test results.
-- The integration catalog itself lives in Go code (static, product-defined).

CREATE TABLE IF NOT EXISTS marketplace_configs (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       TEXT         NOT NULL,
    integration_id  TEXT         NOT NULL,   -- matches catalog ID, e.g. "prometheus"
    enabled         BOOLEAN      NOT NULL DEFAULT false,
    auth_config     TEXT         NOT NULL DEFAULT '{}',   -- JSON: API keys, tokens, URLs
    routing_config  TEXT         NOT NULL DEFAULT '{}',   -- JSON: severity/event-type filters
    source_id       TEXT         NOT NULL DEFAULT '',     -- source_registry ID if inbound
    last_tested_at  TIMESTAMPTZ,
    test_status     TEXT         NOT NULL DEFAULT '',     -- 'ok' | 'error' | 'config_ready'
    test_message    TEXT         NOT NULL DEFAULT '',
    enabled_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, integration_id)
);

CREATE INDEX IF NOT EXISTS idx_marketplace_configs_tenant
    ON marketplace_configs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_marketplace_configs_enabled
    ON marketplace_configs (tenant_id, enabled) WHERE enabled = true;

-- from db/migrations/019_push_subscriptions.sql
-- 019_push_subscriptions.sql
-- Mobile push notification subscriptions.
-- Stores Expo Push Tokens (handles FCM + APNs routing) and Web Push subscriptions.

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id           TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id      TEXT        NOT NULL,
    tenant_id    TEXT        NOT NULL DEFAULT 'default',
    -- 'expo' = Expo Push Token (covers iOS + Android via Expo)
    -- 'web'  = Web Push / VAPID subscription
    platform     TEXT        NOT NULL CHECK (platform IN ('expo', 'web')),
    -- Expo push token (ExponentPushToken[...]) or Web Push endpoint URL
    device_token TEXT        NOT NULL,
    -- Web Push keys (only for platform = 'web')
    p256dh       TEXT,
    auth_key     TEXT,
    -- App version for targeted rollouts
    app_version  TEXT        NOT NULL DEFAULT '1.0.0',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, device_token)
);

CREATE INDEX IF NOT EXISTS idx_push_subs_tenant  ON push_subscriptions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_push_subs_user    ON push_subscriptions (user_id);
`,
	},

	// ─── Auto-close on recovery ──────────────────────────────────────────
	// events.alert_status records what the source said about the alert:
	// 'firing', 'resolved', or '' for sources that never send a resolve
	// (OTel, custom). An incident is auto-closed only when every alert in it
	// is 'resolved'; incident_auto_close holds incidents in their quiet period.
	{
		name: "alert_auto_close",
		sql: `
ALTER TABLE events ADD COLUMN IF NOT EXISTS alert_status TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS incident_auto_close (
    incident_id  TEXT        PRIMARY KEY,
    tenant_id    TEXT        NOT NULL,
    recovered_at TIMESTAMPTZ NOT NULL,
    closes_at    TIMESTAMPTZ NOT NULL,
    alert_count  INTEGER     NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_incident_auto_close_due ON incident_auto_close (closes_at);
`,
	},

	// ─── Alert mutes ─────────────────────────────────────────────────────
	// Admin-created rules that stop matching alerts from creating or
	// updating incidents. Replaces the old "5 noise votes = muted forever"
	// fingerprint suppression. Every mute ends 7 days after creation.
	//
	// alert_mute_log keeps one row per alert a mute stopped, so muted
	// alerts stay reviewable instead of disappearing.
	{
		name: "alert_mutes",
		sql: `
CREATE TABLE IF NOT EXISTS alert_mutes (
    id               TEXT        PRIMARY KEY,
    tenant_id        TEXT        NOT NULL,
    alert_names_json TEXT        NOT NULL DEFAULT '[]',
    devices_json     TEXT        NOT NULL DEFAULT '[]',
    service          TEXT        NOT NULL DEFAULT '',
    environment      TEXT        NOT NULL DEFAULT '',
    value_min        DOUBLE PRECISION,
    value_max        DOUBLE PRECISION,
    reason           TEXT        NOT NULL,
    incident_id      TEXT        NOT NULL DEFAULT '',
    created_by       TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at          TIMESTAMPTZ NOT NULL,
    unmuted_at       TIMESTAMPTZ,
    unmuted_by       TEXT        NOT NULL DEFAULT '',
    match_count      BIGINT      NOT NULL DEFAULT 0,
    last_matched_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_mutes_tenant_active ON alert_mutes (tenant_id, ends_at) WHERE unmuted_at IS NULL;

CREATE TABLE IF NOT EXISTS alert_mute_log (
    id          BIGSERIAL   PRIMARY KEY,
    tenant_id   TEXT        NOT NULL,
    mute_id     TEXT        NOT NULL,
    event_id    TEXT        NOT NULL,
    alert_name  TEXT        NOT NULL DEFAULT '',
    device      TEXT        NOT NULL DEFAULT '',
    service     TEXT        NOT NULL DEFAULT '',
    value       TEXT        NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_mute_log_tenant_ts ON alert_mute_log (tenant_id, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_mute_log_mute      ON alert_mute_log (mute_id, received_at DESC);
`,
	},
}
