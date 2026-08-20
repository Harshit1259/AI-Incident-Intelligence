-- 006_tenant_isolation_hardening.sql
-- Makes tenant isolation complete across all tables that were missing it.

-- ── Events: add tenant_id column ─────────────────────────────────────────────
-- Events were previously stored without a tenant, making cross-tenant leakage
-- possible for any endpoint that reads events.
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
CREATE INDEX IF NOT EXISTS idx_events_tenant_id ON events (tenant_id);

-- ── Source Registry: add ingest_token column ─────────────────────────────────
-- Public ingest endpoints (Prometheus, generic webhook) now authenticate via
-- a per-source ingest token. The token is looked up to derive the owning tenant.
ALTER TABLE source_registry ADD COLUMN IF NOT EXISTS ingest_token TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_source_registry_ingest_token
    ON source_registry (ingest_token)
    WHERE ingest_token != '';

-- ── Backfill: assign existing events to the default tenant ───────────────────
UPDATE events SET tenant_id = 'default' WHERE tenant_id = '' OR tenant_id IS NULL;
