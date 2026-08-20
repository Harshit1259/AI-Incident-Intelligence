-- 013_change_tenant_isolation.sql
-- The changes table already has a tenant_id column (added in 011_change_intelligence.sql).
-- This migration adds a composite index that supports the tenant-scoped query used in
-- incident correlation: WHERE tenant_id = $1 AND service = $2 AND timestamp BETWEEN $3 AND $4
-- Without this index, GetRecentChangeByService scans all rows for a given tenant.

CREATE INDEX IF NOT EXISTS idx_changes_tenant_service_ts
    ON changes (tenant_id, service, timestamp DESC);
