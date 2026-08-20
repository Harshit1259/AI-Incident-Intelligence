import { useCallback, useEffect, useState } from "react";
import { apiRequest } from "../api/client";
import {
  Activity, AlertTriangle, BarChart3, Bell,
  CheckCircle, Clock, Filter, RefreshCw,
  Shield, TrendingDown, TrendingUp, Trash2, XCircle, Zap,
} from "lucide-react";

// ── Helpers ────────────────────────────────────────────────────────────────

function timeAgo(dateStr) {
  if (!dateStr) return "—";
  const d = new Date(dateStr);
  if (isNaN(d)) return dateStr;
  const secs = Math.floor((Date.now() - d) / 1000);
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d ago`;
}

// ── Recommendation badge mapping ───────────────────────────────────────────

const REC_META = {
  delete:                       { label: "Delete",          cls: "critical" },
  tune_threshold:               { label: "Tune Threshold",  cls: "warning"  },
  consolidate:                  { label: "Consolidate",     cls: "primary"  },
  review:                       { label: "Review",          cls: "neutral"  },
  merge:                        { label: "Merge",           cls: "critical" },
  add_dedup_window:             { label: "Add Dedup",       cls: "warning"  },
  archive:                      { label: "Archive",         cls: "neutral"  },
  review_condition:             { label: "Review Condition",cls: "neutral"  },
  tune_or_delete_noisy_rules:   { label: "Tune / Delete",  cls: "critical" },
  merge_duplicate_alerts:       { label: "Merge Dups",      cls: "warning"  },
  archive_stale_rules:          { label: "Archive Stale",   cls: "neutral"  },
  review_false_positive_alerts: { label: "Review FP",       cls: "warning"  },
  review_alert_inventory:       { label: "Inventory Review",cls: "neutral"  },
};

function RecBadge({ rec }) {
  const meta = REC_META[rec] || { label: String(rec || "—").replace(/_/g, " "), cls: "neutral" };
  return <span className={`badge ${meta.cls}`} style={{ fontSize: 10 }}>{meta.label}</span>;
}

// ── Score bar component ────────────────────────────────────────────────────

function ScoreBar({ score, invert = false, height = 6 }) {
  const pct = Math.max(0, Math.min(100, score));
  const bad = invert ? pct : 100 - pct;
  const color = bad >= 60 ? "var(--danger)" : bad >= 35 ? "var(--warning)" : "var(--success)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-3)" }}>
      <div style={{
        flex: 1, height, background: "var(--bg-overlay)", borderRadius: 3, overflow: "hidden",
      }}>
        <div style={{
          width: `${pct}%`, height: "100%",
          background: color, borderRadius: 3, transition: "width .4s ease",
        }} />
      </div>
      <span style={{ fontSize: 11, color, fontWeight: 700, minWidth: 26, textAlign: "right" }}>
        {score}
      </span>
    </div>
  );
}

// ── Skeleton row ───────────────────────────────────────────────────────────

function SkeletonCards({ count = 3 }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="skeleton" style={{ height: 120, borderRadius: "var(--r-lg)" }} />
      ))}
    </div>
  );
}

// ── Empty state ────────────────────────────────────────────────────────────

function TabEmpty({ icon: Icon, title, desc }) {
  return (
    <div className="empty-state" style={{ padding: "var(--sp-10) 0" }}>
      <div className="empty-icon"><Icon size={26} style={{ color: "var(--text-muted)" }} /></div>
      <div className="empty-title">{title}</div>
      <div className="empty-desc">{desc}</div>
    </div>
  );
}

// ── Noisy Alerts tab ───────────────────────────────────────────────────────

function NoisyTab({ items, loading }) {
  if (loading) return <SkeletonCards count={3} />;
  if (!items || items.length === 0) {
    return (
      <TabEmpty
        icon={Bell}
        title="No noisy alerts detected"
        desc="All your alert rules are firing within acceptable signal-to-noise thresholds."
      />
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      {items.map((n) => (
        <div key={n.fingerprint} className="card fade-in">
          <div className="card-header">
            <div className="card-header-left" style={{ minWidth: 0, flex: 1 }}>
              <div className="card-icon danger"><Bell size={15} /></div>
              <div style={{ minWidth: 0 }}>
                <div className="card-title" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {n.title || n.fingerprint?.slice(0, 28) || "Unnamed rule"}
                </div>
                <div className="card-subtitle">
                  {[n.source, n.service, n.team].filter(Boolean).join(" · ")}
                </div>
              </div>
            </div>
            <RecBadge rec={n.recommendation} />
          </div>
          <div className="card-body">
            {/* 4 metric tiles */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "var(--sp-3)", marginBottom: "var(--sp-4)" }}>
              {[
                { label: "Fires (7d)",       value: n.fire_count_7d,              color: "var(--text-primary)" },
                { label: "False positives",  value: n.fp_count,                   color: "var(--danger)"  },
                { label: "Noise fires",      value: n.noise_count,                color: "var(--warning)" },
                { label: "Useful fires",     value: n.useful_count,               color: "var(--success)" },
              ].map((m) => (
                <div key={m.label} style={{
                  background: "var(--bg-elevated)", borderRadius: "var(--r-md)",
                  padding: "var(--sp-3)", textAlign: "center", border: "1px solid var(--border)",
                }}>
                  <div style={{ fontSize: 20, fontWeight: 800, color: m.color, letterSpacing: "-0.03em" }}>
                    {m.value ?? 0}
                  </div>
                  <div style={{ fontSize: 10, color: "var(--text-muted)", marginTop: 2 }}>{m.label}</div>
                </div>
              ))}
            </div>

            {/* Score bars */}
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "var(--sp-4)" }}>
              <div>
                <div style={{ fontSize: 11, color: "var(--text-muted)", marginBottom: "var(--sp-2)" }}>
                  FP Rate — {Math.round((n.fp_rate || 0) * 100)}%
                </div>
                <ScoreBar score={Math.round((n.fp_rate || 0) * 100)} invert />
              </div>
              <div>
                <div style={{ fontSize: 11, color: "var(--text-muted)", marginBottom: "var(--sp-2)" }}>
                  Quality Score — {n.quality_score}
                </div>
                <ScoreBar score={n.quality_score || 0} />
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ── Duplicates tab ─────────────────────────────────────────────────────────

function DuplicatesTab({ items, loading }) {
  if (loading) return <SkeletonCards count={3} />;
  if (!items || items.length === 0) {
    return (
      <TabEmpty
        icon={Filter}
        title="No duplicate pairs found"
        desc="No alert pairs are consistently co-firing within your deduplication window."
      />
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      {items.map((d, i) => (
        <div key={i} className="card fade-in">
          <div className="card-header">
            <div className="card-header-left">
              <div className="card-icon warning"><Zap size={15} /></div>
              <div>
                <div className="card-title">Duplicate Pair</div>
                <div className="card-subtitle">
                  Co-fires <strong style={{ color: "var(--text-primary)" }}>{d.co_occurrence_count}×</strong>
                  {" · "}{Math.round((d.co_occurrence_rate || 0) * 100)}% overlap rate
                </div>
              </div>
            </div>
            <RecBadge rec={d.recommendation} />
          </div>
          <div className="card-body">
            <div style={{ display: "grid", gridTemplateColumns: "1fr auto 1fr", gap: "var(--sp-3)", alignItems: "center" }}>
              {[
                { title: d.title1 || d.fingerprint1?.slice(0, 20), service: d.service1, fp: d.fingerprint1 },
                null,
                { title: d.title2 || d.fingerprint2?.slice(0, 20), service: d.service2, fp: d.fingerprint2 },
              ].map((a, j) => {
                if (a === null) return (
                  <div key="arrow" style={{ textAlign: "center", color: "var(--warning)", fontSize: 20, fontWeight: 700 }}>↔</div>
                );
                return (
                  <div key={j} style={{
                    background: "var(--bg-elevated)", borderRadius: "var(--r-lg)",
                    padding: "var(--sp-3) var(--sp-4)", border: "1px solid var(--border)",
                  }}>
                    <div style={{ fontSize: 13, fontWeight: 600, color: "var(--text-primary)", marginBottom: 3 }}>
                      {a.title || "Unnamed alert"}
                    </div>
                    {a.service && (
                      <div style={{ fontSize: 11, color: "var(--primary-light)" }}>{a.service}</div>
                    )}
                    {a.fp && (
                      <div style={{ fontSize: 10, color: "var(--text-muted)", marginTop: 4, fontFamily: "monospace" }}>
                        {a.fp.slice(0, 16)}…
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ── Stale Rules tab ────────────────────────────────────────────────────────

function StaleTab({ items, loading }) {
  if (loading) return <SkeletonCards count={4} />;
  if (!items || items.length === 0) {
    return (
      <TabEmpty
        icon={CheckCircle}
        title="No stale rules detected"
        desc="All registered alert rules have fired within the staleness threshold."
      />
    );
  }
  return (
    <div className="card">
      <div className="card-header">
        <div className="card-header-left">
          <div className="card-icon neutral"><Clock size={15} /></div>
          <div>
            <div className="card-title">Stale Alert Rules</div>
            <div className="card-subtitle">Rules that haven't fired in 30+ days</div>
          </div>
        </div>
        <span className="badge neutral">{items.length} rules</span>
      </div>
      <table className="data-table">
        <thead>
          <tr>
            <th>Rule Name</th>
            <th>Service</th>
            <th>Source</th>
            <th>Days Stale</th>
            <th>Total Fires</th>
            <th>Recommendation</th>
          </tr>
        </thead>
        <tbody>
          {items.map((s) => (
            <tr key={s.fingerprint}>
              <td style={{ color: "var(--text-primary)", fontWeight: 500, maxWidth: 220 }}>
                <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {s.title || s.fingerprint?.slice(0, 24) || "—"}
                </div>
              </td>
              <td>
                {s.service ? <span className="badge primary" style={{ fontSize: 10 }}>{s.service}</span> : "—"}
              </td>
              <td style={{ color: "var(--text-muted)" }}>{s.source || "—"}</td>
              <td>
                <span style={{
                  color: s.days_since_last_fire > 60 ? "var(--danger)" : s.days_since_last_fire > 30 ? "var(--warning)" : "var(--text-secondary)",
                  fontWeight: 600,
                }}>
                  {s.days_since_last_fire}d
                </span>
              </td>
              <td style={{ color: "var(--text-muted)" }}>{s.total_fire_count}</td>
              <td><RecBadge rec={s.recommendation} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Alert Debt tab ─────────────────────────────────────────────────────────

function DebtTab({ items, loading }) {
  if (loading) return <SkeletonCards count={3} />;
  if (!items || items.length === 0) {
    return (
      <TabEmpty
        icon={BarChart3}
        title="No alert debt data"
        desc="Register alert rules and submit feedback to build up alert debt scoring over time."
      />
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      {items.map((d) => (
        <div key={d.service || d.team} className="card fade-in">
          <div className="card-header">
            <div className="card-header-left">
              <div className="card-icon danger"><TrendingDown size={15} /></div>
              <div>
                <div className="card-title">{d.service || "Unknown service"}</div>
                <div className="card-subtitle">
                  {d.team ? `Team: ${d.team} · ` : ""}{d.unique_rules} rules · {d.total_alerts_30d} fires (30d)
                </div>
              </div>
            </div>
            <div style={{ display: "flex", gap: "var(--sp-2)", alignItems: "center" }}>
              <RecBadge rec={d.top_recommendation} />
            </div>
          </div>
          <div className="card-body">
            {/* Debt score bar */}
            <div style={{ marginBottom: "var(--sp-4)" }}>
              <div style={{
                display: "flex", alignItems: "center", justifyContent: "space-between",
                marginBottom: "var(--sp-2)",
              }}>
                <span style={{ fontSize: 11, color: "var(--text-muted)" }}>Alert Debt Score (higher = worse)</span>
                <span style={{
                  fontSize: 13, fontWeight: 700,
                  color: d.debt_score >= 60 ? "var(--danger)" : d.debt_score >= 35 ? "var(--warning)" : "var(--success)",
                }}>
                  {d.debt_score} / 100
                </span>
              </div>
              <ScoreBar score={d.debt_score} invert height={8} />
            </div>

            {/* Issue breakdown */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "var(--sp-3)" }}>
              {[
                { label: "Noisy rules",   value: d.noisy_count,        color: "var(--danger)"  },
                { label: "Stale rules",   value: d.stale_count,        color: "var(--text-muted)" },
                { label: "Duplicates",    value: d.duplicate_count,    color: "var(--warning)" },
                { label: "FP feedback",   value: d.fp_feedback_count,  color: "var(--sev-high)" },
              ].map((m) => (
                <div key={m.label} style={{
                  background: "var(--bg-elevated)", borderRadius: "var(--r-md)",
                  padding: "var(--sp-3)", textAlign: "center", border: "1px solid var(--border)",
                }}>
                  <div style={{ fontSize: 22, fontWeight: 800, color: m.color, letterSpacing: "-0.03em" }}>
                    {m.value ?? 0}
                  </div>
                  <div style={{ fontSize: 10, color: "var(--text-muted)", marginTop: 2 }}>{m.label}</div>
                </div>
              ))}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// ── Tab bar configuration ──────────────────────────────────────────────────

const TABS = [
  { key: "noisy",      label: "Noisy Alerts",    icon: Bell,          colorVar: "--danger"  },
  { key: "duplicates", label: "Duplicate Pairs",  icon: Zap,           colorVar: "--warning" },
  { key: "stale",      label: "Stale Rules",      icon: Clock,         colorVar: "--text-muted" },
  { key: "debt",       label: "Alert Debt",       icon: TrendingDown,  colorVar: "--sev-high" },
];

// ── Main panel ─────────────────────────────────────────────────────────────

export default function AlertQualityPanel() {
  const [report, setReport]         = useState(null);
  const [loading, setLoading]       = useState(false);
  const [err, setErr]               = useState("");
  const [activeTab, setActiveTab]   = useState("noisy");
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
      setErr(e.message || "Failed to load alert quality report");
    } finally {
      if (!silent) setLoading(false);
    }
  }, [windowDays]);

  useEffect(() => { load(); }, [load]);

  const summary = report?.summary || {};
  const tabCounts = report ? {
    noisy:      report.summary?.noisy_count       ?? 0,
    duplicates: report.summary?.duplicate_pair_count ?? 0,
    stale:      report.summary?.stale_count       ?? 0,
    debt:       report.alert_debt?.length         ?? 0,
  } : {};

  const kpiCards = [
    {
      label: "Total Fires (30d)", value: summary.total_fired_30d ?? 0,
      icon: Activity,  color: "primary", accent: "--primary",
    },
    {
      label: "Unique Rules",      value: summary.unique_rules ?? 0,
      icon: Shield,    color: "info",    accent: "--info",
    },
    {
      label: "Noisy Alerts",      value: summary.noisy_count ?? 0,
      icon: Bell,      color: "critical", accent: "--danger",
    },
    {
      label: "Duplicate Pairs",   value: summary.duplicate_pair_count ?? 0,
      icon: Zap,       color: "warning",  accent: "--warning",
    },
    {
      label: "Est. Noise",        value: `${summary.estimated_noise_pct ?? 0}%`,
      icon: TrendingUp, color: "high",   accent: "--sev-high",
    },
    {
      label: "Recommendations",   value: summary.total_recommendations ?? 0,
      icon: CheckCircle, color: "success", accent: "--success",
    },
  ];

  return (
    <div className="page-inner fade-in">
      {/* Error toast */}
      {err && (
        <div className="toast error" style={{ marginBottom: "var(--sp-4)", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <span>{err}</span>
          <button onClick={() => setErr("")} style={{ background: "none", border: "none", color: "inherit", cursor: "pointer", fontSize: 18, lineHeight: 1, marginLeft: "var(--sp-4)" }}>✕</button>
        </div>
      )}

      {/* ── Page Header ─────────────────────────────────────────────── */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "var(--sp-6)" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-4)" }}>
          <div style={{
            width: 40, height: 40, borderRadius: "var(--r-lg)",
            background: "linear-gradient(135deg, var(--warning) 0%, var(--sev-high) 100%)",
            display: "flex", alignItems: "center", justifyContent: "center",
            boxShadow: "0 4px 16px rgba(245,158,11,0.35)",
            flexShrink: 0,
          }}>
            <Bell size={20} color="white" />
          </div>
          <div>
            <h1 style={{ fontSize: 22, fontWeight: 800, letterSpacing: "-0.03em", lineHeight: 1.1, margin: 0 }}>
              Alert Quality Governance
            </h1>
            <div style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 3 }}>
              NeuroOps · Detect noisy, duplicate & stale alerts · reduce alert debt
            </div>
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-3)" }}>
          {lastLoaded && (
            <div style={{ fontSize: 11, color: "var(--text-muted)" }}>
              Updated {timeAgo(lastLoaded)}
            </div>
          )}
          <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-2)" }}>
            <span style={{ fontSize: 12, color: "var(--text-muted)" }}>Window:</span>
            <select
              className="form-select"
              style={{ width: "auto", padding: "6px 10px", fontSize: 12 }}
              value={windowDays}
              onChange={(e) => setWindowDays(Number(e.target.value))}
            >
              {[7, 14, 30, 60, 90].map((d) => (
                <option key={d} value={d}>{d} days</option>
              ))}
            </select>
          </div>
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => load()}
            disabled={loading}
          >
            <RefreshCw size={13} style={loading ? { animation: "spin 1s linear infinite" } : {}} />
            {loading ? "Loading…" : "Refresh"}
          </button>
        </div>
      </div>

      {/* ── KPI Stats ────────────────────────────────────────────────── */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(6, 1fr)", gap: "var(--sp-3)", marginBottom: "var(--sp-5)" }}>
        {kpiCards.map((k) => {
          const Icon = k.icon;
          return (
            <div key={k.label} className={`stat-card ${k.color}`} style={{ padding: "var(--sp-4) var(--sp-5)" }}>
              <Icon size={16} style={{ position: "absolute", top: "var(--sp-4)", right: "var(--sp-4)", opacity: 0.2 }} />
              <div className="stat-label">{k.label}</div>
              <div className="stat-value" style={{ fontSize: 28 }}>
                {loading && !report ? (
                  <div className="skeleton" style={{ width: 48, height: 28, borderRadius: "var(--r-sm)" }} />
                ) : (
                  k.value
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* ── Quality Tab Bar ───────────────────────────────────────────── */}
      <div className="card" style={{ marginBottom: "var(--sp-4)", overflow: "visible" }}>
        <div style={{ display: "flex" }}>
          {TABS.map((tab, idx) => {
            const Icon = tab.icon;
            const count = tabCounts[tab.key] ?? 0;
            const isActive = activeTab === tab.key;
            return (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                style={{
                  flex: 1,
                  display: "flex", alignItems: "center", gap: "var(--sp-3)",
                  padding: "var(--sp-4) var(--sp-5)",
                  background: isActive ? `rgba(var(--bg-hover-raw, 99,102,241), 0.04)` : "transparent",
                  border: "none",
                  borderBottom: isActive ? `2px solid var(${tab.colorVar})` : "2px solid transparent",
                  borderRight: idx < TABS.length - 1 ? "1px solid var(--border)" : "none",
                  cursor: "pointer", transition: "all 0.15s",
                }}
              >
                <div style={{
                  width: 32, height: 32, borderRadius: "var(--r-md)", flexShrink: 0,
                  background: isActive ? `rgba(99,102,241,0.12)` : "var(--bg-elevated)",
                  display: "flex", alignItems: "center", justifyContent: "center",
                  border: "1px solid var(--border)",
                }}>
                  <Icon size={15} style={{ color: isActive ? `var(${tab.colorVar})` : "var(--text-muted)" }} />
                </div>
                <div style={{ flex: 1, textAlign: "left" }}>
                  <div style={{
                    fontSize: 13, fontWeight: 700, lineHeight: 1.2,
                    color: isActive ? `var(${tab.colorVar})` : "var(--text-secondary)",
                  }}>
                    {tab.label}
                  </div>
                </div>
                {count > 0 && (
                  <div style={{
                    minWidth: 24, height: 20, borderRadius: 10, padding: "0 6px",
                    background: isActive ? `var(${tab.colorVar})` : "var(--bg-overlay)",
                    color: isActive ? "#fff" : "var(--text-muted)",
                    fontSize: 11, fontWeight: 700,
                    display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0,
                  }}>
                    {count}
                  </div>
                )}
              </button>
            );
          })}
        </div>
      </div>

      {/* ── Tab Content ──────────────────────────────────────────────── */}
      <div className="fade-in" key={activeTab}>
        {activeTab === "noisy" && (
          <NoisyTab items={report?.noisy_alerts} loading={loading && !report} />
        )}
        {activeTab === "duplicates" && (
          <DuplicatesTab items={report?.duplicates} loading={loading && !report} />
        )}
        {activeTab === "stale" && (
          <StaleTab items={report?.stale_rules} loading={loading && !report} />
        )}
        {activeTab === "debt" && (
          <DebtTab items={report?.alert_debt} loading={loading && !report} />
        )}
      </div>

      {/* ── Footer ───────────────────────────────────────────────────── */}
      {report && (
        <div style={{
          marginTop: "var(--sp-6)", paddingTop: "var(--sp-4)",
          borderTop: "1px solid var(--border)",
          display: "flex", justifyContent: "space-between", alignItems: "center",
          fontSize: 11, color: "var(--text-muted)",
        }}>
          <span>Generated: {report.generated_at || timeAgo(lastLoaded)}</span>
          <span>{report.window_days}d analysis window · Tenant: {report.tenant_id || "default"}</span>
        </div>
      )}
    </div>
  );
}
