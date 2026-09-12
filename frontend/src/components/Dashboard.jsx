/**
 * NeuroOps — Command Center Dashboard
 *
 * All data sourced from live API. No mocks, no fallback fabrication.
 *
 * Layout intent: the page answers three questions in descending urgency —
 *   1. Is anything on fire right now?          (status strip + KPI row)
 *   2. What is it, and what changed?           (incident feed, widest column)
 *   3. Is the platform itself healthy?         (right rail, quieter)
 * Everything visual comes from the shared UI kit, so this page cannot drift
 * away from the rest of the product.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity, AlertTriangle, ArrowRight, BellOff, Bot, Brain, CheckCircle2, Clock,
  Cpu, Flame, Gauge, Globe, HardDrive, RefreshCw, Server,
  Shield, Sparkles, XCircle, Zap,
} from "lucide-react";

import {
  AllClearState, BarList, EmptyState, Grid, ListRow, MetricRow, Page,
  PageHeader, Panel, SeverityPill, SkeletonRows, Split, StatTile,
  StatusPill, StatusPulse,
} from "./ui/Primitives.jsx";
import MutesDrawer from "./MutesDrawer.jsx";
import { roleFromToken } from "../api/auth.js";

async function api(path, token) {
  try {
    const r = await fetch(`/api/v1${path}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    if (!r.ok) return null;
    return await r.json();
  } catch {
    return null;
  }
}

function fmtAge(iso) {
  if (!iso) return "—";
  const s = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function fmtUptime(s) {
  if (!s) return "—";
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h < 24 ? `${h}h ${m}m` : `${Math.floor(h / 24)}d ${h % 24}h`;
}

function deriveStats(items) {
  const all = Array.isArray(items) ? items : [];
  const by = (fn) => all.filter(fn).length;

  let mttrSum = 0;
  let mttrCount = 0;
  for (const i of all) {
    if (i.status === "resolved" && i.first_event_time && i.last_event_time) {
      const d = new Date(i.last_event_time) - new Date(i.first_event_time);
      if (d > 0) {
        mttrSum += d;
        mttrCount++;
      }
    }
  }
  const mttrMs = mttrCount ? mttrSum / mttrCount : null;

  return {
    total: all.length,
    open: by((i) => i.status === "open"),
    acknowledged: by((i) => i.status === "acknowledged"),
    resolved: by((i) => i.status === "resolved"),
    critical: by((i) => i.severity === "critical"),
    high: by((i) => i.severity === "high"),
    medium: by((i) => i.severity === "medium"),
    low: by((i) => i.severity === "low"),
    recurring: by((i) => i.seen_before),
    changeLinked: by((i) => i.has_what_changed),
    mttrStr: mttrMs
      ? mttrMs < 3_600_000
        ? `${Math.round(mttrMs / 60000)}m`
        : `${(mttrMs / 3_600_000).toFixed(1)}h`
      : "—",
  };
}

/* ── Status donut ────────────────────────────────────────────────────────
   Kept as a local component because it is genuinely one-off. Colours come
   from tokens, not literals.                                               */
