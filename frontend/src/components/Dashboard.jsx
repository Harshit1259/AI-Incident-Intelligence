/**
 * NeuroOps — Command Center Dashboard
 * All data sourced from live API. No mocks, no fallback fabrication.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity, AlertTriangle, ArrowRight, Bot, Brain,
  CheckCircle2, ChevronRight, Clock, Cpu, Database,
  Flame, Gauge, Globe, HardDrive, RefreshCw,
  Server, Shield, Sparkles, TrendingDown, TrendingUp,
  XCircle, Zap,
} from "lucide-react";

async function api(path, token) {
  try {
    const r = await fetch(`/api/v1${path}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    if (!r.ok) return null;
    return await r.json();
  } catch { return null; }
}

function fmtAge(iso) {
  if (!iso) return "—";
  const s = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (s < 60)    return `${s}s ago`;
  if (s < 3600)  return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function fmtUptime(s) {
  if (!s) return "—";
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
  return h < 24 ? `${h}h ${m}m` : `${Math.floor(h / 24)}d ${h % 24}h`;
}

function deriveStats(items) {
  const all = Array.isArray(items) ? items : [];
  const open         = all.filter(i => i.status === "open").length;
  const acknowledged = all.filter(i => i.status === "acknowledged").length;
  const resolved     = all.filter(i => i.status === "resolved").length;
  const critical     = all.filter(i => i.severity === "critical").length;
  const high         = all.filter(i => i.severity === "high").length;
  const medium       = all.filter(i => i.severity === "medium").length;
  const low          = all.filter(i => i.severity === "low").length;
  let mttrSum = 0, mttrCount = 0;
  for (const i of all) {
    if (i.status === "resolved" && i.first_event_time && i.last_event_time) {
      const d = new Date(i.last_event_time) - new Date(i.first_event_time);
      if (d > 0) { mttrSum += d; mttrCount++; }
    }
  }
  const mttrMs = mttrCount ? mttrSum / mttrCount : null;
  const mttrStr = mttrMs
    ? mttrMs < 3_600_000 ? `${Math.round(mttrMs / 60000)}m` : `${(mttrMs / 3_600_000).toFixed(1)}h`
    : "—";
  const changeLinked = all.filter(i => i.has_what_changed).length;
  return { total: all.length, open, acknowledged, resolved, critical, high, medium, low, mttrStr, changeLinked };
}

/* ── Sub-components ──────────────────────────────────────────────── */

function PulseDot({ color }) {
  return (
    <span style={{ position: "relative", display: "inline-flex", width: 9, height: 9, flexShrink: 0 }}>
      <span style={{
        position: "absolute", inset: 0, borderRadius: "50%", background: color, opacity: 0.4,
        animation: "pulse-ring 1.8s ease-out infinite",
      }} />
      <span style={{ width: 9, height: 9, borderRadius: "50%", background: color, display: "block" }} />
    </span>
  );
}

function KPI({ label, value, sub, icon, color, trend, onClick, loading }) {
  const Icon = icon;
  const palette = {
    danger:  { bg: "var(--red-dim)",    border: "rgba(255,59,59,0.18)",    text: "var(--red)",     bar: "var(--red)" },
    warning: { bg: "var(--amber-dim)",  border: "rgba(255,188,0,0.18)",    text: "var(--amber)",   bar: "var(--amber)" },
    success: { bg: "var(--green-dim)",  border: "rgba(0,208,132,0.18)",    text: "var(--green)",   bar: "var(--green)" },
    blue:    { bg: "var(--blue-dim)",   border: "rgba(0,102,255,0.18)",    text: "var(--blue-lt)", bar: "var(--blue)" },
    cyan:    { bg: "var(--cyan-dim)",   border: "rgba(0,200,232,0.18)",    text: "var(--cyan)",    bar: "var(--cyan)" },
    neutral: { bg: "rgba(255,255,255,0.03)", border: "var(--border)",      text: "var(--t2)",      bar: "var(--t3)" },
  };
  const c = palette[color] || palette.neutral;

  return (
    <div
      onClick={onClick}
      style={{
        background: c.bg, border: `1px solid ${c.border}`, borderRadius: 14,
        padding: "18px 20px", cursor: onClick ? "pointer" : "default",
        transition: "transform 0.15s, border-color 0.15s", position: "relative", overflow: "hidden",
      }}
      onMouseEnter={e => onClick && (e.currentTarget.style.transform = "translateY(-1px)")}
      onMouseLeave={e => onClick && (e.currentTarget.style.transform = "translateY(0)")}
    >
      <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: 2, background: c.bar, borderRadius: "14px 14px 0 0" }} />

      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 10 }}>
            {label}
          </div>
          {loading ? (
            <div className="skeleton" style={{ width: 56, height: 32 }} />
          ) : (
            <div style={{ fontSize: 30, fontWeight: 800, letterSpacing: "-0.04em", lineHeight: 1, color: c.text }}>
              {value ?? "—"}
            </div>
          )}
          {sub && !loading && (
            <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 7, display: "flex", alignItems: "center", gap: 4 }}>
              {trend === "up"   && <TrendingUp   size={10} color="var(--red)" />}
              {trend === "down" && <TrendingDown size={10} color="var(--green)" />}
              {sub}
            </div>
          )}
        </div>
        <div style={{
          width: 36, height: 36, borderRadius: 10, background: `${c.bar}22`,
          border: `1px solid ${c.border}`,
          display: "flex", alignItems: "center", justifyContent: "center", color: c.text, flexShrink: 0,
        }}>
          <Icon size={16} />
        </div>
      </div>
    </div>
  );
}

