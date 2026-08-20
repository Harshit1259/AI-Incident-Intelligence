// AnomalyPanel.jsx — Phase 3, Week 8
// Pre-failure anomaly detection. Trend analysis on metrics.

import { useState, useEffect, useCallback } from "react";
import { getAnomalies, checkMetric, ackAnomaly, simulateAnomaly } from "../api/phase3.js";

const SEV_COLOR = { critical: "#ef4444", high: "#f97316", warning: "#f59e0b", medium: "#f59e0b", low: "#6b7280" };
const TREND_ICON = { rising: "📈", falling: "📉", spike: "⚡", stable: "—" };

function AnomalyCard({ alert, onAck }) {
  const color = SEV_COLOR[alert.severity] || "#6b7280";
  return (
    <div className="anomaly-card" style={{ borderLeftColor: color }}>
      <div className="anomaly-card-top">
        <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
          <span style={{ fontSize: "1.2rem" }}>{TREND_ICON[alert.trend] || "—"}</span>
          <div>
            <div className="slo-name">{alert.metric_name}</div>
            <div className="slo-service">{alert.service} · {alert.severity}</div>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
          {alert.acknowledged ? (
            <span className="lux-mini-chip" style={{ color: "#10b981" }}>ACK</span>
          ) : (
            <button className="lux-secondary-btn small" onClick={() => onAck(alert.id)}>Acknowledge</button>
          )}
        </div>
      </div>
      <div className="anomaly-message">{alert.message}</div>
      <div className="anomaly-metrics">
        <div className="anomaly-metric">
          <span className="slo-metric-label">Current</span>
          <span className="anomaly-val" style={{ color }}>{alert.current_value?.toFixed(1)}</span>
        </div>
        <div className="anomaly-metric">
          <span className="slo-metric-label">Threshold</span>
          <span className="anomaly-val">{alert.threshold?.toFixed(1)}</span>
        </div>
        <div className="anomaly-metric">
          <span className="slo-metric-label">Trend</span>
          <span className="anomaly-val">{alert.trend}</span>
        </div>
        <div className="anomaly-metric">
          <span className="slo-metric-label">Fired</span>
          <span className="anomaly-val">{alert.fired_at ? new Date(alert.fired_at).toLocaleTimeString() : "—"}</span>
        </div>
      </div>
    </div>
  );
}

export default function AnomalyPanel() {
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCheck, setShowCheck] = useState(false);
  const [checkError, setCheckError] = useState("");
  const [form, setForm] = useState({ service: "", metric_name: "db_connections", value: 87, threshold: 100, trend: "rising" });

  const load = useCallback(async () => {
    try {
      const data = await getAnomalies();
      setAlerts(Array.isArray(data) ? data : []);
    } catch { setAlerts([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleAck(id) {
    try { await ackAnomaly(id); load(); } catch { /* ignore — list refreshes on next poll */ }
  }

  async function handleCheck(e) {
    e.preventDefault();
    setCheckError("");
    try {
      await checkMetric({ ...form, value: parseFloat(form.value), threshold: parseFloat(form.threshold) });
      setShowCheck(false);
      load();
    } catch (err) { setCheckError("Failed: " + err.message); }
  }

  async function handleSimulate() {
    try {
      await simulateAnomaly(form.service || "payments-api");
      load();
    } catch { /* ignore — simulation errors are non-critical */ }
  }

  const active = alerts.filter(a => !a.acknowledged).length;

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">ANOMALY DETECTION</div>
          <h2 style={{ margin: "0.25rem 0" }}>Pre-Failure Alerts</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            Trend analysis on metrics. "DB connections at 87% and growing" alerts before user impact.
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
          <button className="lux-secondary-btn" onClick={handleSimulate}>Simulate Anomaly</button>
          <button className="lux-primary-btn" onClick={() => setShowCheck(!showCheck)}>
            {showCheck ? "Cancel" : "Check Metric"}
          </button>
        </div>
      </div>

      <div className="p3-kpi-strip">
        <div className="p3-kpi"><div className="p3-kpi-val">{alerts.length}</div><div className="p3-kpi-label">Total Alerts</div></div>
        <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#ef4444" }}>{active}</div><div className="p3-kpi-label">Active</div></div>
        <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#10b981" }}>{alerts.length - active}</div><div className="p3-kpi-label">Acknowledged</div></div>
      </div>

      {showCheck && (
        <form className="p3-form" onSubmit={handleCheck}>
          <div className="p3-form-grid">
            <label><span className="pm-label">Service</span><input className="pm-input" value={form.service} onChange={e => setForm({ ...form, service: e.target.value })} required /></label>
            <label><span className="pm-label">Metric</span><input className="pm-input" value={form.metric_name} onChange={e => setForm({ ...form, metric_name: e.target.value })} /></label>
            <label><span className="pm-label">Current Value</span><input className="pm-input" type="number" step="0.1" value={form.value} onChange={e => setForm({ ...form, value: e.target.value })} /></label>
            <label><span className="pm-label">Threshold</span><input className="pm-input" type="number" step="0.1" value={form.threshold} onChange={e => setForm({ ...form, threshold: e.target.value })} /></label>
            <label>
              <span className="pm-label">Trend</span>
              <select className="pm-input" value={form.trend} onChange={e => setForm({ ...form, trend: e.target.value })}>
                <option value="rising">Rising</option><option value="falling">Falling</option><option value="spike">Spike</option><option value="stable">Stable</option>
              </select>
            </label>
          </div>
          {checkError && (
            <div style={{ marginTop: "0.5rem", fontSize: "0.82rem", color: "#fca5a5" }}>{checkError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>Submit Check</button>
        </form>
      )}

      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading anomalies...</div>
      ) : alerts.length === 0 ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>✅</div>
          <p>No anomaly alerts. All metrics within normal ranges.</p>
          <button className="lux-secondary-btn" onClick={handleSimulate}>Simulate one for demo</button>
        </div>
      ) : (
        <div className="anomaly-list">
          {alerts.map(a => <AnomalyCard key={a.id} alert={a} onAck={handleAck} />)}
        </div>
      )}
    </div>
  );
}
