// StatusPageView.jsx — SaaS Feature 6: Public Status Page
// Two export modes:
//   default export  → full standalone public page (public /status route)
//   EmbeddedStatus  → compact view for the dashboard "Status" tab

import { useState, useEffect, useCallback } from "react";
import { getStatus, subscribeToStatus } from "../api/integrations.js";

// ── Constants ─────────────────────────────────────────────────────────────────

const OVERALL_CONFIG = {
  operational: { label: "All Systems Operational", color: "#10b981", icon: "✅", bg: "rgba(16,185,129,0.08)", border: "rgba(16,185,129,0.3)" },
  degraded:    { label: "Partial System Degradation", color: "#f59e0b", icon: "⚠️", bg: "rgba(245,158,11,0.08)", border: "rgba(245,158,11,0.3)" },
  outage:      { label: "Major Outage in Progress", color: "#ef4444", icon: "🔴", bg: "rgba(239,68,68,0.08)", border: "rgba(239,68,68,0.3)" },
};

const SEV_COLOR = {
  critical: "#ef4444",
  high:     "#f97316",
  medium:   "#f59e0b",
  low:      "#6b7280",
};

const SVC_DOT = {
  operational: "#10b981",
  degraded:    "#f59e0b",
  outage:      "#ef4444",
};

// ── Sub-components ────────────────────────────────────────────────────────────

function OverallBanner({ status, updatedAt }) {
  const cfg = OVERALL_CONFIG[status] || OVERALL_CONFIG.operational;
  return (
    <div style={{
      background: cfg.bg,
      border: `1px solid ${cfg.border}`,
      borderRadius: "16px",
      padding: "1.75rem 2rem",
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      gap: "1rem",
      flexWrap: "wrap",
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: "1.25rem" }}>
        <span style={{ fontSize: "2.5rem", lineHeight: 1 }}>{cfg.icon}</span>
        <div>
          <div style={{ fontSize: "1.35rem", fontWeight: 700, color: cfg.color }}>{cfg.label}</div>
          <div style={{ fontSize: "0.78rem", color: "var(--muted,#6b7280)", marginTop: "0.3rem" }}>
            Updated {updatedAt ? new Date(updatedAt).toLocaleString() : "just now"}
          </div>
        </div>
      </div>
    </div>
  );
}

function KPIBar({ summary }) {
  const uptime = summary?.uptime_90d ?? 100;
  const mttr   = summary?.mttr_minutes ?? 0;
  const count  = summary?.resolved_last_30 ?? 0;
  const uptimeColor = uptime >= 99.5 ? "#10b981" : uptime >= 95 ? "#f59e0b" : "#ef4444";
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(3,1fr)", gap: "1rem" }}>
      {[
        { label: "90-Day Uptime", value: `${uptime.toFixed(2)}%`, color: uptimeColor },
        { label: "Avg Time to Resolve", value: mttr > 0 ? `${mttr} min` : "—", color: "#818cf8" },
        { label: "Incidents Resolved (30d)", value: count.toString(), color: "#06b6d4" },
      ].map(kpi => (
        <div key={kpi.label} className="sp-kpi-card">
          <div style={{ fontSize: "1.75rem", fontWeight: 700, color: kpi.color }}>{kpi.value}</div>
          <div className="lux-muted" style={{ fontSize: "0.78rem", marginTop: "0.25rem" }}>{kpi.label}</div>
        </div>
      ))}
    </div>
  );
}

function UptimeBar({ pct }) {
  const color = pct >= 99.5 ? "#10b981" : pct >= 95 ? "#f59e0b" : "#ef4444";
  return (
    <div style={{ background: "rgba(255,255,255,0.06)", borderRadius: "4px", height: "6px", width: "100%", overflow: "hidden" }}>
      <div style={{ background: color, height: "100%", width: `${Math.min(pct, 100)}%`, borderRadius: "4px", transition: "width 0.4s ease" }} />
    </div>
  );
}