function Card({ children, style = {}, glow }) {
  return (
    <div style={{
      background: "var(--surface-1)",
      border: `1px solid ${glow ? "rgba(0,102,255,0.22)" : "var(--border)"}`,
      borderRadius: 16, padding: "20px 22px",
      boxShadow: glow ? "0 0 0 1px rgba(0,102,255,0.08), 0 8px 32px rgba(0,0,0,0.4)" : "var(--shadow-sm)",
      ...style,
    }}>
      {children}
    </div>
  );
}

function SH({ title, sub, action, onAction }) {
  return (
    <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 16 }}>
      <div>
        <div style={{ fontSize: 13, fontWeight: 700, color: "var(--t1)", letterSpacing: "-0.01em" }}>{title}</div>
        {sub && <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 2 }}>{sub}</div>}
      </div>
      {action && (
        <button onClick={onAction} style={{
          background: "none", border: "none", cursor: "pointer", fontSize: 11, color: "var(--blue-lt)",
          fontWeight: 600, display: "flex", alignItems: "center", gap: 3,
        }}>
          {action} <ChevronRight size={11} />
        </button>
      )}
    </div>
  );
}

function SevBadge({ s }) {
  const m = {
    critical: { bg: "var(--red-dim)",    color: "var(--red)" },
    high:     { bg: "var(--orange-dim)", color: "var(--orange)" },
    medium:   { bg: "var(--amber-dim)",  color: "var(--amber)" },
    low:      { bg: "var(--green-dim)",  color: "var(--green)" },
  };
  const c = m[s] || m.low;
  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: 5, padding: "2px 8px",
      borderRadius: 99, fontSize: 10, fontWeight: 700, letterSpacing: "0.06em",
      textTransform: "uppercase", background: c.bg, color: c.color, flexShrink: 0,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: "50%", background: c.color }} />
      {s}
    </span>
  );
}

function StatusBadge({ s }) {
  const m = {
    open:         { bg: "var(--red-dim)",   color: "var(--red)" },
    acknowledged: { bg: "var(--amber-dim)", color: "var(--amber)" },
    resolved:     { bg: "var(--green-dim)", color: "var(--green)" },
  };
  const c = m[s] || m.open;
  return (
    <span style={{ padding: "2px 7px", borderRadius: 99, fontSize: 10, fontWeight: 600, background: c.bg, color: c.color, textTransform: "uppercase" }}>
      {s || "open"}
    </span>
  );
}

function SeverityBars({ stats }) {
  const bars = [
    { label: "Critical", count: stats.critical, color: "var(--red)" },
    { label: "High",     count: stats.high,     color: "var(--orange)" },
    { label: "Medium",   count: stats.medium,   color: "var(--amber)" },
    { label: "Low",      count: stats.low,      color: "var(--green)" },
  ];
  const max = Math.max(...bars.map(b => b.count), 1);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {bars.map(b => (
        <div key={b.label} style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ width: 54, fontSize: 11, color: "var(--t3)", fontWeight: 500, textAlign: "right" }}>{b.label}</div>
          <div style={{ flex: 1, height: 6, background: "var(--surface-3)", borderRadius: 99, overflow: "hidden" }}>
            <div style={{
              height: "100%", width: `${(b.count / max) * 100}%`, background: b.color, borderRadius: 99,
              transition: "width 0.6s cubic-bezier(0.4,0,0.2,1)",
            }} />
          </div>
          <div style={{ width: 24, fontSize: 12, fontWeight: 700, color: b.count > 0 ? b.color : "var(--t4)", textAlign: "right" }}>{b.count}</div>
        </div>
      ))}
    </div>
  );
}

