import { useState, useCallback, useEffect } from "react";

// ─── Constants ────────────────────────────────────────────────────────────────

const CHANGE_TYPE_STYLE = {
  deployment:    { icon: "🚀", color: "#34d399", label: "Deployment" },
  rollback:      { icon: "↩️", color: "#f87171", label: "Rollback" },
  release:       { icon: "🏷️", color: "#a78bfa", label: "Release" },
  commit:        { icon: "📝", color: "#94a3b8", label: "Commit" },
  infra:         { icon: "🏗️", color: "#38bdf8", label: "Infra" },
  terraform:     { icon: "🏗️", color: "#38bdf8", label: "Terraform" },
  config_change: { icon: "⚙️", color: "#f59e0b", label: "Config" },
  config:        { icon: "⚙️", color: "#f59e0b", label: "Config" },
  feature_flag:  { icon: "🚩", color: "#fb923c", label: "Feature Flag" },
  config_drift:  { icon: "⚠️", color: "#ef4444", label: "Config Drift" },
};

const DRIFT_SEVERITY_COLOR = {
  critical: "#f87171",
  high:     "#f59e0b",
  medium:   "#38bdf8",
  low:      "#64748b",
};

const SOURCE_BADGE = {
  github:       { label: "GitHub",  color: "#94a3b8" },
  gitlab:       { label: "GitLab",  color: "#fb923c" },
  terraform:    { label: "TF",      color: "#a78bfa" },
  manual:       { label: "Manual",  color: "#64748b" },
  feature_flag: { label: "Flags",   color: "#fb923c" },
};

function authHeaders() {
  const token = localStorage.getItem("authToken") || "";
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function typeStyle(t) {
  return CHANGE_TYPE_STYLE[t] || { icon: "🔄", color: "#94a3b8", label: t || "Change" };
}

function scoreBar(score) {
  const color = score >= 70 ? "#34d399" : score >= 40 ? "#f59e0b" : "#f87171";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 5, flexShrink: 0 }}>
      <div style={{ width: 40, height: 3, borderRadius: 2, background: "rgba(255,255,255,0.07)" }}>
        <div style={{ width: `${score}%`, height: "100%", borderRadius: 2, background: color }} />
      </div>
      <span style={{ fontSize: "0.65rem", fontWeight: 700, color, minWidth: 26 }}>{score}%</span>
    </div>
  );
}

// ─── Tab: "What Changed First" Causality Timeline ─────────────────────────────

