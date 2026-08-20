import { useEffect, useState } from "react";
import { getRiskExposure } from "../api/phase3.js";

function KpiCard({ label, value, unit, criticalColor, highlight }) {
  const borderAlpha = highlight ? "0.32" : "0.14";
  const bgAlpha = highlight ? "0.14" : "0.04";
  const rgb = criticalColor || "58,167,255";
  return (
    <div style={{
      padding: "14px 16px", borderRadius: 12,
      background: `rgba(${rgb},${bgAlpha})`,
      border: `1px solid rgba(${rgb},${borderAlpha})`,
    }}>
      <div style={{ fontSize: "0.67rem", color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.1em", marginBottom: 5 }}>
        {label}
      </div>
      <div style={{
        fontSize: "1.65rem", fontWeight: 800, lineHeight: 1,
        color: highlight ? `rgb(${rgb})` : "#edf4ff",
      }}>
        {value}
      </div>
      {unit && <div style={{ fontSize: "0.69rem", color: "#64748b", marginTop: 3 }}>{unit}</div>}
    </div>
  );
}

function RiskScoreBar({ score }) {
  const s = Math.min(100, Math.max(0, score || 0));
  const color = s >= 70 ? "#f87171" : s >= 40 ? "#f59e0b" : "#34d399";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <div style={{ flex: 1, height: 4, borderRadius: 2, background: "rgba(255,255,255,0.08)" }}>
        <div style={{ width: `${s}%`, height: "100%", borderRadius: 2, background: color }} />
      </div>
      <span style={{ fontSize: "0.7rem", color, fontWeight: 700, minWidth: 26 }}>{s}</span>
    </div>
  );
}

function TrendBars({ trend }) {
  if (!trend || trend.length === 0) return null;
  const maxOpen = Math.max(...trend.map((p) => p.open || 0), 1);
  return (
    <div style={{
      padding: "16px", borderRadius: 12,
      background: "rgba(255,255,255,0.03)", border: "1px solid rgba(90,123,186,0.14)",
      marginTop: 24,
    }}>
      <div className="lux-eyebrow" style={{ marginBottom: 14 }}>7-DAY EXPOSURE TREND</div>
      <div style={{ display: "flex", alignItems: "flex-end", gap: 6, height: 88 }}>
        {trend.map((point) => {
          const pct = point.open / maxOpen;
          const barH = Math.max(4, Math.round(pct * 76));
          return (
            <div key={point.date} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 4 }}>
              <div style={{ fontSize: "0.62rem", color: "#9eb5da", fontWeight: 600 }}>{point.open || 0}</div>
              <div style={{ width: "100%", height: barH, borderRadius: "4px 4px 0 0", background: "rgba(58,167,255,0.38)" }} />
              <div style={{ fontSize: "0.56rem", color: "#64748b" }}>
                {point.date ? point.date.slice(5) : ""}
              </div>
            </div>
          );
        })}
      </div>
      <div style={{ display: "flex", gap: 16, marginTop: 10, fontSize: "0.68rem", color: "#64748b" }}>
        <span style={{ color: "#3aa7ff" }}>■</span> Open count per day
        <span>New: {trend.reduce((a, p) => a + (p.new || 0), 0)} total</span>
        <span>Resolved: {trend.reduce((a, p) => a + (p.resolved || 0), 0)} total</span>
      </div>
    </div>
  );
}

function AtRiskServiceRow({ svc }) {
  const fmtMin = (m) => {
    if (!m || m <= 0) return "-";
    if (m < 60) return `${m.toFixed(0)}m`;
    return `${(m / 60).toFixed(1)}h`;
  };
  const sevColor = svc.max_severity === "critical" ? "#f87171"
    : svc.max_severity === "high" ? "#fdba74"
    : svc.max_severity === "medium" ? "#fcd34d"
    : "#94a3b8";
  const sevBg = svc.max_severity === "critical" ? "rgba(248,113,113,0.12)"
    : svc.max_severity === "high" ? "rgba(251,146,60,0.12)"
    : svc.max_severity === "medium" ? "rgba(245,158,11,0.10)"
    : "rgba(148,163,184,0.10)";
  return (
    <div style={{
      padding: "12px 16px", borderRadius: 12,
      background: "rgba(255,255,255,0.04)", border: "1px solid rgba(90,123,186,0.14)",
      display: "grid", gridTemplateColumns: "1fr auto auto 130px", gap: 14, alignItems: "center",
    }}>
      <div>
        <div style={{ fontWeight: 700, fontSize: "0.88rem", marginBottom: 2 }}>{svc.service}</div>
        <div style={{ fontSize: "0.68rem", color: "#64748b" }}>
          {svc.open_incidents} open incident{svc.open_incidents !== 1 ? "s" : ""}
          {svc.incident_count_7d > 0 ? ` · ${svc.incident_count_7d} in 7d` : ""}
          {svc.is_customer_facing ? " · Customer-facing" : ""}
        </div>
      </div>
      <span style={{
        padding: "3px 10px", borderRadius: 20, fontSize: "0.7rem", fontWeight: 700,
        background: sevBg, color: sevColor, textTransform: "uppercase",
      }}>
        {svc.max_severity || "-"}
      </span>
      <span style={{ fontSize: "0.7rem", color: "#9eb5da", whiteSpace: "nowrap" }}>
        {svc.avg_ttr_minutes > 0 ? `MTTR ${fmtMin(svc.avg_ttr_minutes)}` : "No TTR data"}
      </span>
      <RiskScoreBar score={svc.risk_score} />
    </div>
  );
}

