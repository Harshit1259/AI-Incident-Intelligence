-- 008_typed_timestamps.sql
-- Fixes the timestamp typing bug: two TEXT columns that should be TIMESTAMPTZ.
--
-- Changes table: timestamp column was created as TEXT in 004_day28_incident_intelligence.sql.
-- Incidents table: what_changed_timestamp column was created as TEXT in the same migration.
--
-- Text timestamps break:
--   - ORDER BY timestamp (lexicographic order != chronological order for all formats)
--   - Date range queries with BETWEEN / < / >
--   - pg_dump time-zone consistency
--   - Any future analytics aggregation by time bucket
--
-- Strategy: add a new TIMESTAMPTZ column, backfill using the stored RFC3339 string,
-- swap the column name, drop the old column. This is safe on a live database because
-- both columns exist during the migration and the old column is dropped last.

BEGIN;

-- ── changes.timestamp ──────────────────────────────────────────────────────

-- Step 1: add typed column alongside the existing TEXT column.
ALTER TABLE changes ADD COLUMN IF NOT EXISTS ts TIMESTAMPTZ;

-- Step 2: backfill from the TEXT column.
-- Rows with an empty string or non-parseable value become NULL.
UPDATE changes
SET ts = CASE
    WHEN timestamp IS NOT NULL AND timestamp <> ''
    THEN timestamp::TIMESTAMPTZ
    ELSE NULL
END;

-- Step 3: make the new column NOT NULL with a safe default for any NULLs.
UPDATE changes SET ts = NOW() WHERE ts IS NULL;
ALTER TABLE changes ALTER COLUMN ts SET NOT NULL;
ALTER TABLE changes ALTER COLUMN ts SET DEFAULT NOW();

-- Step 4: drop the old TEXT column and rename.
ALTER TABLE changes DROP COLUMN IF EXISTS timestamp;
ALTER TABLE changes RENAME COLUMN ts TO timestamp;

-- Step 5: recreate the index on the typed column (old index is dropped with the column).
DROP INDEX IF EXISTS idx_changes_service_timestamp;
CREATE INDEX IF NOT EXISTS idx_changes_service_timestamp
ON changes (service, timestamp DESC);

-- ── incidents.what_changed_timestamp ────────────────────────────────────────

-- Step 1: add typed column.
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS what_changed_ts TIMESTAMPTZ;

-- Step 2: backfill — empty string becomes NULL (no change event).
UPDATE incidents
SET what_changed_ts = CASE
    WHEN what_changed_timestamp IS NOT NULL AND what_changed_timestamp <> ''
    THEN what_changed_timestamp::TIMESTAMPTZ
    ELSE NULL
END;

-- Nullable is correct here: most incidents have no change event linked.
-- Step 3: drop and rename.
ALTER TABLE incidents DROP COLUMN IF EXISTS what_changed_timestamp;
ALTER TABLE incidents RENAME COLUMN what_changed_ts TO what_changed_timestamp;

COMMIT;
