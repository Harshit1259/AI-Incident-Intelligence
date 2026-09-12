/**
 * Public status page.
 *
 * Two export modes:
 *   PublicStatusPage → standalone page served at /status (and /status/{tenant})
 *   default export   → embedded view inside the operator dashboard
 *
 * This page has a different audience from the rest of the product. A visitor
 * arrives anxious and asks one question — "is it down?" — often on a phone,
 * often while something else of theirs is already broken. So:
 *
 *   • The answer is the largest thing on the page and needs no interpretation.
 *   • Active incidents are promoted directly under the banner. They were the
 *     fourth section before, below uptime stats and the service grid, which is
 *     backwards: the incident is why the visitor is here.
 *   • No emoji. ✅ ⚠️ 🔴 ⚡ rendered differently on every platform and read as
 *     unserious on a page whose entire job is to look maintained and credible.
 *   • Services render as a list, not a grid — a status list is scanned top to
 *     bottom for the one name you care about.
 *   • Past incidents group by month, the convention every status page follows.
 */
import { useState, useEffect, useCallback, useMemo } from "react";
import {
  AlertTriangle, Bell, Check, CheckCircle2, ChevronLeft, Clock,
  RefreshCw, ShieldAlert, Wifi,
} from "lucide-react";

import { getStatus, subscribeToStatus } from "../api/integrations.js";

/* ── Status vocabulary ─────────────────────────────────────────────────────
   Three states, each with an icon and a tone class. Nothing here names a
   colour; `sp-*` tone classes resolve to design-system tokens.              */
const OVERALL = {
  operational: { label: "All systems operational", icon: CheckCircle2, tone: "sp-ok" },
  degraded: { label: "Partial system degradation", icon: AlertTriangle, tone: "sp-warn" },
  outage: { label: "Major outage in progress", icon: ShieldAlert, tone: "sp-down" },
};

const SERVICE_TONE = {
  operational: "sp-ok",
  degraded: "sp-warn",
  outage: "sp-down",
};

const SEVERITY_TONE = {
  critical: "sp-down",
  high: "sp-warn",
  medium: "sp-warn",
  low: "sp-muted",
};

/* Uptime bands. 99.5% is the usual "three nines and a half" expectation; below
   95% a service is not meaningfully available. */
function uptimeTone(pct) {
  if (pct >= 99.5) return "sp-ok";
  if (pct >= 95) return "sp-warn";
  return "sp-down";
}

