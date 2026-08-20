import { useCallback, useEffect, useState } from "react";
import {
  createPolicy,
  deletePolicy,
  evaluatePolicy,
  getCircuitBreakers,
  getPolicies,
  getPolicyTrail,
  getTenantTrail,
  resetCircuitBreaker,
  simulatePolicy,
  updatePolicy,
} from "../api/policy.js";

// ── tiny helpers ──────────────────────────────────────────────────────────────
const fmt = (v) => (v ? new Date(v).toLocaleString() : "—");
const badge = (color, label) => (
  <span
    style={{
      display: "inline-block",
      padding: "2px 8px",
      borderRadius: 4,
      fontSize: 11,
      fontWeight: 600,
      background: color,
      color: "#fff",
      marginLeft: 4,
    }}
  >
    {label}
  </span>
);

const DECISION_COLORS = {
  allowed: "#16a34a",
  denied: "#dc2626",
  simulation: "#7c3aed",
  dry_run: "#0284c7",
};

const MODE_COLORS = {
  auto: "#16a34a",
  manual: "#6b7280",
  approval: "#d97706",
  simulation: "#7c3aed",
  dry_run: "#0284c7",
};

function decisionBadge(decision) {
  return badge(DECISION_COLORS[decision] ?? "#6b7280", decision ?? "unknown");
}

function modeBadge(mode) {
  return badge(MODE_COLORS[mode] ?? "#6b7280", mode ?? "—");
}

// ── sub-components ────────────────────────────────────────────────────────────

