-- 010_verification_closed_loop.sql
-- Extends verification_records with closed-loop action verification fields:
-- before/after system snapshots, confidence tracking, proof items, rollback flag,
-- and auto-close eligibility gate.

ALTER TABLE verification_records
    ADD COLUMN IF NOT EXISTS before_snapshot_json  TEXT    NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS after_snapshot_json   TEXT    NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS confidence_before     INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS confidence_after      INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS proof_items_json      TEXT    NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS rollback_triggered    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS auto_close_eligible   BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_verification_records_execution
    ON verification_records (execution_id) WHERE execution_id != '';
