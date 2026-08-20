// EngineeringHealthPanel.jsx — Phase 3, Week 8
// MTTR per team, toil %, on-call hours per engineer, burnout risk flag.

import { useState, useEffect, useCallback } from "react";
import { getEngineeringHealth } from "../api/phase3.js";

const RISK_COLOR = { high: "#ef4444", medium: "#f59e0b", low: "#10b981" };

function StatCard({ label, value, unit, color }) {
  return (
    <div className="eh-stat">
      <div className="eh-stat-val" style={color ? { color } : {}}>{value}</div>
      <div className="eh-stat-unit">{unit}</div>
      <div className="slo-metric-label">{label}</div>
    </div>
  );
}

export default function EngineeringHealthPanel() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);

  const load = useCallback(async () => {
    setLoading(true);
    try { const d = await getEngineeringHealth(days); setData(d); }
    catch { setData(null); }
    finally { setLoading(false); }
  }, [days]);

  useEffect(() => { load(); }, [load]);

  const fmtTime = (s) => {
    if (!s || s <= 0) return "0m";
    if (s < 60) return `${s.toFixed(0)}s`;
    if (s < 3600) return `${(s / 60).toFixed(0)}m`;
    return `${(s / 3600).toFixed(1)}h`;
  };

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">ENGINEERING HEALTH</div>
          <h2 style={{ margin: "0.25rem 0" }}>Team Performance Dashboard</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            MTTR per team. Toil %. On-call hours per engineer. Burnout risk flags.
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          <select className="pm-input" value={days} onChange={e => setDays(parseInt(e.target.value))} style={{ width: "auto" }}>
            <option value={7}>7 days</option><option value={14}>14 days</option><option value={30}>30 days</option><option value={90}>90 days</option>
          </select>
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
        </div>
      </div>

      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading health data...</div>
      ) : !data ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>📊</div>
          <p>No engineering health data available yet. Incident metrics are recorded as incidents are resolved.</p>
        </div>
      ) : (
        <>
          {/* Top-level KPIs */}
          <div className="eh-kpi-grid">
            <StatCard label="Total Incidents" value={data.total_incidents} unit="incidents" />
            <StatCard label="Mean Time to Resolve" value={fmtTime(data.mttr_seconds)} unit="MTTR" color={data.mttr_seconds > 3600 ? "#ef4444" : "#10b981"} />
            <StatCard label="Mean Time to Acknowledge" value={fmtTime(data.mtta_seconds)} unit="MTTA" />
            <StatCard label="Mean Time to Detect" value={fmtTime(data.mttd_seconds)} unit="MTTD" />
            <StatCard label="Auto-Resolved" value={data.auto_resolved} unit="incidents" color="#10b981" />
            <StatCard label="Total Toil" value={data.total_toil_hours?.toFixed(1) || "0"} unit="hours" color={data.total_toil_hours > 40 ? "#ef4444" : undefined} />
          </div>

          {/* Burnout Risk */}
          {(data.burnout_risk || []).length > 0 && (
            <div className="eh-section">
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>BURNOUT RISK FLAGS</div>
              <div className="eh-burnout-list">
                {data.burnout_risk.map((flag, i) => (
                  <div key={i} className="eh-burnout-card" style={{ borderLeftColor: RISK_COLOR[flag.risk_level] || "#6b7280" }}>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <strong>{flag.responder}</strong>
                      <span className="lux-mini-chip" style={{ color: RISK_COLOR[flag.risk_level] }}>{flag.risk_level?.toUpperCase()} RISK</span>
                    </div>
                    <div className="slo-service">{flag.reason}</div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Top Responders */}
          {(data.top_responders || []).length > 0 && (
            <div className="eh-section">
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>TOP RESPONDERS</div>
              <div className="eh-table">
                <div className="eh-table-head">
                  <span>Responder</span><span>Incidents</span><span>Avg TTR</span><span>Toil Hours</span><span>On-Call Hours</span>
                </div>
                {data.top_responders.map((r, i) => (
                  <div key={i} className="eh-table-row">
                    <span className="eh-name">{r.responder}</span>
                    <span>{r.incident_count}</span>
                    <span>{fmtTime(r.avg_ttr_seconds)}</span>
                    <span>{r.toil_hours?.toFixed(1) || "0"}h</span>
                    <span>{r.on_call_hours?.toFixed(0) || "0"}h</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Team Health */}
          {(data.by_team || []).length > 0 && (
            <div className="eh-section">
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>BY TEAM</div>
              <div className="eh-table">
                <div className="eh-table-head">
                  <span>Team</span><span>Incidents</span><span>MTTR</span><span>Toil Hours</span>
                </div>
                {data.by_team.map((t, i) => (
                  <div key={i} className="eh-table-row">
                    <span className="eh-name">{t.team_name}</span>
                    <span>{t.incident_count}</span>
                    <span>{fmtTime(t.mttr_seconds)}</span>
                    <span>{t.toil_hours?.toFixed(1) || "0"}h</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