function TimelineTab({ timeline, firstChange, primaryCorr }) {
  if (!timeline || timeline.length === 0) {
    return <div className="empty-state">No changes found in the correlation window (−60 min to +15 min around incident start).</div>;
  }

  return (
    <div>
      {(firstChange || primaryCorr) && (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 14 }}>
          {firstChange && (
            <div style={{ padding: "8px 12px", borderRadius: 9, background: "rgba(248,113,113,0.07)", border: "1px solid rgba(248,113,113,0.22)", flex: 1, minWidth: 180 }}>
              <div style={{ fontSize: "0.6rem", color: "#f87171", fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.07em", marginBottom: 4 }}>FIRST CHANGE</div>
              <div style={{ fontSize: "0.78rem", color: "#e2e8f0", fontWeight: 600 }}>{firstChange.title}</div>
              <div style={{ fontSize: "0.68rem", color: "#64748b", marginTop: 2 }}>{firstChange.lag_from_incident} before incident</div>
            </div>
          )}
          {primaryCorr && (
            <div style={{ padding: "8px 12px", borderRadius: 9, background: "rgba(52,211,153,0.07)", border: "1px solid rgba(52,211,153,0.22)", flex: 1, minWidth: 180 }}>
              <div style={{ fontSize: "0.6rem", color: "#34d399", fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.07em", marginBottom: 4 }}>PRIMARY CORRELATION</div>
              <div style={{ fontSize: "0.78rem", color: "#e2e8f0", fontWeight: 600 }}>{primaryCorr.service} {primaryCorr.type}</div>
              <div style={{ fontSize: "0.68rem", color: "#64748b", marginTop: 2 }}>Correlation score: {primaryCorr.correlation_score}%</div>
            </div>
          )}
        </div>
      )}

      <div style={{ position: "relative", paddingLeft: 20 }}>
        {/* Vertical timeline line */}
        <div style={{ position: "absolute", left: 6, top: 0, bottom: 0, width: 1, background: "rgba(90,123,186,0.18)" }} />

        {timeline.map((entry, i) => {
          const ts = typeStyle(entry.entry_type);
          const beforeIncident = entry.lag_seconds <= 0;
          const accentColor = entry.is_primary_correlation ? "#34d399" :
                              entry.is_first_change ? "#f87171" :
                              beforeIncident ? ts.color : "#475569";

          return (
            <div key={i} style={{ position: "relative", marginBottom: 10 }}>
              {/* Dot */}
              <div style={{ position: "absolute", left: -17, top: 8, width: 8, height: 8, borderRadius: "50%", background: accentColor, border: `1px solid ${accentColor}44`, zIndex: 1 }} />

              <div style={{ padding: "9px 12px", borderRadius: 9, background: "rgba(255,255,255,0.02)", border: `1px solid ${accentColor}22`, opacity: beforeIncident ? 1 : 0.55 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                  <span style={{ fontSize: "0.85rem" }}>{ts.icon}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: "0.78rem", fontWeight: 600, color: "#e2e8f0" }}>
                      {entry.title}
                      {entry.is_first_change && <span style={{ marginLeft: 6, fontSize: "0.58rem", fontWeight: 700, color: "#f87171", border: "1px solid rgba(248,113,113,0.3)", padding: "1px 5px", borderRadius: 4 }}>FIRST</span>}
                      {entry.is_primary_correlation && <span style={{ marginLeft: 6, fontSize: "0.58rem", fontWeight: 700, color: "#34d399", border: "1px solid rgba(52,211,153,0.3)", padding: "1px 5px", borderRadius: 4 }}>PRIMARY</span>}
                    </div>
                    {entry.description && <div style={{ fontSize: "0.68rem", color: "#475569", marginTop: 2 }}>{entry.description}</div>}
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 3, flexShrink: 0 }}>
                    {scoreBar(entry.correlation_score)}
                    <span style={{ fontSize: "0.62rem", color: beforeIncident ? "#64748b" : "#334155" }}>
                      {entry.lag_from_incident}
                    </span>
                  </div>
                </div>
                <div style={{ display: "flex", gap: 6, marginTop: 5, flexWrap: "wrap" }}>
                  {entry.service && <span style={{ fontSize: "0.62rem", color: ts.color, padding: "1px 6px", borderRadius: 4, border: `1px solid ${ts.color}33` }}>{entry.service}</span>}
                  {entry.environment && <span style={{ fontSize: "0.62rem", color: "#475569" }}>{entry.environment}</span>}
                  {entry.author && <span style={{ fontSize: "0.62rem", color: "#475569" }}>by {entry.author}</span>}
                  {entry.version && <span style={{ fontSize: "0.62rem", color: "#64748b", fontFamily: "monospace" }}>{entry.version}</span>}
                  {entry.metadata?.branch && <span style={{ fontSize: "0.62rem", color: "#64748b" }}>branch: {entry.metadata.branch}</span>}
                  <span style={{ fontSize: "0.62rem", color: "#334155", marginLeft: "auto" }}>{new Date(entry.timestamp).toLocaleString()}</span>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Tab: Deployments ─────────────────────────────────────────────────────────

function DeploymentsTab({ deployments }) {
  if (!deployments || deployments.length === 0) {
    return <div className="empty-state">No deployments or commits found in the correlation window.</div>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {deployments.map((d, i) => {
        const ts = typeStyle(d.type);
        const src = SOURCE_BADGE[d.change_source] || SOURCE_BADGE.manual;
        return (
          <div key={i} style={{ padding: "10px 14px", borderRadius: 10, background: "rgba(255,255,255,0.025)", border: `1px solid ${ts.color}22` }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 5 }}>
              <span style={{ fontSize: "0.9rem" }}>{ts.icon}</span>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: "0.8rem", fontWeight: 700, color: "#e2e8f0" }}>{d.service}</div>
                <div style={{ fontSize: "0.68rem", color: "#475569" }}>{d.description}</div>
              </div>
              {d.correlation_score > 0 && scoreBar(d.correlation_score)}
            </div>
            <div style={{ display: "flex", gap: 6, flexWrap: "wrap", fontSize: "0.65rem" }}>
              <span style={{ color: ts.color, fontWeight: 600, padding: "1px 6px", borderRadius: 4, border: `1px solid ${ts.color}33` }}>{ts.label}</span>
              <span style={{ color: src.color, padding: "1px 6px", borderRadius: 4, border: `1px solid ${src.color}33` }}>{src.label}</span>
              {d.version && <span style={{ color: "#94a3b8", fontFamily: "monospace" }}>{d.version}</span>}
              {d.commit_sha && <span style={{ color: "#475569", fontFamily: "monospace" }}>{d.commit_sha.slice(0, 8)}</span>}
              {d.author && <span style={{ color: "#475569" }}>👤 {d.author}</span>}
              {d.pr_number && <span style={{ color: "#38bdf8" }}>PR #{d.pr_number}</span>}
              {d.environment && <span style={{ color: "#64748b" }}>env: {d.environment}</span>}
              {d.changed_files_count > 0 && <span style={{ color: "#64748b" }}>{d.changed_files_count} commits</span>}
              <span style={{ color: "#334155", marginLeft: "auto" }}>{new Date(d.timestamp).toLocaleString()}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Tab: Feature Flags ───────────────────────────────────────────────────────

function FeatureFlagsTab({ flags }) {
  if (!flags || flags.length === 0) {
    return <div className="empty-state">No feature flag changes found. POST flag toggles to /api/v1/ingest/feature-flags.</div>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {flags.map((f, i) => (
        <div key={i} style={{ padding: "10px 14px", borderRadius: 10, background: "rgba(251,146,60,0.05)", border: "1px solid rgba(251,146,60,0.2)" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 5 }}>
            <span style={{ fontSize: "0.9rem" }}>🚩</span>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: "0.8rem", fontWeight: 700, color: "#e2e8f0" }}>{f.flag_name}</div>
              <div style={{ fontSize: "0.68rem", color: "#475569", fontFamily: "monospace" }}>{f.flag_key}</div>
            </div>
            {f.correlation_score > 0 && scoreBar(f.correlation_score)}
          </div>
          <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap", fontSize: "0.7rem", marginBottom: 4 }}>
            <span style={{ color: "#f87171", background: "rgba(248,113,113,0.10)", padding: "2px 8px", borderRadius: 5, fontFamily: "monospace" }}>{f.old_value || "—"}</span>
            <span style={{ color: "#64748b" }}>→</span>
            <span style={{ color: "#34d399", background: "rgba(52,211,153,0.10)", padding: "2px 8px", borderRadius: 5, fontFamily: "monospace" }}>{f.new_value || "—"}</span>
            <span style={{ color: "#f59e0b", marginLeft: "auto" }}>{f.affected_pct}% traffic</span>
          </div>
          <div style={{ display: "flex", gap: 6, fontSize: "0.63rem", color: "#475569", flexWrap: "wrap" }}>
            {f.changed_by && <span>by {f.changed_by}</span>}
            <span>env: {f.environment}</span>
            {f.linked_incident_id && <span style={{ color: "#34d399" }}>→ {f.linked_incident_id}</span>}
            <span style={{ marginLeft: "auto", color: "#334155" }}>{new Date(f.timestamp).toLocaleString()}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

// ─── Tab: Config Drift ────────────────────────────────────────────────────────

function ConfigDriftTab({ drift }) {
  if (!drift || drift.length === 0) {
    return <div className="empty-state">No config drift detected. POST config snapshots to /api/v1/ingest/config-drift to enable drift detection.</div>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {drift.map((d, i) => {
        const sevColor = DRIFT_SEVERITY_COLOR[d.drift_severity] || "#94a3b8";
        return (
          <div key={i} style={{ padding: "10px 14px", borderRadius: 10, background: `${sevColor}08`, border: `1px solid ${sevColor}28` }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 5, flexWrap: "wrap" }}>
              <span style={{ fontSize: "0.9rem" }}>⚠️</span>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: "0.8rem", fontWeight: 700, color: "#e2e8f0" }}>{d.config_key}</div>
                <div style={{ fontSize: "0.67rem", color: "#475569" }}>{d.service}</div>
              </div>
              <span style={{ fontSize: "0.62rem", fontWeight: 700, color: sevColor, padding: "1px 7px", borderRadius: 5, border: `1px solid ${sevColor}44`, textTransform: "uppercase" }}>
                {d.drift_severity}
              </span>
            </div>
            <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap", fontSize: "0.7rem", marginBottom: 4 }}>
              <div>
                <span style={{ fontSize: "0.6rem", color: "#64748b", textTransform: "uppercase", marginRight: 4 }}>Baseline</span>
                <span style={{ color: "#94a3b8", fontFamily: "monospace", background: "rgba(255,255,255,0.04)", padding: "1px 6px", borderRadius: 4 }}>{d.baseline_value || "—"}</span>
              </div>
              <span style={{ color: "#334155" }}>→</span>
              <div>
                <span style={{ fontSize: "0.6rem", color: "#64748b", textTransform: "uppercase", marginRight: 4 }}>Current</span>
                <span style={{ color: sevColor, fontFamily: "monospace", background: `${sevColor}10`, padding: "1px 6px", borderRadius: 4 }}>{d.current_value || "—"}</span>
              </div>
            </div>
            <div style={{ fontSize: "0.63rem", color: "#475569" }}>
              Detected: {new Date(d.detected_at).toLocaleString()}
              {d.linked_incident_id && <span style={{ color: "#34d399", marginLeft: 8 }}>→ {d.linked_incident_id}</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Main export ──────────────────────────────────────────────────────────────

const TABS = [
  { key: "timeline",     label: "Timeline" },
  { key: "deployments",  label: "Deployments" },
  { key: "flags",        label: "Feature Flags" },
  { key: "drift",        label: "Config Drift" },
];

export default function ChangeIntelligencePanel({ incidentID }) {
  const [report, setReport] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [tab, setTab] = useState("timeline");
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!incidentID) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch(`/api/v1/incidents/${incidentID}/change-intelligence`, {
        headers: authHeaders(),
      });
      if (!res.ok) throw new Error("Failed to load change intelligence report");
      setReport(await res.json());
      setLoaded(true);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [incidentID]);

  useEffect(() => {
    if (!loaded && incidentID) {
      load();
    }
  }, [incidentID, loaded, load]);

  // Badge counts per tab
  function tabCount(key) {
    if (!report) return 0;
    switch (key) {
      case "timeline":    return report.causality_timeline?.length || 0;
      case "deployments": return (report.deployments?.length || 0) + (report.infra_changes?.length || 0);
      case "flags":       return report.feature_flag_changes?.length || 0;
      case "drift":       return report.config_drift?.length || 0;
      default:            return 0;
    }
  }

  return (
    <div className="panel">
      <div className="panel-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h3>Change Intelligence</h3>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {report && (
            <span style={{ fontSize: "0.65rem", color: "#64748b" }}>
              Confidence: <span style={{ color: report.overall_confidence >= 60 ? "#34d399" : "#f59e0b", fontWeight: 700 }}>{report.overall_confidence}%</span>
            </span>
          )}
          <button onClick={load} disabled={loading} style={{ fontSize: "0.65rem", padding: "3px 10px", borderRadius: 6, cursor: "pointer", background: "transparent", color: "#64748b", border: "1px solid rgba(90,123,186,0.22)" }}>
            {loading ? "Loading…" : "Refresh"}
          </button>
        </div>
      </div>

      {error && <div className="error-text" style={{ marginTop: 8 }}>{error}</div>}
      {loading && <div className="empty-state">Assembling change intelligence report…</div>}

      {report && !loading && (
        <>
          {/* Window badge */}
          <div style={{ fontSize: "0.63rem", color: "#334155", marginBottom: 12 }}>
            Window: {new Date(report.window_start).toLocaleString()} → {new Date(report.window_end).toLocaleString()}
          </div>

          {/* Tabs */}
          <div style={{ display: "flex", gap: 4, marginBottom: 14, flexWrap: "wrap" }}>
            {TABS.map(t => {
              const count = tabCount(t.key);
              const active = tab === t.key;
              return (
                <button
                  key={t.key}
                  onClick={() => setTab(t.key)}
                  style={{
                    padding: "4px 12px", borderRadius: 6, cursor: "pointer", fontSize: "0.72rem", fontWeight: active ? 700 : 400,
                    background: active ? "rgba(58,167,255,0.12)" : "transparent",
                    color: active ? "#3aa7ff" : "#64748b",
                    border: active ? "1px solid rgba(58,167,255,0.28)" : "1px solid rgba(90,123,186,0.18)",
                  }}
                >
                  {t.label}
                  {count > 0 && (
                    <span style={{ marginLeft: 5, fontSize: "0.6rem", fontWeight: 700, color: active ? "#3aa7ff" : "#475569" }}>
                      {count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Tab content */}
          {tab === "timeline" && (
            <TimelineTab
              timeline={report.causality_timeline}
              firstChange={report.first_change}
              primaryCorr={report.primary_correlation}
            />
          )}
          {tab === "deployments" && (
            <DeploymentsTab deployments={[...(report.deployments || []), ...(report.infra_changes || [])]} />
          )}
          {tab === "flags" && (
            <FeatureFlagsTab flags={report.feature_flag_changes} />
          )}
          {tab === "drift" && (
            <ConfigDriftTab drift={report.config_drift} />
          )}
        </>
      )}

      {!report && !loading && !error && (
        <div className="empty-state">Loading change intelligence…</div>
      )}
    </div>
  );
}
