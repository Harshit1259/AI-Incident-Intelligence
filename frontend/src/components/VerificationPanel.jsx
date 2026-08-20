import { useState, useCallback } from "react";

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_COLOR = {
  passed:   "#34d399",
  failed:   "#f87171",
  skipped:  "#94a3b8",
  pending:  "#f59e0b",
};

const RESULT_COLOR = {
  resolved:            "#34d399",
  partially_resolved:  "#f59e0b",
  not_resolved:        "#f87171",
  rolled_back:         "#a78bfa",
  pending:             "#64748b",
};

const PROOF_ICON = {
  metric_drop:    "📉",
  latency_drop:   "⚡",
  error_clear:    "✅",
  alert_resolved: "🔔",
  manual:         "📋",
};

const PROOF_STATUS_COLOR = { improved: "#34d399", degraded: "#f87171", neutral: "#94a3b8" };

function authHeaders() {
  const token = localStorage.getItem("authToken") || "";
  return token ? { Authorization: `Bearer ${token}`, "Content-Type": "application/json" } : { "Content-Type": "application/json" };
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function ConfidenceDelta({ before, after }) {
  if (before === 0 && after === 0) return null;
  const delta = after - before;
  const color = delta >= 10 ? "#34d399" : delta >= 0 ? "#f59e0b" : "#f87171";
  const arrow = delta > 0 ? "↑" : delta < 0 ? "↓" : "→";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "10px 14px", borderRadius: 10, background: "rgba(255,255,255,0.03)", border: "1px solid rgba(90,123,186,0.15)", marginBottom: 14 }}>
      <div style={{ flex: 1 }}>
        <div style={{ fontSize: "0.62rem", color: "#64748b", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 700, marginBottom: 4 }}>CONFIDENCE</div>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span style={{ fontSize: "1.1rem", fontWeight: 800, color: "#94a3b8" }}>{before}%</span>
          <span style={{ fontSize: "0.85rem", color }}>→ {after}%</span>
          <span style={{ fontSize: "0.85rem", fontWeight: 700, color }}>{arrow} {Math.abs(delta)} pts</span>
        </div>
      </div>
    </div>
  );
}