function fmtDuration(minutes) {
  if (!minutes || minutes <= 0) return "under a minute";
  if (minutes < 60) return `${minutes} min`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h}h ${m}m` : `${h}h`;
}

function fmtDateTime(iso) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
  });
}

/* How long an incident has been running — what a visitor actually wants to
   know about an open incident, more than when it started. */
function elapsedSince(iso) {
  if (!iso) return "";
  const mins = Math.floor((Date.now() - new Date(iso).getTime()) / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

/* ── Overall banner ────────────────────────────────────────────────────────*/
function OverallBanner({ status, updatedAt }) {
  const cfg = OVERALL[status] || OVERALL.operational;
  const Icon = cfg.icon;
  return (
    <section className={`sp-banner ${cfg.tone}`} aria-live="polite">
      <span className="sp-banner-icon" aria-hidden="true">
        {Icon && <Icon size={28} />}
      </span>
      <div className="sp-banner-text">
        <h2 className="sp-banner-title">{cfg.label}</h2>
        <p className="sp-banner-meta">
          Last checked {updatedAt ? fmtDateTime(updatedAt) : "just now"}
        </p>
      </div>
    </section>
  );
}

/* ── Uptime summary ────────────────────────────────────────────────────────*/
function UptimeSummary({ summary }) {
  const uptime = summary?.uptime_90d ?? 100;
  const mttr = summary?.mttr_minutes ?? 0;
  const resolved = summary?.resolved_last_30 ?? 0;

  const stats = [
    { label: "90-day uptime", value: `${uptime.toFixed(2)}%`, tone: uptimeTone(uptime) },
    { label: "Average time to resolve", value: mttr > 0 ? fmtDuration(mttr) : "—", tone: "sp-info" },
    { label: "Incidents resolved (30d)", value: String(resolved), tone: "sp-info" },
  ];

  return (
    <div className="sp-stat-row">
      {stats.map((s) => (
        <div key={s.label} className={`sp-stat ${s.tone}`}>
          <span className="sp-stat-value">{s.value}</span>
          <span className="sp-stat-label">{s.label}</span>
        </div>
      ))}
    </div>
  );
}

/* ── Services ──────────────────────────────────────────────────────────────*/
function ServiceRow({ svc }) {
  const tone = SERVICE_TONE[svc.status] || "sp-ok";
  const uptime = svc.uptime_90d ?? 100;

  return (
    <li className={`sp-service ${tone}`}>
      <div className="sp-service-head">
        <span className="sp-service-name">
          <span className="sp-dot" aria-hidden="true" />
          {svc.name}
        </span>
        <span className="sp-service-status">{svc.status}</span>
      </div>

      <div className={`sp-uptime ${uptimeTone(uptime)}`}>
        <div className="sp-uptime-track">
          <div className="sp-uptime-fill" style={{ width: `${Math.min(uptime, 100)}%` }} />
        </div>
        <div className="sp-uptime-meta">
          <span>90-day uptime</span>
          <strong>{uptime.toFixed(2)}%</strong>
        </div>
      </div>

      {svc.active_incidents > 0 && (
        <p className="sp-service-note">
          {svc.active_incidents} active incident{svc.active_incidents > 1 ? "s" : ""}
        </p>
      )}
    </li>
  );
}

/* ── Incidents ─────────────────────────────────────────────────────────────*/
function ActiveIncident({ inc }) {
  const tone = SEVERITY_TONE[inc.severity] || "sp-muted";
  return (
    <li className={`sp-incident ${tone}`}>
      <div className="sp-incident-main">
        <h3 className="sp-incident-title">{inc.title}</h3>
        <p className="sp-incident-meta">
          {inc.service} · ongoing for {elapsedSince(inc.started_at)} · started {fmtDateTime(inc.started_at)}
        </p>
      </div>
      <div className="sp-incident-tags">
        <span className="sp-tag sp-tag-sev">{inc.severity}</span>
        <span className="sp-tag">{inc.status === "acknowledged" ? "investigating" : inc.status}</span>
      </div>
    </li>
  );
}

/* Past incidents group by month — the convention every status page follows,
   and the only way a long history stays scannable. */
function groupByMonth(history) {
  const groups = new Map();
  for (const entry of history) {
    const when = entry.resolved_at || entry.started_at;
    const key = when
      ? new Date(when).toLocaleDateString(undefined, { month: "long", year: "numeric" })
      : "Earlier";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(entry);
  }
  return [...groups.entries()];
}

function HistoryEntry({ entry }) {
  const tone = SEVERITY_TONE[entry.severity] || "sp-muted";
  return (
    <li className={`sp-history ${tone}`}>
      <span className="sp-history-marker" aria-hidden="true" />
      <div className="sp-history-body">
        <h4 className="sp-history-title">{entry.title}</h4>
        <p className="sp-history-meta">
          {entry.service} · resolved{" "}
          {entry.resolved_at
            ? new Date(entry.resolved_at).toLocaleDateString(undefined, { month: "short", day: "numeric" })
            : "—"}{" "}
          · down for {fmtDuration(entry.duration_minutes)}
        </p>
      </div>
      <span className="sp-history-resolved">
        <Check size={11} /> Resolved
      </span>
    </li>
  );
}

/* ── Subscribe ─────────────────────────────────────────────────────────────*/
function SubscribePanel({ tenant }) {
  const [channel, setChannel] = useState("email");
  const [target, setTarget] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);

  async function handleSubscribe(e) {
    e.preventDefault();
    if (!target.trim()) {
      setResult({ ok: false, message: "Enter an address to subscribe." });
      return;
    }
    setLoading(true);
    setResult(null);
    try {
      const res = await subscribeToStatus(channel, target.trim(), tenant);
      setResult({ ok: true, message: res.message || "Subscribed. You'll be notified of new incidents." });
      setTarget("");
    } catch (ex) {
      setResult({ ok: false, message: ex.message || "Subscription failed. Please try again." });
    } finally {
      setLoading(false);
    }
  }

  const feedURL = `${window.location.origin}/api/v1/status${
    tenant && tenant !== "default" ? `/${tenant}` : ""
  }`;

  return (
    <div className="sp-subscribe">
      <div className="sp-subscribe-intro">
        <span className="sp-subscribe-icon" aria-hidden="true">
          <Bell size={16} />
        </span>
        <div>
          <h3 className="sp-subscribe-title">Get notified</h3>
          <p className="sp-subscribe-sub">
            We&rsquo;ll email you — or post to Slack — whenever an incident opens or resolves.
          </p>
        </div>
      </div>

      <form className="sp-subscribe-form" onSubmit={handleSubscribe}>
        <div className="sp-field sp-field-channel">
          <label className="sp-label" htmlFor="sp-channel">Channel</label>
          <select
            id="sp-channel"
            className="sp-select"
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
          >
            <option value="email">Email</option>
            <option value="slack">Slack</option>
          </select>
        </div>

        <div className="sp-field sp-field-target">
          <label className="sp-label" htmlFor="sp-target">
            {channel === "email" ? "Email address" : "Slack webhook URL"}
          </label>
          <input
            id="sp-target"
            className="sp-input"
            type={channel === "email" ? "email" : "url"}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder={channel === "email" ? "you@company.com" : "https://hooks.slack.com/…"}
          />
        </div>

        <button type="submit" className="sp-button" disabled={loading}>
          {loading ? "Subscribing…" : "Subscribe"}
        </button>
      </form>

      {result && (
        <p className={`sp-subscribe-result ${result.ok ? "sp-ok" : "sp-down"}`} role="status">
          {result.ok ? <Check size={12} /> : <AlertTriangle size={12} />}
          {result.message}
        </p>
      )}

      <p className="sp-feed">
        Prefer to poll? <code>{feedURL}</code> returns this page as JSON. No authentication required.
      </p>
    </div>
  );
}

/* ── Section wrapper ───────────────────────────────────────────────────────*/
function Section({ title, count, children }) {
  return (
    <section className="sp-section">
      <h2 className="sp-section-title">
        {title}
        {count != null && <span className="sp-section-count">{count}</span>}
      </h2>
      {children}
    </section>
  );
}

/* ── Data ──────────────────────────────────────────────────────────────────*/
function useStatusData(tenant) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshAt, setRefreshAt] = useState(null);

  const load = useCallback(async () => {
    setError("");
    try {
      const result = await getStatus(tenant === "default" ? "" : tenant);
      setData(result);
      setRefreshAt(new Date());
    } catch (e) {
      setError(e.message || "Unable to load status right now.");
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

/* ── Body ──────────────────────────────────────────────────────────────────*/
function StatusBody({ tenant, standalone = false }) {
  const { data, loading, error, refreshAt, reload } = useStatusData(tenant);

  const services = data?.services || [];
  const active = data?.active_incidents || [];
  const history = data?.history || [];
  const summary = data?.uptime_summary || {};

  // Memo keys off data.history itself. Keying off the `|| []` fallback above
  // would allocate a fresh array on every render and re-group every time.
  const monthly = useMemo(() => groupByMonth(data?.history || []), [data?.history]);

  if (loading && !data) {
    return (
      <div className="sp-body">
        <div className="sp-skeleton sp-skeleton-banner" />
        <div className="sp-skeleton sp-skeleton-row" />
        <div className="sp-skeleton sp-skeleton-row" />
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="sp-body">
        <div className="sp-error">
          <AlertTriangle size={22} />
          <p className="sp-error-title">Status is temporarily unavailable</p>
          <p className="sp-error-message">{error}</p>
          <button type="button" className="sp-button sp-button-ghost" onClick={reload}>
            <RefreshCw size={12} /> Try again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="sp-body">
      <OverallBanner status={data?.status} updatedAt={data?.updated_at} />

      {/* Promoted above everything else: if something is broken, this is why
          the visitor opened the page. */}
      {active.length > 0 && (
        <Section title="Active incidents" count={active.length}>
          <ul className="sp-list">
            {active.map((inc) => (
              <ActiveIncident key={inc.id} inc={inc} />
            ))}
          </ul>
        </Section>
      )}

      <Section title="Uptime">
        <UptimeSummary summary={summary} />
      </Section>

      <Section title="Services" count={services.length || null}>
        {services.length === 0 ? (
          <p className="sp-empty">
            No services are being tracked yet. They appear here automatically as incidents arrive.
          </p>
        ) : (
          <ul className="sp-list sp-services">
            {services.map((svc) => (
              <ServiceRow key={svc.name} svc={svc} />
            ))}
          </ul>
        )}
      </Section>

      <Section title="Past incidents">
        {history.length === 0 ? (
          <p className="sp-empty sp-ok">
            <Wifi size={14} /> No incidents on record. Nothing has gone wrong yet.
          </p>
        ) : (
          monthly.map(([month, entries]) => (
            <div key={month} className="sp-month">
              <h3 className="sp-month-title">{month}</h3>
              <ul className="sp-list">
                {entries.map((e) => (
                  <HistoryEntry key={e.id} entry={e} />
                ))}
              </ul>
            </div>
          ))
        )}
      </Section>

      <Section title="Subscribe to updates">
        <SubscribePanel tenant={tenant} />
      </Section>

      <footer className="sp-refresh">
        <Clock size={11} />
        {refreshAt
          ? `Updated ${refreshAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · refreshes every 30s`
          : "Refreshes every 30s"}
        <button type="button" className="sp-refresh-btn" onClick={reload} disabled={loading}>
          <RefreshCw size={11} className={loading ? "is-spinning" : ""} /> Refresh now
        </button>
        {standalone && tenant !== "default" && <span className="sp-tenant">Tenant: {tenant}</span>}
      </footer>
    </div>
  );
}

/* ── Standalone public page ────────────────────────────────────────────────*/
export function PublicStatusPage({ tenant = "default" }) {
  return (
    <div className="sp-page">
      <header className="sp-header">
        <div className="sp-header-inner">
          <a className="sp-brand" href="/">
            <span className="sp-brand-mark" aria-hidden="true">N</span>
            <span className="sp-brand-text">
              <span className="sp-brand-name">NeuroOps</span>
              <span className="sp-brand-sub">Status</span>
            </span>
          </a>
          <a href="/" className="sp-button sp-button-ghost sp-header-link">
            <ChevronLeft size={13} /> Dashboard
          </a>
        </div>
      </header>

      <main className="sp-main">
        <StatusBody tenant={tenant} standalone />
      </main>

      <footer className="sp-footer">
        <span>Status is published directly from live incident data — no manual updates.</span>
      </footer>
    </div>
  );
}

/* ── Embedded dashboard view ───────────────────────────────────────────────*/
export default function StatusPageView({ tenant = "default" }) {
  return (
    <div className="sp-embedded">
      <StatusBody tenant={tenant} />
    </div>
  );
}
