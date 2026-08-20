// SLODashboard.jsx — Phase 3, Week 7
// SLO tracking: define SLOs per service, error budget burn rate, time-to-breach.

import { useState, useEffect, useCallback } from "react";
import { getSLOs, createSLO, deleteSLO } from "../api/phase3.js";

const STATUS_COLORS = {
  healthy:  { bg: "var(--green-dim)", border: "rgba(0,208,132,0.4)", text: "var(--green)" },
  warning:  { bg: "var(--amber-dim)", border: "rgba(255,188,0,0.4)",  text: "var(--amber)" },
  critical: { bg: "var(--red-dim)",   border: "rgba(255,59,59,0.4)",  text: "var(--red)" },
  breached: { bg: "var(--red-dim)",   border: "rgba(255,59,59,0.55)", text: "var(--red)" },
};

function BudgetBar({ remaining }) {
  const pct = Math.max(0, Math.min(100, remaining));
  const color = pct > 25 ? "#00D084" : pct > 10 ? "#FFBC00" : "#FF3B3B";
  return (
    <div className="slo-budget-bar">
      <div className="slo-budget-fill" style={{ width: `${pct}%`, background: color }} />
      <span className="slo-budget-label">{pct.toFixed(1)}%</span>
    </div>
  );
}

function SLOCard({ slo, onDelete }) {
  const def = slo.definition || {};
  const sc = STATUS_COLORS[slo.status] || STATUS_COLORS.healthy;
  return (
    <div className="slo-card" style={{ borderColor: sc.border }}>
      <div className="slo-card-header">
        <div>
          <div className="slo-name">{def.name || "Unnamed SLO"}</div>
          <div className="slo-service">{def.service} · {def.metric_type} · {def.window_days}d window</div>
        </div>
        <span className="slo-status-badge" style={{ background: sc.bg, color: sc.text, borderColor: sc.border }}>
          {(slo.status || "unknown").toUpperCase()}
        </span>
      </div>

      <div className="slo-metrics-grid">
        <div className="slo-metric">
          <div className="slo-metric-label">Current</div>
          <div className="slo-metric-value">{(slo.current_percent || 0).toFixed(3)}%</div>
        </div>
        <div className="slo-metric">
          <div className="slo-metric-label">Target</div>
          <div className="slo-metric-value">{(def.target_percent || 99.9).toFixed(1)}%</div>
        </div>
        <div className="slo-metric">
          <div className="slo-metric-label">Burn Rate</div>
          <div className="slo-metric-value">{(slo.burn_rate || 0).toFixed(2)}x</div>
        </div>
        <div className="slo-metric">
          <div className="slo-metric-label">Time to Breach</div>
          <div className="slo-metric-value">
            {slo.time_to_breach_hours < 0 ? "∞" : `${slo.time_to_breach_hours.toFixed(0)}h`}
          </div>
        </div>
      </div>

      <div className="slo-budget-section">
        <div className="slo-metric-label">Error Budget Remaining</div>
        <BudgetBar remaining={slo.error_budget_remaining || 0} />
      </div>

      {def.description && <div className="slo-desc">{def.description}</div>}

      <div style={{ display: "flex", justifyContent: "flex-end", marginTop: "0.5rem" }}>
        <button className="lux-secondary-btn small" onClick={() => onDelete(def.id)}>Delete</button>
      </div>
    </div>
  );
}

export default function SLODashboard() {
  const [slos, setSlos] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState({ service: "", name: "", description: "", target_percent: 99.9, window_days: 30, metric_type: "availability" });

  const load = useCallback(async () => {
    try {
      const data = await getSLOs();
      setSlos(Array.isArray(data) ? data : []);
    } catch { setSlos([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCreate(e) {
    e.preventDefault();
    setCreateError("");
    try {
      await createSLO({ ...form, target_percent: parseFloat(form.target_percent), window_days: parseInt(form.window_days) });
      setShowForm(false);
      setForm({ service: "", name: "", description: "", target_percent: 99.9, window_days: 30, metric_type: "availability" });
      load();
    } catch (err) { setCreateError("Failed: " + err.message); }
  }

  async function handleDelete(id) {
    try { await deleteSLO(id); load(); } catch { /* ignore — list refreshes on retry */ }
  }


  const healthy = slos.filter(s => s.status === "healthy").length;
  const warning = slos.filter(s => s.status === "warning").length;
  const critical = slos.filter(s => s.status === "critical" || s.status === "breached").length;

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">SLO TRACKING</div>
          <h2 style={{ margin: "0.25rem 0" }}>Service Level Objectives</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            Define SLOs per service. Track error budget burn rate. Get time-to-SLA-breach projections.
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
          <button className="lux-primary-btn" onClick={() => setShowForm(!showForm)}>
            {showForm ? "Cancel" : "+ Define SLO"}
          </button>
        </div>
      </div>

      {/* KPI strip */}
      <div className="p3-kpi-strip">
        <div className="p3-kpi"><div className="p3-kpi-val">{slos.length}</div><div className="p3-kpi-label">Total SLOs</div></div>
        <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#00D084" }}>{healthy}</div><div className="p3-kpi-label">Healthy</div></div>
        <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#FFBC00" }}>{warning}</div><div className="p3-kpi-label">Warning</div></div>
        <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#FF3B3B" }}>{critical}</div><div className="p3-kpi-label">Critical</div></div>
      </div>

      {/* Create form */}
      {showForm && (
        <form className="p3-form" onSubmit={handleCreate}>
          <div className="p3-form-grid">
            <label>
              <span className="pm-label">Service</span>
              <input className="pm-input" value={form.service} onChange={e => setForm({ ...form, service: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">SLO Name</span>
              <input className="pm-input" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">Target %</span>
              <input className="pm-input" type="number" step="0.01" value={form.target_percent} onChange={e => setForm({ ...form, target_percent: e.target.value })} />
            </label>
            <label>
              <span className="pm-label">Window (days)</span>
              <input className="pm-input" type="number" value={form.window_days} onChange={e => setForm({ ...form, window_days: e.target.value })} />
            </label>
            <label>
              <span className="pm-label">Metric Type</span>
              <select className="pm-input" value={form.metric_type} onChange={e => setForm({ ...form, metric_type: e.target.value })}>
                <option value="availability">Availability</option>
                <option value="latency">Latency</option>
                <option value="error_rate">Error Rate</option>
              </select>
            </label>
            <label>
              <span className="pm-label">Description</span>
              <input className="pm-input" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} />
            </label>
          </div>
          {createError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{createError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>Create SLO</button>
        </form>
      )}

      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading SLOs...</div>
      ) : slos.length === 0 ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>📊</div>
          <p>No SLOs defined yet. Click "+ Define SLO" to create your first one.</p>
        </div>
      ) : (
        <div className="slo-grid">
          {slos.map(slo => (
            <SLOCard key={slo.definition?.id} slo={slo} onDelete={handleDelete} />
          ))}
        </div>
      )}
    </div>
  );
}
