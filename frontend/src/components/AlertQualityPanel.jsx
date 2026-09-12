/**
 * Alert Quality Governance — find noisy, duplicate and stale rules.
 *
 * The page is an argument, not a dump: the KPI row states how much noise you
 * have, the tabs let you attack it one category at a time, and every card ends
 * in a recommendation. Tabs carry counts so you can see where the work is
 * before clicking.
 *
 * This file previously referenced a second token vocabulary (--sp-3, --r-lg,
 * --text-muted, --danger …) that the design system does not define, so much of
 * it fell back to browser defaults. Everything now uses the real tokens.
 */
import { useCallback, useEffect, useState } from "react";
import {
  Activity, Bell, CheckCircle, Clock, Filter, RefreshCw,
  Shield, TrendingDown, TrendingUp, Zap,
} from "lucide-react";

import { apiRequest } from "../api/client";
import {
  EmptyState, ErrorState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

function timeAgo(dateStr) {
  if (!dateStr) return "—";
  const d = new Date(dateStr);
  if (Number.isNaN(d.getTime())) return dateStr;
  const secs = Math.floor((Date.now() - d) / 1000);
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d ago`;
}

/* Recommendations map to tones by how destructive the suggested fix is. */
const REC_META = {
  delete: { label: "Delete", tone: "tone-danger" },
  merge: { label: "Merge", tone: "tone-danger" },
  tune_or_delete_noisy_rules: { label: "Tune or delete", tone: "tone-danger" },
  tune_threshold: { label: "Tune threshold", tone: "tone-warning" },
  add_dedup_window: { label: "Add dedup window", tone: "tone-warning" },
  merge_duplicate_alerts: { label: "Merge duplicates", tone: "tone-warning" },
  review_false_positive_alerts: { label: "Review false positives", tone: "tone-warning" },
  consolidate: { label: "Consolidate", tone: "tone-brand" },
  review: { label: "Review", tone: "tone-neutral" },
  archive: { label: "Archive", tone: "tone-neutral" },
  review_condition: { label: "Review condition", tone: "tone-neutral" },
  archive_stale_rules: { label: "Archive stale", tone: "tone-neutral" },
  review_alert_inventory: { label: "Inventory review", tone: "tone-neutral" },
};

function RecBadge({ rec }) {
  const meta = REC_META[rec] || {
    label: String(rec || "—").replace(/_/g, " "),
    tone: "tone-neutral",
  };
  return <span className={`pill is-plain ${meta.tone}`}>{meta.label}</span>;
}

/* A 0–100 bar. `invert` means high is bad (false-positive rate, debt score). */
function ScoreBar({ score, invert = false }) {
  const pct = Math.max(0, Math.min(100, score || 0));
  const bad = invert ? pct : 100 - pct;
  const tone = bad >= 60 ? "tone-danger" : bad >= 35 ? "tone-warning" : "tone-success";
  return (
    <div className={`score-bar ${tone}`}>
      <div className="score-bar-track">
        <div className="score-bar-fill" style={{ width: `${pct}%` }} />
      </div>
      <span className="score-bar-value">{pct}</span>
    </div>
  );
}

/* Small labelled numbers used inside cards. */
function StatGrid({ items }) {
  return (
    <div className="mini-stat-grid cols-4">
      {items.map((m) => (
        <div key={m.label} className={`mini-stat ${m.tone || "tone-neutral"}${m.tone ? " is-flagged" : ""}`}>
          <span className="mini-stat-label">{m.label}</span>
          <span className="mini-stat-value">{m.value ?? 0}</span>
        </div>
      ))}
    </div>
  );
}

/* ── Tabs ────────────────────────────────────────────────────────────────*/

function NoisyTab({ items, loading }) {
  if (loading) return <SkeletonRows count={3} height={150} />;
  if (!items?.length) {
    return (
      <EmptyState
        icon={Bell}
        tone="success"
        title="No noisy alerts"
        message="Every rule is firing within acceptable signal-to-noise thresholds."
      />
    );
  }
  return (
    <div className="quality-list">
      {items.map((n) => (
        <Panel
          key={n.fingerprint}
          title={n.title || n.fingerprint?.slice(0, 28) || "Unnamed rule"}
          sub={[n.source, n.service, n.team].filter(Boolean).join(" · ") || "no metadata"}
          action={<RecBadge rec={n.recommendation} />}
        >
          <StatGrid
            items={[
              { label: "Fires (7d)", value: n.fire_count_7d },
              { label: "False positives", value: n.fp_count, tone: n.fp_count > 0 ? "tone-danger" : null },
              { label: "Noise fires", value: n.noise_count, tone: n.noise_count > 0 ? "tone-warning" : null },
              { label: "Useful fires", value: n.useful_count, tone: n.useful_count > 0 ? "tone-success" : null },
            ]}
          />
          <div className="score-row">
            <div className="score-item">
              <span className="score-label">False-positive rate — {Math.round((n.fp_rate || 0) * 100)}%</span>
              <ScoreBar score={Math.round((n.fp_rate || 0) * 100)} invert />
            </div>
            <div className="score-item">
              <span className="score-label">Quality score — {n.quality_score ?? 0}</span>
              <ScoreBar score={n.quality_score || 0} />
            </div>
          </div>
        </Panel>
      ))}
    </div>
  );
}

function DuplicatesTab({ items, loading }) {
  if (loading) return <SkeletonRows count={3} height={130} />;
  if (!items?.length) {
    return (
      <EmptyState
        icon={Filter}
        tone="success"
        title="No duplicate pairs"
        message="No alert pairs are consistently co-firing inside your deduplication window."
      />
    );
  }
  return (
    <div className="quality-list">
      {items.map((d, i) => (
        <Panel
          key={i}
          title="Duplicate pair"
          sub={`Co-fires ${d.co_occurrence_count}× · ${Math.round((d.co_occurrence_rate || 0) * 100)}% overlap`}
          action={<RecBadge rec={d.recommendation} />}
        >
          <div className="dup-pair">
            {[
              { title: d.title1, service: d.service1, fp: d.fingerprint1 },
              { title: d.title2, service: d.service2, fp: d.fingerprint2 },
            ].map((a, j) => (
              <div key={j} className="dup-card">
                <span className="dup-title">{a.title || a.fp?.slice(0, 20) || "Unnamed alert"}</span>
                {a.service && <span className="dup-service">{a.service}</span>}
                {a.fp && <span className="dup-fp">{a.fp.slice(0, 16)}…</span>}
              </div>
            ))}
            <span className="dup-link tone-warning" aria-hidden="true">
              <Zap size={14} />
            </span>
          </div>
        </Panel>
      ))}
    </div>
  );
}

function StaleTab({ items, loading }) {
  if (loading) return <SkeletonRows count={5} height={44} />;
  if (!items?.length) {
    return (
      <EmptyState
        icon={CheckCircle}
        tone="success"
        title="No stale rules"
        message="Every registered rule has fired inside the staleness threshold."
      />
    );
  }
  return (
    <Panel title="Stale alert rules" sub="have not fired in 30+ days" flush>
      <div className="table-scroll">
        <table className="data-table">
          <thead>
            <tr>
              <th scope="col">Rule</th>
              <th scope="col">Service</th>
              <th scope="col">Source</th>
              <th scope="col">Days stale</th>
              <th scope="col">Total fires</th>
              <th scope="col">Recommendation</th>
            </tr>
          </thead>
          <tbody>
            {items.map((s) => (
              <tr key={s.fingerprint}>
                <td className="cell-truncate">{s.title || s.fingerprint?.slice(0, 24) || "—"}</td>
                <td>{s.service ? <span className="chip tone-brand">{s.service}</span> : "—"}</td>
                <td className="cell-muted">{s.source || "—"}</td>
                <td>
                  <span
                    className={`cell-strong ${
                      s.days_since_last_fire > 60
                        ? "tone-danger"
                        : s.days_since_last_fire > 30
                          ? "tone-warning"
                          : "tone-neutral"
                    }`}
                  >
                    {s.days_since_last_fire}d
                  </span>
                </td>
                <td className="cell-muted">{s.total_fire_count}</td>
                <td><RecBadge rec={s.recommendation} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Panel>
  );
}

function DebtTab({ items, loading }) {
  if (loading) return <SkeletonRows count={3} height={160} />;
  if (!items?.length) {
    return (
      <EmptyState
        icon={TrendingDown}
        title="No alert debt data"
        message="Register rules and submit feedback to build up debt scoring over time."
      />
    );
  }
  return (
    <div className="quality-list">
      {items.map((d) => (
        <Panel
          key={d.service || d.team}
          title={d.service || "Unknown service"}
          sub={`${d.team ? `${d.team} · ` : ""}${d.unique_rules} rules · ${d.total_alerts_30d} fires in 30d`}
          action={<RecBadge rec={d.top_recommendation} />}
        >
          <div className="score-item debt-score">
            <span className="score-label">
              Alert debt score — higher is worse
              <strong className="debt-value">{d.debt_score} / 100</strong>
            </span>
            <ScoreBar score={d.debt_score} invert />
          </div>
          <StatGrid
            items={[
              { label: "Noisy rules", value: d.noisy_count, tone: d.noisy_count > 0 ? "tone-danger" : null },
              { label: "Stale rules", value: d.stale_count },
              { label: "Duplicates", value: d.duplicate_count, tone: d.duplicate_count > 0 ? "tone-warning" : null },
              { label: "FP feedback", value: d.fp_feedback_count, tone: d.fp_feedback_count > 0 ? "tone-warning" : null },
            ]}
          />
        </Panel>
      ))}
    </div>
  );
}

const TABS = [
  { key: "noisy", label: "Noisy alerts", icon: Bell },
  { key: "duplicates", label: "Duplicate pairs", icon: Zap },
  { key: "stale", label: "Stale rules", icon: Clock },
  { key: "debt", label: "Alert debt", icon: TrendingDown },
];

export default function AlertQualityPanel() {
  const [report, setReport] = useState(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [activeTab, setActiveTab] = useState("noisy");
  const [windowDays, setWindowDays] = useState(30);
  const [lastLoaded, setLastLoaded] = useState(null);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    setErr("");
    try {
      const data = await apiRequest(`/alert-quality/report?window=${windowDays}`);
      setReport(data);
      setLastLoaded(new Date());
    } catch (e) {
      setErr(e.message || "Failed to load the alert quality report");
    } finally {
      if (!silent) setLoading(false);
    }
  }, [windowDays]);

  useEffect(() => {
    load();
  }, [load]);

  const summary = report?.summary || {};
  const tabCounts = {
    noisy: summary.noisy_count ?? 0,
    duplicates: summary.duplicate_pair_count ?? 0,
    stale: summary.stale_count ?? 0,
    debt: report?.alert_debt?.length ?? 0,
  };
  const busy = loading && !report;
  const noise = summary.estimated_noise_pct ?? 0;

  return (
    <Page>
      <PageHeader
        title="Alert quality governance"
        meta={`Detect noisy, duplicate and stale alerts · reduce alert debt${
          lastLoaded ? ` · updated ${timeAgo(lastLoaded)}` : ""
        }`}
        actions={
          <>
            <select
              className="form-select window-select"
              value={windowDays}
              aria-label="Analysis window"
              onChange={(e) => setWindowDays(Number(e.target.value))}
            >
              {[7, 14, 30, 60, 90].map((d) => (
                <option key={d} value={d}>{d} days</option>
              ))}
            </select>
            <button type="button" className="btn btn-ghost" onClick={() => load()} disabled={loading}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} />
              Refresh
            </button>
          </>
        }
      />

      {err && <ErrorState message={err} onRetry={() => load()} />}

      <Grid cols={6}>
        <StatTile label="Total fires" icon={Activity} tone="brand" value={summary.total_fired_30d ?? 0} sub={`last ${windowDays}d`} loading={busy} />
        <StatTile label="Unique rules" icon={Shield} tone="info" value={summary.unique_rules ?? 0} sub="registered" loading={busy} />
        <StatTile label="Noisy alerts" icon={Bell} tone={tabCounts.noisy > 0 ? "danger" : "success"} value={tabCounts.noisy} sub="above threshold" loading={busy} />
        <StatTile label="Duplicate pairs" icon={Zap} tone={tabCounts.duplicates > 0 ? "warning" : "success"} value={tabCounts.duplicates} sub="co-firing" loading={busy} />
        <StatTile label="Estimated noise" icon={TrendingUp} tone={noise > 30 ? "danger" : noise > 15 ? "warning" : "success"} value={`${noise}%`} sub="of all fires" loading={busy} />
        <StatTile label="Recommendations" icon={CheckCircle} tone="success" value={summary.total_recommendations ?? 0} sub="ready to apply" loading={busy} />
      </Grid>

      <nav className="tabs quality-tabs" role="tablist">
        {TABS.map((tab) => {
          const Icon = tab.icon;
          const active = activeTab === tab.key;
          const count = tabCounts[tab.key] ?? 0;
          return (
            <button
              key={tab.key}
              type="button"
              role="tab"
              aria-selected={active}
              className={`tab-item${active ? " active" : ""}`}
              onClick={() => setActiveTab(tab.key)}
            >
              <Icon size={13} />
              {tab.label}
              {count > 0 && <span className="tab-count">{count}</span>}
            </button>
          );
        })}
      </nav>

      <div className="fade-in" key={activeTab}>
        {activeTab === "noisy" && <NoisyTab items={report?.noisy_alerts} loading={busy} />}
        {activeTab === "duplicates" && <DuplicatesTab items={report?.duplicates} loading={busy} />}
        {activeTab === "stale" && <StaleTab items={report?.stale_rules} loading={busy} />}
        {activeTab === "debt" && <DebtTab items={report?.alert_debt} loading={busy} />}
      </div>

      {report && (
        <footer className="page-meta-footer">
          <span>Generated {report.generated_at || timeAgo(lastLoaded)}</span>
          <span>
            {report.window_days}d window · tenant {report.tenant_id || "default"}
          </span>
        </footer>
      )}
    </Page>
  );
}
