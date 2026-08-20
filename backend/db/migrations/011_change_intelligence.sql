-- 011_change_intelligence.sql
-- Extends the change tracking system with enriched metadata, feature flag
-- correlation, config drift detection, and the data needed to build a
-- "what changed first" causality timeline.

-- ─── Enrich the existing changes table ───────────────────────────────────────
ALTER TABLE changes
    ADD COLUMN IF NOT EXISTS tenant_id           TEXT    NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS environment         TEXT    NOT NULL DEFAULT 'production',
    ADD COLUMN IF NOT EXISTS commit_sha          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS author              TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pr_number           TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS changed_files_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS change_source       TEXT    NOT NULL DEFAULT 'webhook',
    ADD COLUMN IF NOT EXISTS metadata_json       TEXT    NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS correlation_score   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS linked_incident_id  TEXT    NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_changes_tenant_timestamp
    ON changes (tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_changes_linked_incident
    ON changes (linked_incident_id) WHERE linked_incident_id != '';

-- ─── Feature flag changes ─────────────────────────────────────────────────────
-- Records every feature flag toggle for correlation with incidents.
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

CREATE INDEX IF NOT EXISTS idx_feature_flag_changes_tenant_ts
    ON feature_flag_changes (tenant_id, timestamp DESC);

-- ─── Config baselines ─────────────────────────────────────────────────────────
-- Stores the "known-good" baseline value for each service config key.
CREATE TABLE IF NOT EXISTS config_baselines (
    id              SERIAL PRIMARY KEY,
    tenant_id       TEXT    NOT NULL DEFAULT 'default',
    service         TEXT    NOT NULL,
    config_key      TEXT    NOT NULL,
    baseline_value  TEXT    NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, service, config_key)
);

-- ─── Config drift events ──────────────────────────────────────────────────────
-- Recorded whenever a config value deviates from its known-good baseline.
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

CREATE INDEX IF NOT EXISTS idx_config_drift_tenant_ts
    ON config_drift_events (tenant_id, detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_config_drift_service
    ON config_drift_events (tenant_id, service, detected_at DESC);
