// ROIDashboardPanel.jsx — Phase 3, Week 9
// Hours saved, incidents auto-resolved, SRE cost avoided. Justifies renewal.

import { useState, useEffect, useCallback } from "react";
import { getROI } from "../api/phase3.js";

function ROICard({ label, value, unit, highlight }) {
  return (
    <div className={`roi-card ${highlight ? "roi-highlight" : ""}`}>
      <div className="roi-card-val">{value}</div>
      <div className="roi-card-unit">{unit}</div>
      <div className="roi-card-label">{label}</div>
    </div>
  );
}

export default function ROIDashboardPanel() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);

  const load = useCallback(async () => {
    setLoading(true);
    try { const d = await getROI(days); setData(d); }
    catch { setData(null); }
    finally { setLoading(false); }
  }, [days]);

  useEffect(() => { load(); }, [load]);

  const fmtUSD = (v) => {
    if (!v || v <= 0) return "$0";
    if (v >= 1000) return `$${(v / 1000).toFixed(1)}k`;
    return `$${v.toFixed(0)}`;
  };

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">RETURN ON INVESTMENT</div>
          <h2 style={{ margin: "0.25rem 0" }}>ROI Dashboard</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            Hours saved. Incidents auto-resolved. Engineer time recovered. SRE cost avoided. Justifies renewal.
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
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Calculating ROI...</div>
      ) : !data ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>💰</div>
          <p>No ROI data available yet. Metrics are recorded as incidents are resolved.</p>
        </div>
      ) : (
        <>
          {/* Big ROI number */}
          <div className="roi-hero">
            <div className="roi-hero-val">{fmtUSD(data.monthly_roi_usd)}</div>
            <div className="roi-hero-label">Monthly ROI</div>
            <div className="slo-service">Based on {data.total_incidents} incidents over {days} days</div>
          </div>

          {/* KPI grid */}
          <div className="roi-grid">
            <ROICard label="Total Incidents" value={data.total_incidents} unit="incidents" />
            <ROICard label="Auto-Resolved" value={data.auto_resolved} unit="incidents" highlight />
            <ROICard label="Hours Saved" value={data.hours_saved?.toFixed(1) || "0"} unit="hours" highlight />
            <ROICard label="Engineer Time Saved" value={data.engineer_time_saved_hours?.toFixed(1) || "0"} unit="hours" />
            <ROICard label="SRE Cost Avoided" value={fmtUSD(data.sre_cost_avoided_usd)} unit="" highlight />
            <ROICard label="MTTR Reduction" value={`${(data.mttr_reduction_percent || 0).toFixed(0)}%`} unit=""
              highlight={data.mttr_reduction_percent > 0} />
            <ROICard label="Cost per Incident" value={fmtUSD(data.cost_per_incident_usd)} unit="avg" />
            <ROICard label="Incident Reduction" value={`${(data.incident_reduction_percent || 0).toFixed(0)}%`} unit="" />
          </div>

          {/* Top wins */}
          {(data.top_wins || []).length > 0 && (
            <div className="roi-wins">
              <div className="lux-eyebrow" style={{ marginBottom: "0.75rem" }}>TOP WINS</div>
              <div className="roi-wins-list">
                {data.top_wins.map((w, i) => (
                  <div key={i} className="roi-win-card">
                    <div className="roi-win-top">
                      <strong>{w.label}</strong>
                      <span className="roi-win-val">{typeof w.value === "number" && w.value >= 1 ? fmtUSD(w.value) : w.value}</span>
                    </div>
                    <div className="slo-service">{w.description}</div>
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