function StatusDonut({ open, acknowledged, resolved }) {
  const total = open + acknowledged + resolved || 1;
  const o = (open / total) * 360;
  const a = (acknowledged / total) * 360;
  const r = (resolved / total) * 360;

  return (
    <div className="donut-block">
      <div
        className="donut"
        style={{
          background: `conic-gradient(
            var(--red) 0deg ${o}deg,
            var(--amber) ${o}deg ${o + a}deg,
            var(--green) ${o + a}deg ${o + a + r}deg,
            var(--surface-3) ${o + a + r}deg 360deg)`,
        }}
      >
        <div className="donut-hole">
          <span className="donut-value">{open + acknowledged}</span>
          <span className="donut-label">active</span>
        </div>
      </div>
      <ul className="donut-legend">
        {[
          { label: "Open", count: open, tone: "tone-danger" },
          { label: "Acknowledged", count: acknowledged, tone: "tone-warning" },
          { label: "Resolved", count: resolved, tone: "tone-success" },
        ].map((row) => (
          <li key={row.label} className={`donut-legend-row ${row.tone}`}>
            <span className="donut-legend-dot" aria-hidden="true" />
            <span className="donut-legend-label">{row.label}</span>
            <span className="donut-legend-count">{row.count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* A compact ring gauge for the alert-quality score. */
function QualityRing({ score, tone }) {
  const circumference = 163.4; // 2πr, r = 26
  return (
    <div className={`quality-ring ${tone}`}>
      <svg width="68" height="68" viewBox="0 0 68 68" aria-hidden="true">
        <circle cx="34" cy="34" r="26" className="quality-ring-track" />
        <circle
          cx="34"
          cy="34"
          r="26"
          className="quality-ring-value"
          strokeDasharray={`${(score / 100) * circumference} ${circumference}`}
          transform="rotate(-90 34 34)"
        />
      </svg>
      <div className="quality-ring-center">
        <span className="quality-ring-score">{score}</span>
        <span className="quality-ring-caption">quality</span>
      </div>
    </div>
  );
}

/* ── Dashboard ───────────────────────────────────────────────────────────*/
export default function Dashboard({ token, onNavigate }) {
  const [incidents, setIncidents] = useState([]);
  const [platform, setPlatform] = useState(null);
  const [alertQ, setAlertQ] = useState(null);
  const [sources, setSources] = useState([]);
  const [aiStatus, setAiStatus] = useState(null);
  const [health, setHealth] = useState(null);
  const [mutes, setMutes] = useState(null);
  const [agents, setAgents] = useState(null);
  const [mutesOpen, setMutesOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [lastRefresh, setLastRefresh] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const timerRef = useRef(null);

  // fetchAll only fetches; apply sets state. Keeping them apart lets the
  // effect set state in a .then() callback, and ignore a reply that lands
  // after the page was left.
  const fetchAll = useCallback(() => Promise.allSettled([
    api("/incidents?page=1&page_size=100&sort_by=last_event_time&sort_order=desc", token),
    api("/platform/metrics", token),
    api("/alert-quality/report?window=30", token),
    api("/sources/health", token),
    api("/ai/status", null),
    api("/health", null),
    api("/mutes", token),
    api("/agents", token),
  ]), [token]);

  const apply = useCallback(([incR, platR, aqR, srcR, aiR, hlR, muteR, agentR]) => {
    if (incR.status === "fulfilled" && incR.value) {
      const d = incR.value;
      setIncidents(Array.isArray(d) ? d : d.items || d.incidents || d.data || []);
    }
    if (platR.status === "fulfilled" && platR.value) setPlatform(platR.value);
    if (aqR.status === "fulfilled" && aqR.value) setAlertQ(aqR.value);
    if (srcR.status === "fulfilled" && srcR.value) setSources(srcR.value.items || []);
    if (aiR.status === "fulfilled" && aiR.value) setAiStatus(aiR.value);
    if (hlR.status === "fulfilled" && hlR.value) setHealth(hlR.value);
    if (muteR.status === "fulfilled" && muteR.value) setMutes(muteR.value);
    // Viewers may not list agents; the tile then shows "—".
    if (agentR.status === "fulfilled" && Array.isArray(agentR.value)) setAgents(agentR.value);

    setLastRefresh(new Date());
    setLoading(false);
    setRefreshing(false);
  }, []);

  const load = useCallback(() => fetchAll().then(apply), [fetchAll, apply]);

  useEffect(() => {
    let active = true;
    const refresh = () => fetchAll().then((results) => active && apply(results));
    refresh();
    timerRef.current = setInterval(refresh, 30_000);
    return () => {
      active = false;
      clearInterval(timerRef.current);
    };
  }, [fetchAll, apply]);

  const stats = deriveStats(incidents);
  const recent = incidents.slice(0, 8);
  const aq = alertQ?.summary || {};
  const noisePct = aq.estimated_noise_pct || 0;
  const qualityTone = noisePct > 50 ? "tone-danger" : noisePct > 25 ? "tone-warning" : "tone-success";
  const healthy = health?.status === "ok";
  const srcHealthy = sources.filter((s) => s.status === "healthy" && !s.stale).length;
  // Tenant-scoped figures. /platform/metrics counts every tenant's rows (it is
  // for monitoring the platform), so it must not feed customer-facing tiles.
  const eventsIngested = sources.reduce((sum, s) => sum + (s.total_events || 0), 0);
  const activeAgents = agents ? agents.filter((a) => a.status === "active").length : null;
  const activeMutes = (mutes?.items || []).filter((m) => m.status === "active");

  return (
    <Page>
      <PageHeader
        eyebrow={
          <StatusPulse
            tone={healthy ? "success" : "danger"}
            label={`Platform ${healthy ? "online" : "degraded"}`}
            detail={health?.editions?.join(" · ") || "Community"}
          />
        }
        title="Command Center"
        meta={`Refreshed ${
          lastRefresh
            ? lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
            : "—"
        } · auto-refresh 30s`}
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => {
              setRefreshing(true);
              load();
            }} disabled={refreshing}>
              <RefreshCw size={13} className={refreshing ? "is-spinning" : ""} />
              Refresh
            </button>
            <button
              type="button"
              className={stats.open > 0 ? "btn btn-danger" : "btn btn-primary"}
              onClick={() => onNavigate("incidents")}
            >
              {stats.open > 0 ? (
                <>
                  <Flame size={13} /> {stats.open} open
                </>
              ) : (
                <>
                  <CheckCircle2 size={13} /> All clear
                </>
              )}
            </button>
          </>
        }
      />

      {/* First-run guidance — only while there is genuinely nothing to show. */}
      {!loading && stats.total === 0 && (
        <div className="cta-banner">
          <span className="cta-banner-icon" aria-hidden="true">
            <Zap size={20} />
          </span>
          <div className="cta-banner-text">
            <p className="cta-banner-title">Connect your first data source</p>
            <p className="cta-banner-sub">
              Paste a webhook URL into any monitoring tool — or fire a test alert in one click.
            </p>
          </div>
          <div className="cta-banner-actions">
            <button type="button" className="btn btn-primary" onClick={() => onNavigate("onboarding")}>
              <Sparkles size={13} /> Setup guide
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => onNavigate("integrations")}>
              Integrations <ArrowRight size={13} />
            </button>
          </div>
        </div>
      )}

      {/* 1 — Is anything on fire? */}
      <Grid cols={6}>
        <StatTile
          label="Open incidents" icon={Flame} loading={loading}
          tone={stats.open > 0 ? "danger" : "success"}
          value={loading ? null : stats.open} sub="need response"
          onClick={() => onNavigate("incidents")}
        />
        <StatTile
          label="Critical" icon={AlertTriangle} loading={loading}
          tone={stats.critical > 0 ? "danger" : "neutral"}
          value={loading ? null : stats.critical} sub="highest priority"
          onClick={() => onNavigate("incidents")}
        />
        <StatTile
          label="MTTR" icon={Clock} tone="brand" loading={loading}
          value={loading ? null : stats.mttrStr} sub="mean time to resolve"
        />
        <StatTile
          label="Events ingested" icon={Activity} tone="info" loading={loading}
          value={loading ? null : eventsIngested.toLocaleString()}
          sub="from your sources"
        />
        <StatTile
          label="Active agents" icon={Bot} tone="success" loading={loading}
          value={loading ? null : activeAgents ?? "—"} sub="monitoring agents"
          onClick={() => onNavigate("agents")}
        />
        <StatTile
          label="Uptime" icon={Gauge} tone="neutral" loading={loading}
          value={loading ? null : fmtUptime(platform?.uptime_seconds)}
          sub={platform ? `${platform.db_open_connections || 0} db connections` : "—"}
        />
      </Grid>

      {/* 2 — What is it? Incident feed gets the widest column. */}
      <Split>
        <Panel
          title="Recent incidents"
          sub={`${incidents.length} total · showing latest ${Math.min(recent.length, 8)}`}
          action="View all"
          onAction={() => onNavigate("incidents")}
          flush
        >
          {loading ? (
            <SkeletonRows count={5} height={52} />
          ) : recent.length === 0 ? (
            <AllClearState message="No incidents in the current dataset." />
          ) : (
            recent.map((inc) => (
              <ListRow
                key={inc.id}
                lead={<SeverityPill severity={inc.severity} />}
                title={inc.title || `Incident #${inc.id}`}
                sub={
                  <>
                    {inc.service || "—"}
                    {inc.seen_before && <span className="tag-recurring"> · recurring</span>}
                  </>
                }
                trailing={
                  <>
                    <StatusPill status={inc.status} />
                    {fmtAge(inc.last_event_time)}
                  </>
                }
                onClick={() => onNavigate("incidents")}
              />
            ))
          )}
        </Panel>

        <Panel title="Incident status" sub="current snapshot">
          {loading ? (
            <SkeletonRows count={1} height={88} />
          ) : (
            <StatusDonut
              open={stats.open}
              acknowledged={stats.acknowledged}
              resolved={stats.resolved}
            />
          )}
          <div className="panel-footnote">
            {[
              { label: "Total", value: stats.total, cls: "" },
              { label: "Change-linked", value: stats.changeLinked, cls: "is-brand" },
              { label: "Recurring", value: stats.recurring, cls: "is-info" },
            ].map((f) => (
              <div key={f.label} className="footnote-stat">
                <span className="footnote-label">{f.label}</span>
                <span className={`footnote-value ${f.cls}`}>{f.value}</span>
              </div>
            ))}
          </div>
        </Panel>
      </Split>

      {/* 3 — Signal quality and coverage. */}
      <Grid cols={4}>
        <Panel title="Severity breakdown" sub="across all incidents">
          {loading ? (
            <SkeletonRows count={4} height={6} />
          ) : (
            <BarList
              items={[
                { label: "Critical", value: stats.critical, tone: "danger" },
                { label: "High", value: stats.high, tone: "warning" },
                { label: "Medium", value: stats.medium, tone: "warning" },
                { label: "Low", value: stats.low, tone: "success" },
              ]}
            />
          )}
        </Panel>

        <Panel
          title="Alert quality"
          sub="last 30 days"
          action="Full report"
          onAction={() => onNavigate("alerts")}
        >
          {loading ? (
            <SkeletonRows count={2} height={54} />
          ) : !alertQ ? (
            <EmptyState
              icon={Shield}
              title="No quality data yet"
              message="A report appears once alerts have been flowing for a while."
            />
          ) : (
            <>
              <div className="quality-summary">
                <QualityRing score={100 - noisePct} tone={qualityTone} />
                <div className="quality-facts">
                  <span className="quality-facts-label">Estimated noise</span>
                  <span className={`quality-facts-value ${qualityTone}`}>{noisePct}%</span>
                  <span className="quality-facts-sub">{aq.total_fired_30d || 0} alerts fired</span>
                </div>
              </div>
              <div className="mini-stat-grid">
                {[
                  { label: "Noisy rules", value: aq.noisy_count || 0, warn: true },
                  { label: "Duplicates", value: aq.duplicate_pair_count || 0, warn: true },
                  { label: "Stale rules", value: aq.stale_count || 0, warn: true },
                  { label: "Unique rules", value: aq.unique_rules || 0, warn: false },
                ].map((s) => (
                  <div
                    key={s.label}
                    className={`mini-stat ${s.warn && s.value > 0 ? "tone-danger is-flagged" : "tone-neutral"}`}
                  >
                    <span className="mini-stat-label">{s.label}</span>
                    <span className="mini-stat-value">{s.value}</span>
                  </div>
                ))}
              </div>
            </>
          )}
        </Panel>

        <Panel
          title="Data sources"
          sub={`${srcHealthy}/${sources.length} healthy`}
          action="Configure"
          onAction={() => onNavigate("integrations")}
          flush
        >
          {loading ? (
            <SkeletonRows count={3} height={46} />
          ) : sources.length === 0 ? (
            <EmptyState
              icon={Globe}
              title="No sources configured"
              message="Connect a monitoring tool to start receiving alerts."
              action="Quick setup"
              onAction={() => onNavigate("onboarding")}
            />
          ) : (
            sources.slice(0, 5).map((src, i) => {
              const ok = src.status === "healthy" && !src.stale;
              const tone = ok ? "success" : src.stale ? "warning" : "danger";
              return (
                <ListRow
                  key={src.id || i}
                  lead={<span className={`dot tone-${tone}`} aria-hidden="true" />}
                  title={src.name || src.id}
                  sub={`${src.type || "webhook"} · ${fmtAge(src.last_event_at)}`}
                  trailing={
                    <span className={`pill is-plain tone-${tone}`}>
                      {src.stale ? "stale" : src.status || "unknown"}
                    </span>
                  }
                />
              );
            })
          )}
        </Panel>

        <Panel
          title="Muted alerts"
          sub="kept out of incidents"
          action="Open"
          onAction={() => setMutesOpen(true)}
        >
          {loading ? (
            <SkeletonRows count={2} height={46} />
          ) : !mutes ? (
            <EmptyState icon={BellOff} title="Unavailable" message="Muted alerts could not be loaded." />
          ) : (
            <>
              <div className="mini-stat-grid cols-2">
                <button type="button" className="mini-stat tone-neutral mute-stat" onClick={() => setMutesOpen(true)}>
                  <span className="mini-stat-label">Muted · 7 days</span>
                  <span className="mini-stat-value">{mutes.muted_7d}</span>
                </button>
                <button
                  type="button"
                  className={`mini-stat mute-stat ${mutes.active_count > 0 ? "tone-warning is-flagged" : "tone-neutral"}`}
                  onClick={() => setMutesOpen(true)}
                >
                  <span className="mini-stat-label">Active mutes</span>
                  <span className="mini-stat-value">{mutes.active_count}</span>
                </button>
              </div>
              {activeMutes.length === 0 ? (
                <p className="panel-footnote mute-card-note">Nothing is muted right now.</p>
              ) : (
                <ul className="mute-card-list">
                  {activeMutes.slice(0, 3).map((m) => (
                    <li key={m.id}>
                      <button type="button" className="mute-card-row" onClick={() => setMutesOpen(true)}>
                        <span className="mute-card-name">
                          {m.alert_names?.[0] === "all" ? "All alerts" : m.alert_names?.join(", ")}
                        </span>
                        <span className="mute-card-count">{m.match_count} muted</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </Panel>
      </Grid>

      <MutesDrawer
        token={token}
        open={mutesOpen}
        onClose={() => setMutesOpen(false)}
        isAdmin={roleFromToken(token) === "admin"}
        onChanged={() => load()}
      />

      {/* 4 — Platform internals: quietest row, lowest urgency. */}
      <Grid cols={2}>
        <Panel title="AI engine" sub="real-time intelligence" accent>
          <div className={`ai-status ${aiStatus?.configured ? "tone-brand" : "tone-danger"}`}>
            <span className="ai-status-icon" aria-hidden="true">
              <Brain size={17} />
            </span>
            <div className="ai-status-text">
              <span className="ai-status-name">{aiStatus?.provider || "AI Engine"}</span>
              <span className="ai-status-mode">
                {aiStatus?.configured ? `${aiStatus.data_mode || "live"} mode` : "not configured"}
              </span>
            </div>
            <StatusPulse tone={aiStatus?.configured ? "brand" : "danger"} label="" />
          </div>

          {aiStatus && (
            <div className="mini-stat-grid cols-2">
              <div className="mini-stat tone-neutral">
                <span className="mini-stat-label">Total calls</span>
                <span className="mini-stat-value">{aiStatus.total_calls ?? "—"}</span>
              </div>
              <div
                className={`mini-stat ${(aiStatus.failed_calls || 0) > 0 ? "tone-danger is-flagged" : "tone-neutral"}`}
              >
                <span className="mini-stat-label">Failed</span>
                <span className="mini-stat-value">{aiStatus.failed_calls ?? 0}</span>
              </div>
            </div>
          )}

          <div className="inline-note">
            <Sparkles size={12} className="inline-note-icon" />
            <span className="inline-note-label">Last AI call</span>
            <span className="inline-note-value">{fmtAge(aiStatus?.last_call_at)}</span>
          </div>
        </Panel>

        <Panel title="Platform internals" sub={`uptime ${fmtUptime(platform?.uptime_seconds)}`}>
          {loading ? (
            <SkeletonRows count={6} height={28} />
          ) : (
            <>
              <MetricRow icon={Cpu} label="Goroutines" value={platform?.goroutines} />
              <MetricRow icon={HardDrive} tone="success" label="Memory allocated" value={platform?.memory_alloc_mb ? `${platform.memory_alloc_mb} MB` : "—"} />
              <MetricRow icon={Server} tone="warning" label="DB connections" value={platform?.db_open_connections} />
              <MetricRow icon={XCircle} tone={(platform?.events_dropped || 0) > 0 ? "danger" : "neutral"} label="Events dropped" value={platform?.events_dropped} />
              <MetricRow icon={AlertTriangle} tone={(platform?.request_errors || 0) > 0 ? "warning" : "neutral"} label="Request errors" value={platform?.request_errors} />
            </>
          )}
        </Panel>
      </Grid>
    </Page>
  );
}