function PolicyForm({ initial, onSave, onCancel }) {
  const blank = {
    name: "",
    service: "",
    environment: "",
    severity: "",
    priority: 0,
    enabled: true,
    execution_mode: "manual",
    requires_approval: false,
    approval_groups: "",
    approval_timeout_minutes: 60,
    blackout_start: "",
    blackout_end: "",
    business_hours_start: "",
    business_hours_end: "",
    business_hours_timezone: "UTC",
    enforce_business_hours: false,
    max_per_hour: 10,
    max_concurrent_actions: 0,
    max_impacted_services: 0,
    max_retries: 2,
    retry_strategy: "exponential",
    retry_backoff_seconds: 30,
    cooldown_seconds: 300,
    timeout_seconds: 90,
    circuit_breaker_enabled: false,
    circuit_breaker_threshold: 5,
    circuit_breaker_window_seconds: 3600,
    circuit_breaker_cooldown_minutes: 30,
    simulation_mode: false,
    dry_run_mode: false,
    rollback_webhook_url: "",
    rollback_script_ref: "",
  };

  const [form, setForm] = useState(initial || blank);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState("");

  const set = (key, val) => setForm((f) => ({ ...f, [key]: val }));

  const handleSubmit = async () => {
    if (!form.name.trim()) {
      setErr("Name is required");
      return;
    }
    setSaving(true);
    setErr("");
    try {
      await onSave(form);
    } catch (e) {
      setErr(e.message || "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const field = (label, key, type = "text", extra = {}) => (
    <div style={{ marginBottom: 12 }}>
      <label style={{ display: "block", fontSize: 12, color: "#9ca3af", marginBottom: 4 }}>
        {label}
      </label>
      <input
        type={type}
        value={form[key] ?? ""}
        onChange={(e) =>
          set(key, type === "number" ? Number(e.target.value) : e.target.value)
        }
        style={{
          width: "100%",
          padding: "6px 10px",
          background: "#1e2532",
          border: "1px solid #374151",
          borderRadius: 6,
          color: "#e5e7eb",
          fontSize: 13,
          boxSizing: "border-box",
        }}
        {...extra}
      />
    </div>
  );

  const check = (label, key) => (
    <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
      <input
        type="checkbox"
        checked={!!form[key]}
        onChange={(e) => set(key, e.target.checked)}
        style={{ width: 15, height: 15 }}
      />
      <label style={{ fontSize: 13, color: "#d1d5db" }}>{label}</label>
    </div>
  );

  const select = (label, key, options) => (
    <div style={{ marginBottom: 12 }}>
      <label style={{ display: "block", fontSize: 12, color: "#9ca3af", marginBottom: 4 }}>
        {label}
      </label>
      <select
        value={form[key] ?? ""}
        onChange={(e) => set(key, e.target.value)}
        style={{
          width: "100%",
          padding: "6px 10px",
          background: "#1e2532",
          border: "1px solid #374151",
          borderRadius: 6,
          color: "#e5e7eb",
          fontSize: 13,
        }}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  );

  const section = (title) => (
    <div
      style={{
        fontSize: 11,
        fontWeight: 700,
        color: "#6b7280",
        textTransform: "uppercase",
        letterSpacing: 1,
        marginTop: 20,
        marginBottom: 10,
        borderBottom: "1px solid #374151",
        paddingBottom: 4,
      }}
    >
      {title}
    </div>
  );

  return (
    <div style={{ padding: 20 }}>
      {section("Identity")}
      {field("Name *", "name")}
      {field("Priority (0–100, higher wins)", "priority", "number", { min: 0, max: 100 })}
      {check("Enabled", "enabled")}

      {section("Matching")}
      {field("Service (blank = all)", "service")}
      {field("Environment (prod / staging / dev / blank = all)", "environment")}
      {field("Severity (critical / high / medium / low / blank = all)", "severity")}

      {section("Execution Mode")}
      {select("Mode", "execution_mode", [
        { value: "manual", label: "Manual" },
        { value: "auto", label: "Auto" },
        { value: "approval", label: "Approval Required" },
        { value: "simulation", label: "Simulation" },
        { value: "dry_run", label: "Dry Run (log only)" },
      ])}
      {check("Requires Approval", "requires_approval")}
      {field("Approval Groups (comma-separated)", "approval_groups")}
      {field("Approval Timeout (minutes)", "approval_timeout_minutes", "number", { min: 0 })}

      {section("Time Windows")}
      {field("Blackout Start (HH:MM UTC)", "blackout_start")}
      {field("Blackout End (HH:MM UTC)", "blackout_end")}
      {check("Enforce Business Hours", "enforce_business_hours")}
      {field("Business Hours Start (HH:MM)", "business_hours_start")}
      {field("Business Hours End (HH:MM)", "business_hours_end")}
      {field("Business Hours Timezone (IANA e.g. America/New_York)", "business_hours_timezone")}

      {section("Blast-Radius Controls")}
      {field("Max Actions / Hour (0 = unlimited)", "max_per_hour", "number", { min: 0 })}
      {field("Max Concurrent Actions (0 = unlimited)", "max_concurrent_actions", "number", { min: 0 })}
      {field("Max Impacted Services (0 = unlimited)", "max_impacted_services", "number", { min: 0 })}

      {section("Retry / Timeout")}
      {field("Max Retries", "max_retries", "number", { min: 0 })}
      {select("Retry Strategy", "retry_strategy", [
        { value: "fixed", label: "Fixed" },
        { value: "linear", label: "Linear" },
        { value: "exponential", label: "Exponential" },
      ])}
      {field("Retry Backoff (seconds)", "retry_backoff_seconds", "number", { min: 0 })}
      {field("Cooldown (seconds)", "cooldown_seconds", "number", { min: 0 })}
      {field("Timeout (seconds)", "timeout_seconds", "number", { min: 0 })}

      {section("Circuit Breaker")}
      {check("Circuit Breaker Enabled", "circuit_breaker_enabled")}
      {field("Failure Threshold", "circuit_breaker_threshold", "number", { min: 1 })}
      {field("Window (seconds)", "circuit_breaker_window_seconds", "number", { min: 1 })}
      {field("Cooldown (minutes)", "circuit_breaker_cooldown_minutes", "number", { min: 1 })}

      {section("Simulation / Dry-Run")}
      {check("Simulation Mode (record, don't execute)", "simulation_mode")}
      {check("Dry-Run Mode (evaluate + log only)", "dry_run_mode")}

      {section("Rollback Hooks")}
      {field("Rollback Webhook URL", "rollback_webhook_url")}
      {field("Rollback Script / Runbook Ref", "rollback_script_ref")}

      {err && (
        <div style={{ color: "#f87171", fontSize: 13, marginBottom: 12 }}>{err}</div>
      )}

      <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
        <button
          onClick={handleSubmit}
          disabled={saving}
          style={{
            padding: "8px 20px",
            background: saving ? "#374151" : "#3b82f6",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: saving ? "default" : "pointer",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {saving ? "Saving…" : "Save Policy"}
        </button>
        <button
          onClick={onCancel}
          style={{
            padding: "8px 20px",
            background: "#374151",
            color: "#d1d5db",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}

function SimulatePanel() {
  const [ctx, setCtx] = useState({
    service: "",
    environment: "prod",
    severity: "critical",
    impacted_services: 1,
    action_id: "test-action",
    is_dry_run: false,
  });
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  const set = (k, v) => setCtx((c) => ({ ...c, [k]: v }));

  const run = async (evaluate) => {
    setLoading(true);
    setErr("");
    setResult(null);
    try {
      const data = evaluate ? await evaluatePolicy(ctx) : await simulatePolicy(ctx);
      setResult(data);
    } catch (e) {
      setErr(e.message || "Request failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div style={{ marginBottom: 16, color: "#9ca3af", fontSize: 13 }}>
        Test a policy decision for any context without creating an incident or action.
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 16 }}>
        {[
          ["Service", "service"],
          ["Environment", "environment"],
          ["Severity", "severity"],
          ["Impacted Services", "impacted_services", "number"],
          ["Action ID (optional)", "action_id"],
        ].map(([label, key, type]) => (
          <div key={key}>
            <label style={{ display: "block", fontSize: 12, color: "#9ca3af", marginBottom: 4 }}>
              {label}
            </label>
            <input
              type={type || "text"}
              value={ctx[key] ?? ""}
              onChange={(e) =>
                set(key, type === "number" ? Number(e.target.value) : e.target.value)
              }
              style={{
                width: "100%",
                padding: "6px 10px",
                background: "#1e2532",
                border: "1px solid #374151",
                borderRadius: 6,
                color: "#e5e7eb",
                fontSize: 13,
                boxSizing: "border-box",
              }}
            />
          </div>
        ))}
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <input
            type="checkbox"
            checked={ctx.is_dry_run}
            onChange={(e) => set("is_dry_run", e.target.checked)}
          />
          <label style={{ fontSize: 13, color: "#d1d5db" }}>Dry-run flag</label>
        </div>
      </div>

      <div style={{ display: "flex", gap: 10, marginBottom: 20 }}>
        <button
          onClick={() => run(false)}
          disabled={loading}
          style={{
            padding: "8px 18px",
            background: "#7c3aed",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: loading ? "default" : "pointer",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          Simulate (no trail)
        </button>
        <button
          onClick={() => run(true)}
          disabled={loading}
          style={{
            padding: "8px 18px",
            background: "#0284c7",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: loading ? "default" : "pointer",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          Evaluate (writes trail)
        </button>
      </div>

      {err && <div style={{ color: "#f87171", fontSize: 13, marginBottom: 12 }}>{err}</div>}

      {result && (
        <div
          style={{
            background: "#1e2532",
            border: "1px solid #374151",
            borderRadius: 8,
            padding: 16,
          }}
        >
          <div style={{ fontSize: 13, fontWeight: 700, marginBottom: 12, color: "#d1d5db" }}>
            Decision
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "6px 16px", fontSize: 13 }}>
            {[
              ["Allowed", result.decision?.allowed ? "YES" : "NO"],
              ["Mode", result.decision?.execution_mode],
              ["Simulation", result.decision?.is_simulation ? "Yes" : "No"],
              ["Dry-Run", result.decision?.is_dry_run ? "Yes" : "No"],
              ["Circuit Open", result.decision?.circuit_open ? "Yes" : "No"],
              ["Denial Code", result.decision?.denial_code || "—"],
              ["Policy ID", result.decision?.policy_id || "(none)"],
              ["Reason", result.decision?.reason],
            ].map(([k, v]) => (
              <>
                <span key={`k-${k}`} style={{ color: "#6b7280", fontWeight: 600 }}>{k}</span>
                <span key={`v-${k}`} style={{ color: v === "NO" ? "#f87171" : v === "YES" ? "#4ade80" : "#e5e7eb" }}>
                  {v ?? "—"}
                </span>
              </>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function TrailTable({ entries }) {
  if (!entries || entries.length === 0) {
    return <div style={{ color: "#6b7280", fontSize: 13, padding: "20px 0" }}>No trail entries.</div>;
  }
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
        <thead>
          <tr style={{ background: "#1e2532", color: "#6b7280" }}>
            {["Time", "Policy", "Service", "Env", "Severity", "Decision", "Mode", "Actor", "Reason"].map((h) => (
              <th key={h} style={{ padding: "8px 10px", textAlign: "left", whiteSpace: "nowrap" }}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {entries.map((t) => (
            <tr
              key={t.id}
              style={{ borderBottom: "1px solid #1f2937", color: "#d1d5db" }}
            >
              <td style={{ padding: "6px 10px", whiteSpace: "nowrap" }}>{fmt(t.evaluated_at)}</td>
              <td style={{ padding: "6px 10px" }}>{t.policy_name || t.policy_id || "—"}</td>
              <td style={{ padding: "6px 10px" }}>{t.service || "—"}</td>
              <td style={{ padding: "6px 10px" }}>{t.environment || "—"}</td>
              <td style={{ padding: "6px 10px" }}>{t.severity || "—"}</td>
              <td style={{ padding: "6px 10px" }}>{decisionBadge(t.decision)}</td>
              <td style={{ padding: "6px 10px" }}>{modeBadge(t.execution_mode)}</td>
              <td style={{ padding: "6px 10px" }}>{t.actor || "system"}</td>
              <td style={{ padding: "6px 10px", maxWidth: 280 }}>{t.reason || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CircuitBreakerPanel({ breakers, onReset }) {
  if (!breakers || breakers.length === 0) {
    return (
      <div style={{ color: "#6b7280", fontSize: 13, padding: "20px 0" }}>
        No circuit breaker state recorded yet.
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {breakers.map((b) => (
        <div
          key={b.policy_id}
          style={{
            background: "#1e2532",
            border: `1px solid ${b.is_open ? "#dc2626" : "#374151"}`,
            borderRadius: 8,
            padding: 14,
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <div>
            <div style={{ fontSize: 13, fontWeight: 700, color: "#d1d5db", marginBottom: 4 }}>
              {b.policy_id}
              {b.is_open
                ? badge("#dc2626", "OPEN")
                : badge("#16a34a", "CLOSED")}
            </div>
            <div style={{ fontSize: 12, color: "#9ca3af" }}>
              Consecutive failures: {b.consecutive_failures}
              {b.last_failure_at && <> · Last failure: {fmt(b.last_failure_at)}</>}
              {b.will_reset_at && <> · Resets: {fmt(b.will_reset_at)}</>}
            </div>
          </div>
          {b.is_open && (
            <button
              onClick={() => onReset(b.policy_id)}
              style={{
                padding: "6px 14px",
                background: "#374151",
                color: "#d1d5db",
                border: "none",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: 12,
                fontWeight: 600,
              }}
            >
              Reset
            </button>
          )}
        </div>
      ))}
    </div>
  );
}

// ── main component ────────────────────────────────────────────────────────────

const TABS = ["Policies", "Simulate", "Execution Trail", "Circuit Breakers"];

export default function AutomationPolicyEngine() {
  const [tab, setTab] = useState("Policies");
  const [policies, setPolicies] = useState([]);
  const [trail, setTrail] = useState([]);
  const [breakers, setBreakers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [selectedPolicy, setSelectedPolicy] = useState(null);
  const [policyTrail, setPolicyTrail] = useState([]);

  const load = useCallback(async () => {
    setLoading(true);
    setErr("");
    try {
      const [p, t, b] = await Promise.all([
        getPolicies(),
        getTenantTrail(100),
        getCircuitBreakers(),
      ]);
      setPolicies(p.policies || []);
      setTrail(t.trail || []);
      setBreakers(b.circuit_breakers || []);
    } catch (e) {
      setErr(e.message || "Failed to load policy data");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleSave = async (form) => {
    if (editTarget) {
      await updatePolicy(editTarget.id, form);
    } else {
      await createPolicy(form);
    }
    setShowForm(false);
    setEditTarget(null);
    load();
  };

  const [deleteTarget, setDeleteTarget] = useState(null);

  const handleDelete = async (id) => {
    setDeleteTarget(id);
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    await deletePolicy(deleteTarget);
    setDeleteTarget(null);
    load();
  };

  const handleReset = async (policyID) => {
    await resetCircuitBreaker(policyID);
    load();
  };

  const handleSelectPolicy = async (p) => {
    setSelectedPolicy(p);
    const data = await getPolicyTrail(p.id, 50);
    setPolicyTrail(data.trail || []);
    setTab("Execution Trail");
  };

  if (showForm) {
    return (
      <div
        style={{
          background: "#111827",
          borderRadius: 10,
          border: "1px solid #374151",
          maxHeight: "80vh",
          overflowY: "auto",
        }}
      >
        <div
          style={{
            padding: "16px 20px",
            borderBottom: "1px solid #374151",
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <h3 style={{ margin: 0, fontSize: 16, color: "#e5e7eb" }}>
            {editTarget ? "Edit Policy" : "Create Policy"}
          </h3>
        </div>
        <PolicyForm
          initial={editTarget}
          onSave={handleSave}
          onCancel={() => {
            setShowForm(false);
            setEditTarget(null);
          }}
        />
      </div>
    );
  }

  return (
    <div style={{ fontFamily: "inherit" }}>
      {/* Delete confirmation modal */}
      {deleteTarget && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.7)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 1000,
          }}
        >
          <div
            style={{
              background: "#111827",
              border: "1px solid #374151",
              borderRadius: 10,
              padding: 24,
              maxWidth: 380,
              width: "90%",
            }}
          >
            <h3 style={{ margin: "0 0 12px", color: "#e5e7eb", fontSize: 16 }}>
              Delete Policy
            </h3>
            <p style={{ color: "#9ca3af", fontSize: 13, margin: "0 0 20px" }}>
              This will permanently remove the policy and stop it from being
              evaluated. This action cannot be undone.
            </p>
            <div style={{ display: "flex", gap: 10 }}>
              <button
                onClick={confirmDelete}
                style={{
                  padding: "8px 18px",
                  background: "#dc2626",
                  color: "#fff",
                  border: "none",
                  borderRadius: 6,
                  cursor: "pointer",
                  fontSize: 13,
                  fontWeight: 600,
                }}
              >
                Delete
              </button>
              <button
                onClick={() => setDeleteTarget(null)}
                style={{
                  padding: "8px 18px",
                  background: "#374151",
                  color: "#d1d5db",
                  border: "none",
                  borderRadius: 6,
                  cursor: "pointer",
                  fontSize: 13,
                }}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 20,
        }}
      >
        <div>
          <h2 style={{ margin: 0, fontSize: 20, color: "#e5e7eb" }}>
            Automation Policy Engine
          </h2>
          <p style={{ margin: "4px 0 0", fontSize: 13, color: "#6b7280" }}>
            Enterprise-grade controls: allow/deny rules, approval flows, blast-radius
            limits, circuit breakers, simulation, dry-run, and immutable audit trail.
          </p>
        </div>
        <div style={{ display: "flex", gap: 10 }}>
          <button
            onClick={load}
            style={{
              padding: "7px 14px",
              background: "#1f2937",
              border: "1px solid #374151",
              borderRadius: 6,
              color: "#9ca3af",
              cursor: "pointer",
              fontSize: 13,
            }}
          >
            Refresh
          </button>
          <button
            onClick={() => {
              setEditTarget(null);
              setShowForm(true);
            }}
            style={{
              padding: "7px 16px",
              background: "#3b82f6",
              border: "none",
              borderRadius: 6,
              color: "#fff",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            + New Policy
          </button>
        </div>
      </div>

      {err && (
        <div
          style={{
            background: "#1e1010",
            border: "1px solid #7f1d1d",
            borderRadius: 6,
            padding: "10px 14px",
            color: "#f87171",
            marginBottom: 16,
            fontSize: 13,
          }}
        >
          {err}
        </div>
      )}

      {/* Tab bar */}
      <div
        style={{
          display: "flex",
          gap: 2,
          borderBottom: "1px solid #374151",
          marginBottom: 20,
        }}
      >
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              padding: "8px 16px",
              background: tab === t ? "#1e40af" : "transparent",
              border: "none",
              borderRadius: "6px 6px 0 0",
              color: tab === t ? "#eff6ff" : "#6b7280",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: tab === t ? 700 : 400,
            }}
          >
            {t}
            {t === "Circuit Breakers" && breakers.filter((b) => b.is_open).length > 0 && (
              <span
                style={{
                  marginLeft: 6,
                  background: "#dc2626",
                  color: "#fff",
                  borderRadius: 10,
                  padding: "1px 6px",
                  fontSize: 10,
                }}
              >
                {breakers.filter((b) => b.is_open).length}
              </span>
            )}
          </button>
        ))}
      </div>

      {loading && (
        <div style={{ color: "#6b7280", fontSize: 13, padding: "20px 0" }}>Loading…</div>
      )}

      {/* ── Policies tab ─────────────────────────────────────────────────── */}
      {tab === "Policies" && !loading && (
        <div>
          {policies.length === 0 ? (
            <div style={{ color: "#6b7280", fontSize: 13, padding: "20px 0" }}>
              No policies yet. Create one to start controlling automation.
            </div>
          ) : (
            policies.map((p) => (
              <div
                key={p.id}
                style={{
                  background: "#111827",
                  border: "1px solid #374151",
                  borderRadius: 8,
                  padding: 16,
                  marginBottom: 12,
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "flex-start",
                }}
              >
                <div style={{ flex: 1 }}>
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      marginBottom: 6,
                    }}
                  >
                    <span style={{ fontSize: 15, fontWeight: 700, color: "#e5e7eb" }}>
                      {p.name}
                    </span>
                    {modeBadge(p.execution_mode)}
                    {!p.enabled && badge("#6b7280", "disabled")}
                    {p.simulation_mode && badge("#7c3aed", "simulation")}
                    {p.dry_run_mode && badge("#0284c7", "dry-run")}
                    {p.circuit_breaker_enabled && badge("#f59e0b", "CB")}
                  </div>
                  <div style={{ fontSize: 12, color: "#6b7280", display: "flex", gap: 16, flexWrap: "wrap" }}>
                    <span>Service: {p.service || "all"}</span>
                    <span>Env: {p.environment || "all"}</span>
                    <span>Severity: {p.severity || "all"}</span>
                    <span>Priority: {p.priority}</span>
                    {p.max_per_hour > 0 && <span>Max/hr: {p.max_per_hour}</span>}
                    {p.max_concurrent_actions > 0 && <span>MaxConcurrent: {p.max_concurrent_actions}</span>}
                    {p.requires_approval && <span style={{ color: "#d97706" }}>Approval required</span>}
                    {p.blackout_start && (
                      <span style={{ color: "#f87171" }}>
                        Blackout: {p.blackout_start}–{p.blackout_end}
                      </span>
                    )}
                    {p.enforce_business_hours && (
                      <span style={{ color: "#60a5fa" }}>
                        BH: {p.business_hours_start}–{p.business_hours_end} {p.business_hours_timezone}
                      </span>
                    )}
                  </div>
                </div>
                <div style={{ display: "flex", gap: 8, flexShrink: 0, marginLeft: 16 }}>
                  <button
                    onClick={() => handleSelectPolicy(p)}
                    style={{
                      padding: "5px 12px",
                      background: "#1f2937",
                      border: "1px solid #374151",
                      borderRadius: 5,
                      color: "#9ca3af",
                      cursor: "pointer",
                      fontSize: 12,
                    }}
                  >
                    Trail
                  </button>
                  <button
                    onClick={() => {
                      setEditTarget(p);
                      setShowForm(true);
                    }}
                    style={{
                      padding: "5px 12px",
                      background: "#1f2937",
                      border: "1px solid #374151",
                      borderRadius: 5,
                      color: "#9ca3af",
                      cursor: "pointer",
                      fontSize: 12,
                    }}
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => handleDelete(p.id)}
                    style={{
                      padding: "5px 12px",
                      background: "#1f2937",
                      border: "1px solid #7f1d1d",
                      borderRadius: 5,
                      color: "#f87171",
                      cursor: "pointer",
                      fontSize: 12,
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {/* ── Simulate tab ─────────────────────────────────────────────────── */}
      {tab === "Simulate" && <SimulatePanel />}

      {/* ── Execution Trail tab ──────────────────────────────────────────── */}
      {tab === "Execution Trail" && !loading && (
        <div>
          {selectedPolicy && (
            <div style={{ marginBottom: 12, display: "flex", alignItems: "center", gap: 10 }}>
              <span style={{ fontSize: 13, color: "#9ca3af" }}>
                Showing trail for: <strong style={{ color: "#e5e7eb" }}>{selectedPolicy.name}</strong>
              </span>
              <button
                onClick={() => {
                  setSelectedPolicy(null);
                  setPolicyTrail([]);
                }}
                style={{
                  padding: "3px 10px",
                  background: "#374151",
                  border: "none",
                  borderRadius: 4,
                  color: "#9ca3af",
                  cursor: "pointer",
                  fontSize: 12,
                }}
              >
                Show All
              </button>
            </div>
          )}
          <TrailTable entries={selectedPolicy ? policyTrail : trail} />
        </div>
      )}

      {/* ── Circuit Breakers tab ─────────────────────────────────────────── */}
      {tab === "Circuit Breakers" && !loading && (
        <CircuitBreakerPanel breakers={breakers} onReset={handleReset} />
      )}
    </div>
  );
}
