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
