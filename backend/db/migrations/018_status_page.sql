-- 018_status_page.sql
-- Status Page: email and Slack webhook subscriptions.

CREATE TABLE IF NOT EXISTS status_subscriptions (
    id         TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id  TEXT        NOT NULL DEFAULT 'default',
    channel    TEXT        NOT NULL CHECK (channel IN ('email', 'slack')),
    target     TEXT        NOT NULL,
    token      TEXT        NOT NULL,
    verified   BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, channel, target),
    UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_status_subscriptions_tenant
    ON status_subscriptions (tenant_id);

CREATE INDEX IF NOT EXISTS idx_status_subscriptions_token
    ON status_subscriptions (token);