export default function RiskExposureDashboard() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try { setData(await getRiskExposure()); }
    catch (err) { setError(err.message || "Failed to load risk exposure"); }
    finally { setLoading(false); }
  }

  useEffect(() => { load(); }, []);

  if (loading) return <div className="lux-muted" style={{ padding: "1.5rem" }}>Computing risk exposure…</div>;
  if (error) return (
    <div style={{ padding: "1rem" }}>
      <div style={{ color: "#fca5a5", marginBottom: 10 }}>{error}</div>
      <button className="lux-secondary-btn small" onClick={load}>Retry</button>
    </div>
  );
  if (!data) return <div className="lux-muted">No risk data available.</div>;

  const fmtUSD = (v) => {
    if (!v) return "$0";
    if (v >= 1_000_000) return `$${(v / 1_000_000).toFixed(1)}M`;
    if (v >= 1_000) return `$${(v / 1_000).toFixed(1)}k`;
    return `$${v.toFixed(0)}`;
  };
  const fmtMTTR = (m) => {
    if (!m || m <= 0) return "-";
    if (m < 60) return `${m.toFixed(0)}m`;
    return `${(m / 60).toFixed(1)}h`;
  };

  const services = [...(data.at_risk_services || [])].sort((a, b) => (b.risk_score || 0) - (a.risk_score || 0));

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">BUSINESS RISK EXPOSURE</div>
          <h3>Live Risk &amp; Blast Radius Dashboard</h3>
          <div className="lux-action-subtext">
            Computed {data.computed_at ? new Date(data.computed_at).toLocaleString() : "-"}
          </div>
        </div>
        <button className="lux-secondary-btn small" onClick={load}>Refresh</button>
      </div>

      {/* KPI cards */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))", gap: 12, marginBottom: 24 }}>
        <KpiCard label="Critical Open"     value={data.critical_count || 0}        criticalColor="248,113,113" highlight={(data.critical_count || 0) > 0} />
        <KpiCard label="Unacked Critical"  value={data.unacked_critical || 0}       criticalColor="248,113,113" highlight={(data.unacked_critical || 0) > 0} />
        <KpiCard label="High Open"         value={data.high_count || 0}             criticalColor="251,146,60"  highlight={(data.high_count || 0) > 0} />
        <KpiCard label="Medium Open"       value={data.medium_count || 0}           criticalColor="245,158,11" />
        <KpiCard label="Total Open"        value={data.total_open_incidents || 0}   criticalColor="58,167,255" />
        <KpiCard label="Blast Radius"      value={data.total_blast_radius || 0}     unit="services" criticalColor="167,139,250" />
        <KpiCard label="Est. Impact"       value={fmtUSD(data.estimated_impact_usd)} criticalColor="245,158,11" highlight={(data.estimated_impact_usd || 0) > 0} />
        <KpiCard label="7d Avg MTTR"       value={fmtMTTR(data.mttr_trend_minutes)} criticalColor="52,211,153" />
      </div>

      {/* 7-day trend */}
      <TrendBars trend={data.exposure_trend} />

      {/* At-risk services */}
      {services.length > 0 ? (
        <div style={{ marginTop: 24 }}>
          <div className="lux-eyebrow" style={{ marginBottom: 12 }}>AT-RISK SERVICES ({services.length})</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {services.map((svc) => <AtRiskServiceRow key={svc.service} svc={svc} />)}
          </div>
        </div>
      ) : (
        <div style={{
          padding: "28px", borderRadius: 12, textAlign: "center", marginTop: 24,
          background: "rgba(52,211,153,0.06)", border: "1px dashed rgba(52,211,153,0.22)",
          color: "#6ee7b7", fontSize: "0.85rem",
        }}>
          No at-risk services. All clear!
        </div>
      )}
    </div>
  );
}