function StatusDonut({ open, acknowledged, resolved }) {
  const total = open + acknowledged + resolved || 1;
  const o = (open / total) * 360, a = (acknowledged / total) * 360, r = (resolved / total) * 360;
  const conic = `conic-gradient(
    var(--red) 0deg ${o}deg,
    var(--amber) ${o}deg ${o + a}deg,
    var(--green) ${o + a}deg ${o + a + r}deg,
    rgba(255,255,255,0.04) ${o + a + r}deg 360deg
  )`;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
      <div style={{ position: "relative", width: 88, height: 88, flexShrink: 0 }}>
        <div style={{ width: 88, height: 88, borderRadius: "50%", background: conic }} />
        <div style={{
          position: "absolute", top: "50%", left: "50%", transform: "translate(-50%,-50%)",
          width: 54, height: 54, borderRadius: "50%", background: "var(--surface-1)",
          display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
        }}>
          <div style={{ fontSize: 17, fontWeight: 800, color: "var(--t1)", lineHeight: 1 }}>{open + acknowledged}</div>
          <div style={{ fontSize: 8, color: "var(--t3)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", marginTop: 2 }}>active</div>
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
        {[
          { label: "Open",         count: open,         color: "var(--red)" },
          { label: "Acknowledged", count: acknowledged, color: "var(--amber)" },
          { label: "Resolved",     count: resolved,     color: "var(--green)" },
        ].map(row => (
          <div key={row.label} style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ width: 7, height: 7, borderRadius: "50%", background: row.color, flexShrink: 0 }} />
            <span style={{ fontSize: 11, color: "var(--t3)", minWidth: 80 }}>{row.label}</span>
            <span style={{ fontSize: 13, fontWeight: 700, color: "var(--t1)" }}>{row.count}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function SourceRow({ src }) {
  const ok = src.status === "healthy" && !src.stale;
  const dot = ok ? "var(--green)" : src.stale ? "var(--amber)" : "var(--red)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "9px 0", borderBottom: "1px solid var(--border)" }}>
      <span style={{ width: 7, height: 7, borderRadius: "50%", background: dot, flexShrink: 0 }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 12, fontWeight: 500, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {src.name || src.id}
        </div>
        <div style={{ fontSize: 10, color: "var(--t3)", marginTop: 1 }}>
          {src.type || "webhook"} · {fmtAge(src.last_event_at)}
        </div>
      </div>
      <span style={{
        fontSize: 10, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em",
        padding: "2px 7px", borderRadius: 99,
        background: ok ? "var(--green-dim)" : src.stale ? "var(--amber-dim)" : "var(--red-dim)",
        color: dot,
      }}>
        {src.stale ? "stale" : src.status || "unknown"}
      </span>
    </div>
  );
}

function IncidentRow({ inc, onClick }) {
  return (
    <button
      onClick={onClick}
      style={{
        width: "100%", display: "flex", alignItems: "center", gap: 10,
        padding: "10px 12px", borderRadius: 10, cursor: "pointer",
        background: "transparent", border: "1px solid transparent",
        textAlign: "left", transition: "background 0.14s, border-color 0.14s",
      }}
      onMouseEnter={e => {
        e.currentTarget.style.background = "var(--blue-dim)";
        e.currentTarget.style.borderColor = "rgba(0,102,255,0.18)";
      }}
      onMouseLeave={e => {
        e.currentTarget.style.background = "transparent";
        e.currentTarget.style.borderColor = "transparent";
      }}
    >
      <SevBadge s={inc.severity || "low"} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 12, fontWeight: 600, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {inc.title || `Incident #${inc.id}`}
        </div>
        <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 2 }}>
          {inc.service || "—"}
          {inc.seen_before && <span style={{ color: "var(--cyan)", marginLeft: 6 }}>· recurring</span>}
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 3, flexShrink: 0 }}>
        <StatusBadge s={inc.status} />
        <div style={{ fontSize: 10, color: "var(--t4)" }}>{fmtAge(inc.last_event_time)}</div>
      </div>
    </button>
  );
}

