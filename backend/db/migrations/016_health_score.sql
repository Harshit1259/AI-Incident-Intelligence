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
