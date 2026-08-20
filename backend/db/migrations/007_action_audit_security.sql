-- 007_action_audit_security.sql
-- Enriches the action_audit table with security and compliance metadata.
--
-- Before this migration, action_audit had no actor attribution, no source IP,
-- no record of what command was actually forwarded, and no execution mode.
-- This made compliance audits and incident investigations difficult.
--
-- After this migration, every action execution record carries:
--   - actor_id     : the JWT sub (user ID) who triggered the action
--   - tenant_id    : which tenant the action belongs to
--   - source_ip    : caller's IP address for security attribution
--   - command      : the exact command string forwarded to the agent (or empty)
--   - action_type  : diagnostic | investigation | remediation | coordination | verification
--   - risk_level   : low | medium | high
--   - execution_mode: observe | act (see security.ExecutionMode)
--   - policy_reason: why the policy engine allowed or blocked the command

ALTER TABLE action_audit
    ADD COLUMN IF NOT EXISTS actor_id       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tenant_id      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_ip      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS command        TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS action_type    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS risk_level     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS policy_reason  TEXT NOT NULL DEFAULT '';

-- Indexes for compliance queries: "show all actions by actor" / "show all high-risk acts"
CREATE INDEX IF NOT EXISTS idx_action_audit_actor_id
    ON action_audit (actor_id);

CREATE INDEX IF NOT EXISTS idx_action_audit_tenant_id
    ON action_audit (tenant_id);

CREATE INDEX IF NOT EXISTS idx_action_audit_execution_mode
    ON action_audit (tenant_id, execution_mode, executed_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_audit_action_type
    ON action_audit (incident_id, action_type);