function ServicesGrid({ services }) {
  if (!services || services.length === 0) {
    return (
      <div className="lux-muted" style={{ padding: "1rem 0", fontSize: "0.85rem" }}>
        No services tracked yet — incidents will populate this grid automatically.
      </div>
    );
  }
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))", gap: "0.75rem" }}>
      {services.map(svc => {
        const dotColor = SVC_DOT[svc.status] || "#10b981";
        const uptime = svc.uptime_90d ?? 100;
        return (
          <div key={svc.name} className="sp-service-card">
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "0.6rem" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "0.6rem" }}>
                <span style={{ color: dotColor, fontSize: "0.9rem" }}>●</span>
                <span style={{ fontWeight: 600, fontSize: "0.9rem" }}>{svc.name}</span>
              </div>
              <span style={{ fontSize: "0.72rem", fontWeight: 600, color: dotColor, textTransform: "uppercase", letterSpacing: "0.04em" }}>
                {svc.status}
              </span>
            </div>
            <UptimeBar pct={uptime} />
            <div style={{ display: "flex", justifyContent: "space-between", marginTop: "0.4rem" }}>
              <span className="lux-muted" style={{ fontSize: "0.72rem" }}>90d uptime</span>
              <span style={{ fontSize: "0.72rem", fontWeight: 600, color: uptime >= 99.5 ? "#10b981" : uptime >= 95 ? "#f59e0b" : "#ef4444" }}>
                {uptime.toFixed(2)}%
              </span>
            </div>
            {svc.active_incidents > 0 && (
              <div style={{ fontSize: "0.72rem", color: "#f97316", marginTop: "0.3rem" }}>
                {svc.active_incidents} active incident{svc.active_incidents > 1 ? "s" : ""}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ActiveIncidentRow({ inc }) {
  const color = SEV_COLOR[inc.severity] || "#6b7280";
  return (
    <div className="sp-incident-row">
      <div style={{ display: "flex", alignItems: "flex-start", gap: "0.75rem", flex: 1 }}>
        <span style={{ color, fontSize: "0.9rem", marginTop: "3px" }}>●</span>
        <div>
          <div style={{ fontWeight: 600, fontSize: "0.9rem" }}>{inc.title}</div>
          <div className="lux-muted" style={{ fontSize: "0.78rem", marginTop: "0.2rem" }}>
            {inc.service} · {inc.severity?.toUpperCase()} · Started {inc.started_at ? new Date(inc.started_at).toLocaleString() : "—"}
          </div>
        </div>
      </div>
      <span className="lux-mini-chip" style={{ color, borderColor: color, flexShrink: 0 }}>
        {inc.status?.toUpperCase()}
      </span>
    </div>
  );
}

function HistoryRow({ entry }) {
  const color = SEV_COLOR[entry.severity] || "#6b7280";
  const dur = entry.duration_minutes > 0
    ? entry.duration_minutes >= 60
      ? `${Math.floor(entry.duration_minutes / 60)}h ${entry.duration_minutes % 60}m`
      : `${entry.duration_minutes}m`
    : "<1m";
  return (
    <div className="sp-history-row">
      <div style={{ display: "flex", alignItems: "flex-start", gap: "0.6rem", flex: 1 }}>
        <span style={{ color, fontSize: "0.85rem", marginTop: "2px" }}>◉</span>
        <div>
          <div style={{ fontWeight: 600, fontSize: "0.85rem" }}>{entry.title}</div>
          <div className="lux-muted" style={{ fontSize: "0.75rem", marginTop: "0.15rem" }}>
            {entry.service} · Resolved {entry.resolved_at ? new Date(entry.resolved_at).toLocaleDateString() : "—"}
          </div>
        </div>
      </div>
      <div style={{ textAlign: "right", flexShrink: 0 }}>
        <div style={{ fontSize: "0.78rem", fontWeight: 600, color: "#10b981" }}>Resolved</div>
        <div className="lux-muted" style={{ fontSize: "0.72rem" }}>Duration: {dur}</div>
      </div>
    </div>
  );
}

function SubscribePanel({ tenant }) {
  const [channel, setChannel] = useState("email");
  const [target, setTarget] = useState("");
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  async function handleSubscribe(e) {
    e.preventDefault();
    if (!target.trim()) {
      setErr("Please enter a valid target.");
      return;
    }
    setLoading(true);
    setMsg("");
    setErr("");
    try {
      const res = await subscribeToStatus(channel, target.trim(), tenant);
      setMsg(res.message || "Subscribed successfully!");
      setTarget("");
    } catch (ex) {
      setErr("Subscription failed: " + ex.message);
    } finally {
      setLoading(false);
    }
  }

  const rssURL = `${window.location.origin}/api/v1/status${tenant && tenant !== "default" ? `/${tenant}` : ""}`;

  return (
    <div className="sp-subscribe-panel">
      <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>SUBSCRIBE TO UPDATES</div>
      <p className="lux-muted" style={{ fontSize: "0.82rem", margin: "0 0 1rem" }}>
        Get notified when we create or resolve incidents. Email and Slack supported.
      </p>

      {msg && <div className="sp-subscribe-success">{msg}</div>}
      {err && <div className="pm-error">{err}</div>}

      <form onSubmit={handleSubscribe} style={{ display: "flex", gap: "0.75rem", flexWrap: "wrap", alignItems: "flex-end" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.3rem" }}>
          <label style={{ fontSize: "0.75rem", color: "var(--muted,#6b7280)" }}>Channel</label>
          <select
            value={channel}
            onChange={e => setChannel(e.target.value)}
            className="lux-select"
            style={{ minWidth: "110px" }}
          >
            <option value="email">Email</option>
            <option value="slack">Slack</option>
          </select>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.3rem", flex: 1, minWidth: "220px" }}>
          <label style={{ fontSize: "0.75rem", color: "var(--muted,#6b7280)" }}>
            {channel === "email" ? "Email address" : "Slack webhook URL"}
          </label>
          <input
            type={channel === "email" ? "email" : "url"}
            value={target}
            onChange={e => setTarget(e.target.value)}
            placeholder={channel === "email" ? "ops@yourcompany.com" : "https://hooks.slack.com/..."}
            className="lux-input"
            style={{ minWidth: "220px" }}
          />
        </div>
        <button type="submit" className="lux-btn" disabled={loading} style={{ height: "36px" }}>
          {loading ? "Subscribing..." : "Subscribe"}
        </button>
      </form>

      <div style={{ marginTop: "1rem", fontSize: "0.78rem", color: "var(--muted,#6b7280)" }}>
        RSS / JSON feed:{" "}
        <code style={{ background: "rgba(0,0,0,0.15)", padding: "1px 5px", borderRadius: "3px" }}>
          GET {rssURL}
        </code>
        {" — No auth required. Poll every 60 s from any uptime monitor."}
      </div>
    </div>
  );
}

// ── Section wrapper ───────────────────────────────────────────────────────────

function Section({ eyebrow, title, children }) {
  return (
    <div style={{ marginTop: "2rem" }}>
      <div className="lux-section-head" style={{ padding: 0, marginBottom: "1rem" }}>
        <div>
          <div className="lux-eyebrow">{eyebrow}</div>
          <h3 style={{ margin: "0.2rem 0 0", fontSize: "1.05rem" }}>{title}</h3>
        </div>
      </div>
      {children}
    </div>
  );
}

// ── Core data-loading hook ────────────────────────────────────────────────────

function useStatusData(tenant) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshAt, setRefreshAt] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await getStatus(tenant === "default" ? "" : tenant);
      setData(result);
      setRefreshAt(new Date());
    } catch (e) {
      setError("Unable to load status: " + e.message);
    } finally {
      setLoading(false);
    }
  }, [tenant]);

  useEffect(() => {
    load();
    const timer = setInterval(load, 30000);
    return () => clearInterval(timer);
  }, [load]);

  return { data, loading, error, refreshAt, reload: load };
}

// ── Full page body (shared between standalone and embedded) ───────────────────

function StatusBody({ tenant, standalone = false }) {
  const { data, loading, error, refreshAt, reload } = useStatusData(tenant);

  const overallStatus  = data?.status || "operational";
  const services       = data?.services || [];
  const activeInc      = data?.active_incidents || [];
  const history        = data?.history || [];
  const summary        = data?.uptime_summary || {};

  return (
    <div className={standalone ? "sp-standalone-body" : "status-root"}>
      {/* Header row */}
      <div className={standalone ? "sp-page-header" : "status-header"}>
        <div>
          {!standalone && <div className="lux-eyebrow">LIVE STATUS PAGE</div>}
          {!standalone && <h2 style={{ margin: "0.25rem 0 0" }}>System Status</h2>}
          <div className="lux-muted" style={{ fontSize: "0.78rem", marginTop: "0.2rem" }}>
            Tenant: <code>{tenant}</code> · Auto-refresh every 30 s
            {refreshAt && ` · Last updated ${refreshAt.toLocaleTimeString()}`}
          </div>
        </div>
        <button className="lux-secondary-btn small" onClick={reload} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </button>
      </div>

      {error && <div className="pm-error">{error}</div>}
      {loading && !data && <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading status…</div>}

      {data && (
        <>
          {/* 1. Overall status banner */}
          <OverallBanner status={overallStatus} updatedAt={data.updated_at} />

          {/* 2. KPI headline row */}
          <Section eyebrow="PLATFORM HEALTH" title="90-Day Performance">
            <KPIBar summary={summary} />
          </Section>

          {/* 3. Per-service grid */}
          <Section eyebrow="SERVICES" title={`${services.length} Monitored Service${services.length === 1 ? "" : "s"}`}>
            <ServicesGrid services={services} />
          </Section>

          {/* 4. Active incidents */}
          <Section
            eyebrow="ACTIVE INCIDENTS"
            title={activeInc.length === 0 ? "No active incidents" : `${activeInc.length} Active Incident${activeInc.length === 1 ? "" : "s"}`}
          >
            {activeInc.length === 0 ? (
              <div className="status-no-incidents">
                <span style={{ fontSize: "1.5rem" }}>✅</span>
                <p>All systems are operating normally. No active incidents.</p>
              </div>
            ) : (
              <div className="sp-incident-list">
                {activeInc.map(inc => <ActiveIncidentRow key={inc.id} inc={inc} />)}
              </div>
            )}
          </Section>

          {/* 5. Incident history */}
          <Section eyebrow="PAST INCIDENTS" title={`Last ${history.length} Resolved`}>
            {history.length === 0 ? (
              <div className="lux-muted" style={{ fontSize: "0.85rem", padding: "0.5rem 0" }}>No resolved incidents on record yet.</div>
            ) : (
              <div className="sp-history-list">
                {history.map(e => <HistoryRow key={e.id} entry={e} />)}
              </div>
            )}
          </Section>

          {/* 6. Subscribe panel */}
          <Section eyebrow="NOTIFICATIONS" title="Subscribe to Updates">
            <SubscribePanel tenant={tenant} />
          </Section>
        </>
      )}
    </div>
  );
}

// ── Standalone public page (used by App.jsx for /status route) ───────────────

export function PublicStatusPage({ tenant = "default" }) {
  return (
    <div className="sp-public-root">
      <header className="sp-public-header">
        <div className="sp-public-header-inner">
          <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
            <div style={{
              width: "36px", height: "36px", borderRadius: "10px",
              background: "linear-gradient(135deg,#818cf8,#4f46e5)",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontSize: "1.1rem",
            }}>
              ⚡
            </div>
            <div>
              <div style={{ fontWeight: 700, fontSize: "1.1rem" }}>AIOps Platform</div>
              <div className="lux-muted" style={{ fontSize: "0.72rem" }}>Status & Reliability</div>
            </div>
          </div>
          <a href="/" className="lux-secondary-btn small" style={{ textDecoration: "none" }}>
            ← Dashboard
          </a>
        </div>
      </header>
      <main className="sp-public-main">
        <h1 style={{ fontSize: "1.6rem", fontWeight: 700, margin: "0 0 1.5rem" }}>System Status</h1>
        <StatusBody tenant={tenant} standalone />
      </main>
    </div>
  );
}

// ── Embedded dashboard view (default export, used in IncidentCommandCenter) ──

export default function StatusPageView({ tenant = "default" }) {
  return <StatusBody tenant={tenant} standalone={false} />;
}
