-- Phase 1 Core Migration
-- Week 1: fingerprint dedup + correlation window key
-- Week 3: tenant isolation + JWT auth users

-- ─────────────────────────────────────────────────────
-- 1. Events: add fingerprint column
-- ─────────────────────────────────────────────────────
ALTER TABLE events ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS title       TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_events_fingerprint ON events (fingerprint);
CREATE INDEX IF NOT EXISTS idx_events_fingerprint_ts ON events (fingerprint, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_service_ts     ON events (service, timestamp);

-- ─────────────────────────────────────────────────────
-- 2. Incidents: add fingerprint + tenant
-- ─────────────────────────────────────────────────────
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS tenant_id   TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_incidents_service_status_ts ON incidents (service, status, last_event_time);
CREATE INDEX IF NOT EXISTS idx_incidents_tenant_id         ON incidents (tenant_id);
CREATE INDEX IF NOT EXISTS idx_incidents_fingerprint       ON incidents (fingerprint);

-- ─────────────────────────────────────────────────────
-- 3. Tenants table
-- ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS tenants (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO tenants (id, name, slug)
VALUES ('default', 'Default Tenant', 'default')
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────
-- 4. Users table (JWT auth)
-- ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'operator', -- 'admin' | 'operator'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email     ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users (tenant_id);

-- ─────────────────────────────────────────────────────
-- 5. Backfill existing events with fingerprint
-- ─────────────────────────────────────────────────────
UPDATE events
SET fingerprint = MD5(LOWER(COALESCE(source,'')) || ':' || LOWER(COALESCE(service,'')) || ':' || LOWER(COALESCE(severity,'')))
WHERE fingerprint = '';