function SnapshotComparison({ before, after }) {
  if (!before && !after) return null;

  const rows = [
    { label: "Error rate",  beforeVal: before?.error_rate_pct,   afterVal: after?.error_rate_pct,  unit: "err/min", lowerBetter: true },
    { label: "CPU",         beforeVal: before?.cpu_percent,       afterVal: after?.cpu_percent,     unit: "%",       lowerBetter: true },
    { label: "Memory",      beforeVal: before?.memory_percent,    afterVal: after?.memory_percent,  unit: "%",       lowerBetter: true },
    { label: "Latency p99", beforeVal: before?.latency_ms_p99,    afterVal: after?.latency_ms_p99,  unit: "ms",      lowerBetter: true },
    { label: "Errors",      beforeVal: before?.error_count,       afterVal: after?.error_count,     unit: "",        lowerBetter: true },
  ].filter(r => (r.beforeVal ?? 0) > 0 || (r.afterVal ?? 0) > 0);

  if (rows.length === 0) return null;

  function deltaColor(bv, av, lowerBetter) {
    if (bv === undefined || av === undefined) return "#94a3b8";
    const d = av - bv;
    if (d === 0) return "#94a3b8";
    return (d < 0) === lowerBetter ? "#34d399" : "#f87171";
  }

  return (
    <div style={{ marginBottom: 14 }}>
      <div style={{ fontSize: "0.62rem", color: "#64748b", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 700, marginBottom: 8 }}>BEFORE / AFTER COMPARISON</div>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.75rem" }}>
        <thead>
          <tr>
            {["Metric", "Before", "After", "Change"].map(h => (
              <th key={h} style={{ textAlign: h === "Metric" ? "left" : "right", color: "#475569", fontWeight: 600, paddingBottom: 4, paddingRight: 8 }}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map(({ label, beforeVal, afterVal, unit, lowerBetter }) => {
            const bv = beforeVal ?? 0;
            const av = afterVal ?? 0;
            const delta = av - bv;
            const color = deltaColor(bv, av, lowerBetter);
            return (
              <tr key={label}>
                <td style={{ color: "#e2e8f0", paddingBottom: 4, paddingRight: 8 }}>{label}</td>
                <td style={{ textAlign: "right", color: "#64748b", paddingRight: 8 }}>{bv.toFixed(1)}{unit}</td>
                <td style={{ textAlign: "right", color: "#e2e8f0", paddingRight: 8 }}>{av.toFixed(1)}{unit}</td>
                <td style={{ textAlign: "right", color, fontWeight: 700 }}>{delta >= 0 ? "+" : ""}{delta.toFixed(1)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function ChecksList({ checks }) {
  if (!checks || checks.length === 0) return null;
  return (
    <div style={{ marginBottom: 14 }}>
      <div style={{ fontSize: "0.62rem", color: "#64748b", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 700, marginBottom: 8 }}>VERIFICATION CHECKS</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
        {checks.map((c, i) => (
          <div key={i} style={{ display: "flex", alignItems: "flex-start", gap: 8, padding: "7px 10px", borderRadius: 7, background: "rgba(255,255,255,0.02)", border: "1px solid rgba(90,123,186,0.10)" }}>
            <span style={{ fontSize: "0.65rem", fontWeight: 700, color: STATUS_COLOR[c.status] || "#94a3b8", minWidth: 48, flexShrink: 0, textTransform: "uppercase", marginTop: 1 }}>{c.status}</span>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: "0.75rem", color: "#e2e8f0", fontWeight: 600 }}>{c.name.replace(/_/g, " ")}</div>
              {c.detail && <div style={{ fontSize: "0.68rem", color: "#64748b", marginTop: 2 }}>{c.detail}</div>}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ProofList({ items }) {
  if (!items || items.length === 0) return null;
  return (
    <div style={{ marginBottom: 14 }}>
      <div style={{ fontSize: "0.62rem", color: "#64748b", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 700, marginBottom: 8 }}>PROOF OF EFFECTIVENESS</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
        {items.map((p, i) => (
          <div key={i} style={{ display: "flex", alignItems: "center", gap: 10, padding: "7px 12px", borderRadius: 8, background: "rgba(255,255,255,0.025)", border: `1px solid ${PROOF_STATUS_COLOR[p.status] || "#94a3b8"}22` }}>
            <span style={{ fontSize: "0.9rem", flexShrink: 0 }}>{PROOF_ICON[p.type] || "📄"}</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: "0.75rem", color: "#e2e8f0", fontWeight: 600 }}>{p.label}</div>
              <div style={{ fontSize: "0.67rem", color: "#475569", marginTop: 1 }}>
                {p.threshold > 0 && <span style={{ marginRight: 6 }}>Before: {p.threshold.toFixed(1)}{p.unit}</span>}
                After: {p.value.toFixed(1)}{p.unit}
              </div>
            </div>
            <span style={{ fontSize: "0.62rem", fontWeight: 700, color: PROOF_STATUS_COLOR[p.status] || "#94a3b8", textTransform: "uppercase" }}>{p.status}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function AddProofForm({ vrID, onProofAdded }) {
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [value, setValue] = useState("");
  const [unit, setUnit] = useState("");
  const [type, setType] = useState("manual");
  const [loading, setLoading] = useState(false);
  const [formError, setFormError] = useState("");

  const incidentID = vrID ? vrID.split("-").slice(0, 2).join("-") : "";

  async function submit(e) {
    e.preventDefault();
    if (!label.trim() || !vrID) return;
    setLoading(true);
    setFormError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/verify/${vrID}/proof`, {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({ type, label: label.trim(), value: parseFloat(value) || 0, unit: unit.trim(), status: "improved" }),
      });
      if (!res.ok) throw new Error("Failed to attach proof");
      const data = await res.json();
      onProofAdded(data);
      setLabel(""); setValue(""); setUnit(""); setOpen(false);
    } catch (err) {
      setFormError(err.message);
    } finally {
      setLoading(false);
    }
  }

  if (!open) {
    return (
      <button onClick={() => setOpen(true)} style={{ fontSize: "0.68rem", padding: "4px 10px", borderRadius: 6, cursor: "pointer", background: "transparent", color: "#64748b", border: "1px solid rgba(90,123,186,0.25)", marginBottom: 8 }}>
        + Attach proof
      </button>
    );
  }

  return (
    <form onSubmit={submit} style={{ marginBottom: 12, padding: "10px 12px", borderRadius: 9, background: "rgba(255,255,255,0.03)", border: "1px solid rgba(90,123,186,0.18)" }}>
      <div style={{ fontSize: "0.62rem", color: "#64748b", marginBottom: 8, textTransform: "uppercase", letterSpacing: "0.07em", fontWeight: 700 }}>ATTACH PROOF ITEM</div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
        <input value={label} onChange={e => setLabel(e.target.value)} placeholder="Label (e.g. 'Error rate dropped')" required style={{ flex: 2, minWidth: 160, padding: "4px 8px", borderRadius: 5, background: "rgba(255,255,255,0.05)", color: "#e2e8f0", border: "1px solid rgba(90,123,186,0.25)", fontSize: "0.75rem" }} />
        <input value={value} onChange={e => setValue(e.target.value)} placeholder="Value" type="number" step="any" style={{ width: 80, padding: "4px 8px", borderRadius: 5, background: "rgba(255,255,255,0.05)", color: "#e2e8f0", border: "1px solid rgba(90,123,186,0.25)", fontSize: "0.75rem" }} />
        <input value={unit} onChange={e => setUnit(e.target.value)} placeholder="Unit" style={{ width: 60, padding: "4px 8px", borderRadius: 5, background: "rgba(255,255,255,0.05)", color: "#e2e8f0", border: "1px solid rgba(90,123,186,0.25)", fontSize: "0.75rem" }} />
        <select value={type} onChange={e => setType(e.target.value)} style={{ padding: "4px 8px", borderRadius: 5, background: "#0d1117", color: "#94a3b8", border: "1px solid rgba(90,123,186,0.25)", fontSize: "0.75rem" }}>
          <option value="manual">Manual</option>
          <option value="metric_drop">Metric drop</option>
          <option value="error_clear">Error clear</option>
          <option value="alert_resolved">Alert resolved</option>
        </select>
      </div>
      {formError && <div className="error-text" style={{ marginTop: 6, fontSize: "0.7rem" }}>{formError}</div>}
      <div style={{ display: "flex", gap: 6, marginTop: 8 }}>
        <button type="submit" disabled={loading} style={{ padding: "4px 12px", borderRadius: 5, cursor: "pointer", background: "rgba(52,211,153,0.12)", color: "#34d399", border: "1px solid rgba(52,211,153,0.28)", fontSize: "0.72rem", fontWeight: 700 }}>{loading ? "Saving…" : "Save"}</button>
        <button type="button" onClick={() => setOpen(false)} style={{ padding: "4px 10px", borderRadius: 5, cursor: "pointer", background: "transparent", color: "#64748b", border: "1px solid rgba(90,123,186,0.2)", fontSize: "0.72rem" }}>Cancel</button>
      </div>
    </form>
  );
}

function VerificationRecordCard({ record, incidentID, onUpdate }) {
  const [rolling, setRolling] = useState(false);
  const [completing, setCompleting] = useState(false);
  const [cardError, setCardError] = useState("");
  const [rollbackConfirm, setRollbackConfirm] = useState(false);

  async function completeVerification() {
    setCompleting(true);
    setCardError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/verify/complete`, {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({ vrid: record.id }),
      });
      if (!res.ok) throw new Error("Complete failed");
      onUpdate(await res.json());
    } catch (err) {
      setCardError(err.message);
    } finally {
      setCompleting(false);
    }
  }

  async function doRollback() {
    setRolling(true);
    setRollbackConfirm(false);
    setCardError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/verify/rollback`, {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({ vrid: record.id }),
      });
      if (!res.ok) throw new Error("Rollback failed");
      onUpdate(await res.json());
    } catch (err) {
      setCardError(err.message);
    } finally {
      setRolling(false);
    }
  }

  const resultColor = RESULT_COLOR[record.result] || "#94a3b8";

  return (
    <div style={{ padding: "14px 16px", borderRadius: 12, background: "rgba(5,13,29,0.6)", border: `1px solid ${resultColor}22`, marginBottom: 14 }}>
      {/* Header row */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
        <span style={{ fontSize: "0.62rem", fontWeight: 700, padding: "2px 9px", borderRadius: 5, background: `${resultColor}18`, color: resultColor, border: `1px solid ${resultColor}33`, textTransform: "uppercase", letterSpacing: "0.06em" }}>
          {record.result.replace(/_/g, " ")}
        </span>
        <span style={{ fontSize: "0.62rem", color: "#475569" }}>{record.strategy}</span>
        {record.auto_close_eligible && (
          <span style={{ fontSize: "0.62rem", fontWeight: 700, padding: "2px 8px", borderRadius: 5, background: "rgba(52,211,153,0.10)", color: "#34d399", border: "1px solid rgba(52,211,153,0.28)" }}>
            AUTO-CLOSE ELIGIBLE
          </span>
        )}
        {record.rollback_triggered && (
          <span style={{ fontSize: "0.62rem", fontWeight: 700, padding: "2px 8px", borderRadius: 5, background: "rgba(167,139,250,0.10)", color: "#a78bfa", border: "1px solid rgba(167,139,250,0.28)" }}>
            ROLLED BACK
          </span>
        )}
        <span style={{ marginLeft: "auto", fontSize: "0.63rem", color: "#475569" }}>
          {record.verified_at ? new Date(record.verified_at).toLocaleString() : new Date(record.created_at).toLocaleString()}
        </span>
      </div>

      {/* Confidence delta */}
      {(record.confidence_before > 0 || record.confidence_after > 0) && (
        <ConfidenceDelta before={record.confidence_before} after={record.confidence_after} />
      )}

      {/* Before / after snapshot table */}
      <SnapshotComparison before={record.before_snapshot} after={record.after_snapshot} />

      {/* Checks */}
      <ChecksList checks={record.checks} />

      {/* Proof items */}
      <ProofList items={record.proof_items} />

      {/* Proof attachment form */}
      {!record.rollback_triggered && (
        <AddProofForm vrID={record.id} onProofAdded={onUpdate} />
      )}

      {cardError && <div className="error-text" style={{ marginTop: 6, fontSize: "0.7rem" }}>{cardError}</div>}

      {/* Action buttons */}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 4, alignItems: "center" }}>
        {record.result === "pending" && (
          <button onClick={completeVerification} disabled={completing} style={{ padding: "5px 14px", borderRadius: 6, cursor: "pointer", background: "rgba(58,167,255,0.12)", color: "#3aa7ff", border: "1px solid rgba(58,167,255,0.28)", fontSize: "0.72rem", fontWeight: 700 }}>
            {completing ? "Running checks…" : "Complete verification"}
          </button>
        )}
        {!record.rollback_triggered && record.result !== "pending" && !rollbackConfirm && (
          <button onClick={() => setRollbackConfirm(true)} disabled={rolling} style={{ padding: "5px 14px", borderRadius: 6, cursor: "pointer", background: "rgba(248,113,113,0.08)", color: "#f87171", border: "1px solid rgba(248,113,113,0.22)", fontSize: "0.72rem", fontWeight: 700 }}>
            Trigger rollback
          </button>
        )}
        {rollbackConfirm && (
          <>
            <span style={{ fontSize: "0.7rem", color: "#f87171" }}>Confirm rollback?</span>
            <button onClick={doRollback} disabled={rolling} style={{ padding: "4px 12px", borderRadius: 6, cursor: "pointer", background: "rgba(248,113,113,0.18)", color: "#f87171", border: "1px solid rgba(248,113,113,0.35)", fontSize: "0.72rem", fontWeight: 700 }}>
              {rolling ? "Rolling back…" : "Yes, rollback"}
            </button>
            <button onClick={() => setRollbackConfirm(false)} style={{ padding: "4px 10px", borderRadius: 6, cursor: "pointer", background: "transparent", color: "#64748b", border: "1px solid rgba(90,123,186,0.2)", fontSize: "0.72rem" }}>
              Cancel
            </button>
          </>
        )}
      </div>
    </div>
  );
}

// ─── Main export ──────────────────────────────────────────────────────────────

export default function VerificationPanel({ incidentID }) {
  const [records, setRecords] = useState(null);
  const [loading, setLoading] = useState(false);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!incidentID) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/verification`, { headers: authHeaders() });
      if (!res.ok) throw new Error("Failed to load verification records");
      const data = await res.json();
      setRecords(data.records || []);
      setLoaded(true);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [incidentID]);

  // Load on first render when the panel mounts.
  if (!loaded && !loading && !error && incidentID) {
    load();
  }

  async function startVerification() {
    if (!incidentID) return;
    setStarting(true);
    setError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/verify/start`, {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({ execution_id: "" }),
      });
      if (!res.ok) throw new Error("Failed to start verification");
      const record = await res.json();
      setRecords(prev => [record, ...(prev || [])]);
    } catch (err) {
      setError(err.message);
    } finally {
      setStarting(false);
    }
  }

  function handleUpdate(updatedRecord) {
    setRecords(prev => prev ? prev.map(r => r.id === updatedRecord.id ? updatedRecord : r) : [updatedRecord]);
  }

  return (
    <div className="panel">
      <div className="panel-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h3>Action Verification</h3>
        <div style={{ display: "flex", gap: 8 }}>
          <button onClick={load} disabled={loading} style={{ fontSize: "0.65rem", padding: "3px 10px", borderRadius: 6, cursor: "pointer", background: "transparent", color: "#64748b", border: "1px solid rgba(90,123,186,0.22)" }}>
            {loading ? "Loading…" : "Refresh"}
          </button>
          <button onClick={startVerification} disabled={starting} style={{ fontSize: "0.65rem", padding: "3px 10px", borderRadius: 6, cursor: "pointer", background: "rgba(52,211,153,0.10)", color: "#34d399", border: "1px solid rgba(52,211,153,0.28)", fontWeight: 700 }}>
            {starting ? "Starting…" : "Start verification"}
          </button>
        </div>
      </div>

      {error && <div className="error-text" style={{ marginTop: 8 }}>{error}</div>}

      {loading && <div className="empty-state">Loading verification records…</div>}

      {!loading && records && records.length === 0 && (
        <div className="empty-state">
          No verification records yet. Click "Start verification" before running a remediation action to capture a before-snapshot. After the action completes, click "Complete verification" to run checks and compare before/after.
        </div>
      )}

      {records && records.map(record => (
        <VerificationRecordCard
          key={record.id}
          record={record}
          incidentID={incidentID}
          onUpdate={handleUpdate}
        />
      ))}
    </div>
  );
}
