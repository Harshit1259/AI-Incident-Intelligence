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
