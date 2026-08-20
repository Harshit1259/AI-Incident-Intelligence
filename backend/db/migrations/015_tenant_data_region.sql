-- Migration 015: Multi-Region Data Residency
--
-- Adds data_region to the tenants table so every tenant is permanently bound
-- to the geographic region (US / EU / APAC) where their data must be stored.
-- Existing tenants default to "us" — operators can update via PATCH /api/v1/tenants/{id}/region.

ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS data_region VARCHAR(20) NOT NULL DEFAULT 'us';

-- Index for enforcement queries (look up all tenants in a given region).
CREATE INDEX IF NOT EXISTS idx_tenants_data_region ON tenants (data_region);

-- Ensure the column comment documents the constraint for future DBAs.
COMMENT ON COLUMN tenants.data_region IS
    'Geographic data residency region. Valid values: us | eu | apac. '
    'All PII and operational data for this tenant is stored exclusively in '
    'the declared region. Cross-region requests are rejected 403.';
