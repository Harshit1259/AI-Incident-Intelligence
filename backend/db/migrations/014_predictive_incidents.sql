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