/* ── Main Dashboard ──────────────────────────────────────────────── */
export default function Dashboard({ token, onNavigate }) {
  const [incidents,   setIncidents]   = useState([]);
  const [platform,    setPlatform]    = useState(null);
  const [alertQ,      setAlertQ]      = useState(null);
  const [sources,     setSources]     = useState([]);
  const [aiStatus,    setAiStatus]    = useState(null);
  const [health,      setHealth]      = useState(null);
  const [loading,     setLoading]     = useState(true);
  const [lastRefresh, setLastRefresh] = useState(null);
  const [refreshing,  setRefreshing]  = useState(false);
  const timerRef = useRef(null);

  const load = useCallback(async () => {
    setRefreshing(true);
    const [incR, platR, aqR, srcR, aiR, hlR] = await Promise.allSettled([
      api("/incidents?page=1&page_size=100&sort_by=last_event_time&sort_order=desc", token),
      api("/platform/metrics", token),
      api("/alert-quality/report?window=30", token),
      api("/sources/health", token),
      api("/ai/status", null),
      api("/health", null),
    ]);

    if (incR.status === "fulfilled" && incR.value) {
      const d = incR.value;
      setIncidents(Array.isArray(d) ? d : d.items || d.incidents || d.data || []);
    }
    if (platR.status === "fulfilled" && platR.value) setPlatform(platR.value);
    if (aqR.status   === "fulfilled" && aqR.value)   setAlertQ(aqR.value);
    if (srcR.status  === "fulfilled" && srcR.value)  setSources(srcR.value.items || []);
    if (aiR.status   === "fulfilled" && aiR.value)   setAiStatus(aiR.value);
    if (hlR.status   === "fulfilled" && hlR.value)   setHealth(hlR.value);

    setLastRefresh(new Date());
    setLoading(false);
    setRefreshing(false);
  }, [token]);

  useEffect(() => {
    load();
    timerRef.current = setInterval(load, 30_000);
    return () => clearInterval(timerRef.current);
  }, [load]);

  const stats = deriveStats(incidents);
  const recent = incidents.slice(0, 8);
  const aqSum = alertQ?.summary || {};
  const srcHealthy = sources.filter(s => s.status === "healthy" && !s.stale).length;

  return (
    <div style={{ padding: "24px 28px", minHeight: "calc(100vh - var(--topbar-h))", animation: "fadeIn 0.22s ease" }}>

      {/* Header */}
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", marginBottom: 24, gap: 16 }}>
        <div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
            <PulseDot color={health?.status === "ok" ? "var(--green)" : "var(--red)"} />
            <span style={{ fontSize: 11, fontWeight: 600, letterSpacing: "0.10em", textTransform: "uppercase", color: "var(--t3)" }}>
              Platform {health?.status === "ok" ? "Online" : "Degraded"}
            </span>
            <span style={{ width: 1, height: 11, background: "var(--border-strong)", margin: "0 4px" }} />
            <span style={{ fontSize: 11, color: "var(--t4)" }}>
              {health?.editions?.join(" · ") || "Community"}
            </span>
          </div>
          <h1 style={{ fontSize: 26, fontWeight: 800, letterSpacing: "-0.04em", color: "var(--t1)", margin: 0, lineHeight: 1.2 }}>
            Command Center
          </h1>
          <div style={{ fontSize: 12, color: "var(--t4)", marginTop: 5 }}>
            Refreshed {lastRefresh ? lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "—"} · auto-refresh 30s
          </div>
        </div>

        <div style={{ display: "flex", gap: 8, flexShrink: 0 }}>
          <button
            onClick={load} disabled={refreshing}
            className="btn btn-ghost"
            style={{ gap: 6 }}
          >
            <RefreshCw size={13} style={{ animation: refreshing ? "rotateSpin 0.8s linear infinite" : "none" }} />
            Refresh
          </button>
          <button
            onClick={() => onNavigate("incidents")}
            className="btn btn-primary"
            style={stats.open > 0 ? { background: "var(--red)", borderColor: "var(--red)", boxShadow: "0 4px 16px rgba(255,59,59,0.3)" } : {}}
          >
            {stats.open > 0
              ? <><Flame size={13} /> {stats.open} Open Incidents</>
              : <><CheckCircle2 size={13} /> All Clear</>}
          </button>
        </div>
      </div>

      {/* Quick Start Banner — shown until first real incident arrives */}
      {!loading && stats.total === 0 && (
        <div style={{
          marginBottom: 20, padding: "18px 24px",
          background: "linear-gradient(135deg, rgba(0,102,255,0.08) 0%, rgba(0,200,232,0.06) 100%)",
          border: "1px solid rgba(0,102,255,0.2)", borderRadius: 14,
          display: "flex", alignItems: "center", justifyContent: "space-between", gap: 24, flexWrap: "wrap",
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <div style={{
              width: 44, height: 44, borderRadius: 12, flexShrink: 0,
              background: "linear-gradient(135deg, var(--blue) 0%, var(--cyan) 100%)",
              display: "flex", alignItems: "center", justifyContent: "center",
              boxShadow: "0 4px 16px var(--blue-glow)",
            }}>
              <Zap size={20} color="#fff" />
            </div>
            <div>
              <div style={{ fontSize: 15, fontWeight: 700, color: "var(--t1)", marginBottom: 3 }}>
                Connect your first data source
              </div>
              <div style={{ fontSize: 13, color: "var(--t3)", lineHeight: 1.5 }}>
                Paste a webhook URL into any monitoring tool — or fire a test alert in 1 click.
              </div>
            </div>
          </div>
          <div style={{ display: "flex", gap: 10, flexShrink: 0 }}>
            <button className="btn btn-primary" onClick={() => onNavigate("onboarding")} style={{ gap: 6 }}>
              <Sparkles size={13} /> Open Setup Guide
            </button>
            <button className="btn btn-ghost" onClick={() => onNavigate("integrations")} style={{ gap: 6 }}>
              View All Integrations <ArrowRight size={13} />
            </button>
          </div>
        </div>
      )}

      {/* KPI Row */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(6,1fr)", gap: 12, marginBottom: 20 }}>
        <KPI label="Open Incidents"  icon={Flame}          color={stats.open > 0 ? "danger" : "success"} value={loading ? null : stats.open}         sub="need response"              loading={loading} onClick={() => onNavigate("incidents")} />
        <KPI label="Critical"        icon={AlertTriangle}   color={stats.critical > 0 ? "danger" : "neutral"} value={loading ? null : stats.critical} sub="highest priority"           loading={loading} onClick={() => onNavigate("incidents")} />
        <KPI label="MTTR"            icon={Clock}           color="blue"    value={loading ? null : stats.mttrStr}                                       sub="mean time to resolve"       loading={loading} />
        <KPI label="Events Ingested" icon={Activity}        color="cyan"    value={loading ? null : platform?.events_ingested?.toLocaleString() ?? "—"} sub="platform total"             loading={loading} />
        <KPI label="Active Agents"   icon={Bot}             color="success" value={loading ? null : platform?.active_agents ?? "—"}                      sub="monitoring agents"          loading={loading} onClick={() => onNavigate("agents")} />
        <KPI label="Platform Uptime" icon={Gauge}           color="neutral" value={loading ? null : fmtUptime(platform?.uptime_seconds)}                 sub={platform ? `${platform.db_open_connections || 0} db` : "—"} loading={loading} />
      </div>

      {/* Row 2: Status + Severity + AI Engine */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 16, marginBottom: 16 }}>

        <Card>
          <SH title="Incident Status" sub="current snapshot" />
          {loading
            ? <div className="skeleton" style={{ height: 88 }} />
            : <StatusDonut open={stats.open} acknowledged={stats.acknowledged} resolved={stats.resolved} />
          }
          <div style={{ marginTop: 16, paddingTop: 14, borderTop: "1px solid var(--border)", display: "flex", gap: 12 }}>
            {[
              { label: "Total",        value: stats.total,       color: "var(--t1)" },
              { label: "Change-linked",value: stats.changeLinked, color: "var(--blue-lt)" },
              { label: "Recurring",    value: incidents.filter(i => i.seen_before).length, color: "var(--cyan)" },
            ].map(({ label, value, color }) => (
              <div key={label} style={{ flex: 1, textAlign: "center" }}>
                <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.07em" }}>{label}</div>
                <div style={{ fontSize: 20, fontWeight: 800, color, marginTop: 4 }}>{value}</div>
              </div>
            ))}
          </div>
        </Card>

        <Card>
          <SH title="Severity Breakdown" sub="across all incidents" />
          {loading
            ? <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                {[70,50,85,40].map((w,i) => <div key={i} className="skeleton" style={{ height: 6, width: `${w}%` }} />)}
              </div>
            : <SeverityBars stats={stats} />
          }
          <div style={{ marginTop: 16, paddingTop: 14, borderTop: "1px solid var(--border)" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <div style={{ fontSize: 11, color: "var(--t3)" }}>Priority score avg</div>
              <div style={{ fontSize: 13, fontWeight: 700, color: "var(--blue-lt)" }}>
                {incidents.length
                  ? Math.round(incidents.reduce((a, i) => a + (i.priority_score || 0), 0) / incidents.length)
                  : "—"}
              </div>
            </div>
            <div className="progress-track" style={{ marginTop: 6 }}>
              <div className="progress-fill" style={{
                width: `${Math.min(incidents.length ? Math.round(incidents.reduce((a,i)=>a+(i.priority_score||0),0)/incidents.length) : 0, 100)}%`,
                background: "linear-gradient(90deg, var(--blue), var(--cyan))",
              }} />
            </div>
          </div>
        </Card>

        <Card glow>
          <SH title="AI Engine" sub="real-time intelligence" />
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{
              display: "flex", alignItems: "center", gap: 10, padding: "11px 13px",
              borderRadius: 10,
              background: aiStatus?.configured ? "var(--blue-dim)" : "var(--red-dim)",
              border: `1px solid ${aiStatus?.configured ? "rgba(0,102,255,0.2)" : "rgba(255,59,59,0.2)"}`,
            }}>
              <div style={{
                width: 34, height: 34, borderRadius: 9,
                background: aiStatus?.configured ? "rgba(0,102,255,0.14)" : "var(--red-dim)",
                display: "flex", alignItems: "center", justifyContent: "center",
              }}>
                <Brain size={17} color={aiStatus?.configured ? "var(--blue-lt)" : "var(--red)"} />
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13, fontWeight: 600, color: "var(--t1)" }}>{aiStatus?.provider || "AI Engine"}</div>
                <div style={{ fontSize: 11, color: "var(--t3)" }}>{aiStatus?.configured ? `${aiStatus.data_mode || "live"} mode` : "not configured"}</div>
              </div>
              <PulseDot color={aiStatus?.configured ? "var(--blue-lt)" : "var(--red)"} />
            </div>

            {aiStatus && (
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                {[
                  { label: "Total Calls", value: aiStatus.total_calls ?? "—", accent: false },
                  { label: "Failed",      value: aiStatus.failed_calls ?? 0,  accent: (aiStatus.failed_calls || 0) > 0 },
                ].map(({ label, value, accent }) => (
                  <div key={label} style={{
                    padding: "9px 12px", borderRadius: 9,
                    background: accent ? "var(--red-dim)" : "var(--surface-2)",
                    border: `1px solid ${accent ? "rgba(255,59,59,0.15)" : "var(--border)"}`,
                  }}>
                    <div style={{ fontSize: 10, color: "var(--t3)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.07em" }}>{label}</div>
                    <div style={{ fontSize: 18, fontWeight: 700, color: accent ? "var(--red)" : "var(--t1)", marginTop: 3 }}>{value}</div>
                  </div>
                ))}
              </div>
            )}

            <div style={{
              display: "flex", alignItems: "center", gap: 8, padding: "8px 12px",
              borderRadius: 9, background: "var(--surface-2)", border: "1px solid var(--border)",
            }}>
              <Sparkles size={12} color="var(--cyan)" />
              <div style={{ fontSize: 11, color: "var(--t3)" }}>Last AI call</div>
              <div style={{ marginLeft: "auto", fontSize: 12, fontWeight: 600, color: "var(--t2)" }}>{fmtAge(aiStatus?.last_call_at)}</div>
            </div>
          </div>
        </Card>
      </div>

      {/* Row 3: Recent incidents + Platform internals */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 360px", gap: 16, marginBottom: 16 }}>

        <Card style={{ padding: 0 }}>
          <div style={{ padding: "18px 22px 14px", borderBottom: "1px solid var(--border)" }}>
            <SH title="Recent Incidents" sub={`${incidents.length} total · latest ${Math.min(recent.length, 8)}`} action="View All" onAction={() => onNavigate("incidents")} />
          </div>
          {loading
            ? <div style={{ padding: "14px 22px", display: "flex", flexDirection: "column", gap: 8 }}>
                {[1,2,3,4,5].map(i => <div key={i} className="skeleton" style={{ height: 56, borderRadius: 10 }} />)}
              </div>
            : recent.length === 0
            ? <div style={{ padding: "40px 22px", textAlign: "center" }}>
                <CheckCircle2 size={28} color="var(--green)" style={{ margin: "0 auto 10px" }} />
                <div style={{ fontSize: 14, fontWeight: 600, color: "var(--t2)" }}>All Clear</div>
                <div style={{ fontSize: 12, color: "var(--t3)", marginTop: 4 }}>No incidents in current dataset</div>
              </div>
            : <div style={{ padding: "6px 10px" }}>
                {recent.map(inc => <IncidentRow key={inc.id} inc={inc} onClick={() => onNavigate("incidents")} />)}
              </div>
          }
        </Card>

        <Card>
          <SH title="Platform Internals" sub="live metrics" />
          {loading
            ? <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {[1,2,3,4,5,6].map(i => <div key={i} className="skeleton" style={{ height: 32, borderRadius: 6 }} />)}
              </div>
            : <div style={{ display: "flex", flexDirection: "column" }}>
                {[
                  { label: "Total incidents",    value: platform?.total_incidents?.toLocaleString(),  icon: Flame,         color: "var(--blue-lt)" },
                  { label: "Total events",        value: platform?.total_events?.toLocaleString(),    icon: Activity,      color: "var(--cyan)" },
                  { label: "Log entries",         value: platform?.total_logs?.toLocaleString(),      icon: Database,      color: "var(--t2)" },
                  { label: "Goroutines",          value: platform?.goroutines,                        icon: Cpu,           color: "var(--t2)" },
                  { label: "Memory allocated",    value: platform?.memory_alloc_mb ? `${platform.memory_alloc_mb} MB` : "—", icon: HardDrive, color: "var(--green)" },
                  { label: "DB connections",      value: platform?.db_open_connections,               icon: Server,        color: "var(--amber)" },
                  { label: "Events dropped",      value: platform?.events_dropped,                   icon: XCircle,       color: (platform?.events_dropped || 0) > 0 ? "var(--red)" : "var(--t4)" },
                  { label: "Request errors",      value: platform?.request_errors,                   icon: AlertTriangle, color: (platform?.request_errors || 0) > 0 ? "var(--orange)" : "var(--t4)" },
                ].map(row => (
                  <div key={row.label} style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 0", borderBottom: "1px solid var(--border)" }}>
                    <row.icon size={12} color={row.color} />
                    <div style={{ flex: 1, fontSize: 12, color: "var(--t3)" }}>{row.label}</div>
                    <div style={{ fontSize: 12, fontWeight: 700, color: "var(--t2)" }}>{row.value ?? "—"}</div>
                  </div>
                ))}
              </div>
          }
          <div style={{ marginTop: 14, paddingTop: 12, borderTop: "1px solid var(--border)", textAlign: "center" }}>
            <span style={{ fontSize: 11, color: "var(--t4)" }}>
              Uptime · <span style={{ color: "var(--blue-lt)", fontWeight: 600 }}>{fmtUptime(platform?.uptime_seconds)}</span>
            </span>
          </div>
        </Card>
      </div>

      {/* Row 4: Alert Quality + Sources + Quick Access */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 16 }}>

        <Card>
          <SH title="Alert Quality" sub="last 30 days" action="Full Report" onAction={() => onNavigate("alerts")} />
          {loading
            ? <div className="skeleton" style={{ height: 120 }} />
            : alertQ
            ? <div>
                <div style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 14 }}>
                  <div style={{ position: "relative", width: 68, height: 68, flexShrink: 0 }}>
                    <svg width="68" height="68" viewBox="0 0 68 68">
                      <circle cx="34" cy="34" r="26" fill="none" stroke="var(--surface-2)" strokeWidth="8" />
                      <circle cx="34" cy="34" r="26" fill="none"
                        stroke={aqSum.estimated_noise_pct > 50 ? "var(--red)" : aqSum.estimated_noise_pct > 25 ? "var(--amber)" : "var(--green)"}
                        strokeWidth="8" strokeLinecap="round"
                        strokeDasharray={`${(1 - (aqSum.estimated_noise_pct || 0) / 100) * 163.4} 163.4`}
                        transform="rotate(-90 34 34)"
                      />
                    </svg>
                    <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>
                      <div style={{ fontSize: 16, fontWeight: 800, color: "var(--t1)", lineHeight: 1 }}>
                        {100 - (aqSum.estimated_noise_pct || 0)}
                      </div>
                      <div style={{ fontSize: 8, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.05em" }}>quality</div>
                    </div>
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                    <div style={{ fontSize: 11, color: "var(--t3)" }}>Estimated noise</div>
                    <div style={{ fontSize: 20, fontWeight: 700, color: (aqSum.estimated_noise_pct || 0) > 30 ? "var(--red)" : "var(--amber)" }}>
                      {aqSum.estimated_noise_pct || 0}%
                    </div>
                    <div style={{ fontSize: 11, color: "var(--t4)" }}>{aqSum.total_fired_30d || 0} alerts fired</div>
                  </div>
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                  {[
                    { label: "Noisy rules",  value: aqSum.noisy_count         || 0, warn: true },
                    { label: "Duplicates",   value: aqSum.duplicate_pair_count || 0, warn: true },
                    { label: "Stale rules",  value: aqSum.stale_count          || 0, warn: true },
                    { label: "Unique rules", value: aqSum.unique_rules          || 0, warn: false },
                  ].map(r => (
                    <div key={r.label} style={{
                      padding: "7px 10px", borderRadius: 8,
                      background: r.warn && r.value > 0 ? "var(--red-dim)" : "var(--surface-2)",
                      border: `1px solid ${r.warn && r.value > 0 ? "rgba(255,59,59,0.15)" : "var(--border)"}`,
                    }}>
                      <div style={{ fontSize: 10, color: "var(--t3)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.06em" }}>{r.label}</div>
                      <div style={{ fontSize: 17, fontWeight: 700, color: r.warn && r.value > 0 ? "var(--red)" : "var(--t2)", marginTop: 3 }}>{r.value}</div>
                    </div>
                  ))}
                </div>
              </div>
            : <div style={{ padding: "24px 0", textAlign: "center", fontSize: 12, color: "var(--t4)" }}>Alert quality data unavailable</div>
          }
        </Card>

        <Card>
          <SH title="Data Sources" sub={`${srcHealthy}/${sources.length} healthy`} action="Configure" onAction={() => onNavigate("integrations")} />
          {loading
            ? <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: 48, borderRadius: 8 }} />)}
              </div>
            : sources.length === 0
            ? <div style={{ textAlign: "center", padding: "28px 12px" }}>
                <Globe size={26} color="var(--t4)" style={{ margin: "0 auto 10px" }} />
                <div style={{ fontSize: 13, fontWeight: 600, color: "var(--t3)" }}>No sources configured</div>
                <button onClick={() => onNavigate("onboarding")} className="btn btn-outline btn-sm" style={{ marginTop: 12 }}>
                  <Zap size={11} /> Quick Setup
                </button>
              </div>
            : <div>{sources.slice(0, 5).map((s, i) => <SourceRow key={s.id || i} src={s} />)}</div>
          }
        </Card>

        <Card>
          <SH title="Quick Access" sub="jump to any module" />
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {[
              { label: "Incidents",      sub: `${stats.open} open`,               icon: Flame,     view: "incidents",   hot: stats.open > 0 },
              { label: "AI Engine",      sub: "provider, mode & usage",           icon: Brain,     view: "ai-engine",   hot: false },
              { label: "Topology Graph", sub: "service dependency map",           icon: Globe,     view: "topology",    hot: false },
              { label: "Alert Quality",  sub: `${aqSum.noisy_count || 0} noisy`,  icon: Shield,    view: "alerts",      hot: (aqSum.noisy_count || 0) > 0 },
              { label: "Log Explorer",   sub: `${platform?.total_logs?.toLocaleString() || 0} entries`, icon: Database, view: "logs", hot: false },
              { label: "Fleet Monitor",  sub: "all devices at a glance",          icon: Server,    view: "fleet",       hot: false },
              { label: "Agents",         sub: `${platform?.active_agents || 0} active`, icon: Bot, view: "agents",     hot: false },
              { label: "SLO Dashboard",  sub: "service level objectives",         icon: Gauge,     view: "slo",         hot: false },
            ].map(a => (
              <button
                key={a.view}
                onClick={() => onNavigate(a.view)}
                style={{
                  display: "flex", alignItems: "center", gap: 10, padding: "8px 10px",
                  borderRadius: 9, width: "100%", textAlign: "left", cursor: "pointer",
                  background: a.hot ? "var(--red-dim)" : "var(--surface-2)",
                  border: `1px solid ${a.hot ? "rgba(255,59,59,0.15)" : "var(--border)"}`,
                  transition: "background 0.14s, border-color 0.14s",
                }}
                onMouseEnter={e => {
                  e.currentTarget.style.background = "var(--blue-dim)";
                  e.currentTarget.style.borderColor = "rgba(0,102,255,0.2)";
                }}
                onMouseLeave={e => {
                  e.currentTarget.style.background = a.hot ? "var(--red-dim)" : "var(--surface-2)";
                  e.currentTarget.style.borderColor = a.hot ? "rgba(255,59,59,0.15)" : "var(--border)";
                }}
              >
                <div style={{
                  width: 28, height: 28, borderRadius: 7, flexShrink: 0,
                  background: a.hot ? "var(--red-dim)" : "var(--blue-dim)",
                  display: "flex", alignItems: "center", justifyContent: "center",
                }}>
                  <a.icon size={13} color={a.hot ? "var(--red)" : "var(--blue-lt)"} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 12, fontWeight: 600, color: "var(--t1)" }}>{a.label}</div>
                  <div style={{ fontSize: 10, color: "var(--t3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{a.sub}</div>
                </div>
                <ArrowRight size={11} color="var(--t4)" />
              </button>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
