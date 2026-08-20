/**
 * IncidentWorkspace — Full incident management view.
 * Three tabs: New (open) · In Progress (acknowledged) · Resolved
 * All data from live API. No mocks.
 */
import { useCallback, useEffect, useState } from "react";
import {
  Activity, CheckCircle2, ChevronRight,
  Flame, FileText, MessageSquare, RefreshCw,
  ThumbsUp, Undo2, X, Zap,
} from "lucide-react";
import AnalysisPanel from "./AnalysisPanel";

/**
 * Normalise an /explain or /analyze response into the shape the UI renders.
 *
 * Note what is NOT derived here: a confidence percentage. Confidence in the
 * CAUSE lives on `provenance.rca_confidence` and is null unless something
 * actually analysed the incident. The previous version of this function read
 * `summary.confidence / 100`, which is the severity-seeded correlation score —
 * so an unanalysed critical incident rendered "75%" next to a restated alert
 * title. Provenance is the only source of truth for that number now.
 */
function buildExplanationState(raw) {
  const enriched = raw?.detail;
  const narrative = typeof raw?.explanation === "string" ? raw.explanation : null;
  return {
    narrative,
    // Empty string when the server cleared it — the observation tier asserts
    // no cause, and AnalysisPanel renders no Root Cause heading in that case.
    root_cause: enriched?.summary?.root_cause_summary || enriched?.incident?.root_cause_summary || "",
    root_cause_type: enriched?.summary?.root_cause_type || enriched?.incident?.root_cause_type || "",
    risk_score: enriched?.summary?.risk_score ?? enriched?.incident?.risk_score ?? null,
    reasoning: enriched?.incident?.reasoning || [],
    resolution_steps: enriched?.resolution_steps || [],
    evidence_refs: enriched?.evidence_refs || [],
    context_logs: enriched?.context_logs || [],
  };
}

const API = "/api/v1";

async function apiReq(path, token, opts = {}) {
  const r = await fetch(`${API}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers || {}),
    },
    ...opts,
  });
  if (!r.ok) {
    const b = await r.json().catch(() => ({}));
    throw new Error(b.error || b.message || `HTTP ${r.status}`);
  }
  const ct = r.headers.get("content-type") || "";
  return ct.includes("application/json") ? r.json() : r.text();
}

function fmtAge(iso) {
  if (!iso) return "—";
  const s = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (s < 60)    return `${s}s ago`;
  if (s < 3600)  return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function fmtTs(iso) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
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
      display: "inline-flex", alignItems: "center", gap: 4, padding: "2px 8px",
      borderRadius: 99, fontSize: 10, fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.05em",
      background: c.bg, color: c.color, flexShrink: 0,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: "50%", background: c.color }} />
      {s || "low"}
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
    <span style={{ padding: "2px 8px", borderRadius: 99, fontSize: 10, fontWeight: 600, background: c.bg, color: c.color, textTransform: "uppercase" }}>
      {s || "open"}
    </span>
  );
}

function Skel({ h = 20, w = "100%", r = 6, mb = 0 }) {
  return <div className="skeleton" style={{ height: h, width: w, borderRadius: r, marginBottom: mb }} />;
}

const STATUS_TABS = [
  { key: "open",         label: "New",         color: "var(--red)",   dim: "var(--red-dim)" },
  { key: "acknowledged", label: "In Progress", color: "var(--amber)", dim: "var(--amber-dim)" },
  { key: "resolved",     label: "Resolved",    color: "var(--green)", dim: "var(--green-dim)" },
];

const DETAIL_TABS = [
  { key: "overview",   label: "Overview",    icon: Flame },
  { key: "copilot",    label: "AI Copilot",  icon: Zap },
  { key: "timeline",   label: "Timeline",    icon: Activity },
  { key: "actions",    label: "Actions",     icon: Activity },
  { key: "postmortem", label: "Post-Mortem", icon: FileText },
];

export default function IncidentWorkspace({ token, onRefresh }) {
  const [statusTab,        setStatusTab]        = useState("open");
  const [counts,           setCounts]           = useState({ open: 0, acknowledged: 0, resolved: 0 });
  const [incidents,        setIncidents]        = useState([]);
  const [listLoading,      setListLoading]      = useState(true);
  const [filters,          setFilters]          = useState({ severity: "", service: "", page: 1 });
  const [total,            setTotal]            = useState(0);
  const [selectedId,       setSelectedId]       = useState(null);
  const [detail,           setDetail]           = useState(null);
  const [explanation,      setExplanation]      = useState(null);
  // Which tier produced the causal claim, if any. Drives whether the UI shows
  // an RCA or an "Analyse with AI" button. Null until the incident loads.
  const [provenance,       setProvenance]       = useState(null);
  const [activity,         setActivity]         = useState([]);
  const [executions,       setExecutions]       = useState([]);
  const [postmortem,       setPostmortem]       = useState(null);
  const [detailLoading,    setDetailLoading]    = useState(false);
  const [activeDetailTab,  setActiveDetailTab]  = useState("overview");
  const [copilotQ,         setCopilotQ]         = useState("");
  const [copilotResp,      setCopilotResp]      = useState(null);
  const [copilotLoading,   setCopilotLoading]   = useState(false);
  const [actionLoading,    setActionLoading]    = useState(false);
  const [lastRefreshed,    setLastRefreshed]    = useState(null);
  const [serviceOptions,   setServiceOptions]   = useState([]);

  // Called when an explicit AI analysis completes. Swaps in the new narrative
  // and provenance without refetching the incident, so the panel updates in
  // place and the badge flips from "Observed only" to "AI analysis".
  const handleAnalyzed = useCallback((result) => {
    if (!result) return;
    setExplanation(buildExplanationState(result));
    setProvenance(result?.provenance || result?.detail?.provenance || null);
  }, []);

  const fetchCounts = useCallback(async () => {
    try {
      const d = await apiReq("/incidents/counts", token);
      if (d?.counts) setCounts(d.counts);
    } catch { /**/ }
  }, [token]);

  const fetchIncidents = useCallback(async (silent = false) => {
    if (!silent) setListLoading(true);
    try {
      const params = new URLSearchParams({
        status: statusTab, page: filters.page, page_size: 30,
        sort_by: "last_event_time", sort_order: "desc",
        ...(filters.severity ? { severity: filters.severity } : {}),
        ...(filters.service  ? { service:  filters.service  } : {}),
      });
      const d = await apiReq(`/incidents?${params}`, token);
      const items = Array.isArray(d) ? d : d?.incidents || d?.items || d?.data || [];
      setIncidents(items);
      setTotal(d?.total || items.length);
      setServiceOptions([...new Set(items.map(i => i.service).filter(Boolean))]);
    } catch { /**/ }
    setListLoading(false);
    setLastRefreshed(new Date());
  }, [token, statusTab, filters]);

  const fetchDetail = useCallback(async (id) => {
    setDetailLoading(true);
    setDetail(null); setExplanation(null); setActivity([]); setExecutions([]); setPostmortem(null);
    setActiveDetailTab("overview");
    const [dR, exR, acR, execR, pmR] = await Promise.allSettled([
      apiReq(`/incidents/${id}`, token),
      apiReq(`/incidents/explain/${id}`, token),
      apiReq(`/incidents/activity/${id}`, token),
      apiReq(`/incidents/${id}/executions`, token),
      apiReq(`/incidents/${id}/postmortem`, token),
    ]);
    if (dR.status === "fulfilled") {
      const raw = dR.value;
      // Backend returns IncidentDetail: { incident: {...}, events: [...], summary: {...}, ... }
      // Flatten the nested 'incident' object to the top level so all detail.X accessors work.
      setDetail(raw?.incident ? { ...raw, ...raw.incident } : raw);
    }
    if (exR.status === "fulfilled") {
      const raw = exR.value;
      // Explain endpoint returns { explanation: "narrative string", detail: IncidentDetail }
      // Extract the enriched IncidentDetail and build a structured explanation object.
      setExplanation(buildExplanationState(raw));
      setProvenance(raw?.provenance || raw?.detail?.provenance || null);
    }
    if (acR.status   === "fulfilled") setActivity(acR.value?.items || acR.value?.activity || []);
    if (execR.status === "fulfilled") setExecutions(execR.value?.executions || execR.value?.items || []);
    if (pmR.status   === "fulfilled") setPostmortem(pmR.value);
    setDetailLoading(false);
  }, [token]);

  useEffect(() => { fetchCounts(); fetchIncidents(); }, [fetchCounts, fetchIncidents]);

  // Tab change: reset selection + filters
  const handleTabChange = (tab) => {
    setStatusTab(tab);
    setSelectedId(null);
    setFilters({ severity: "", service: "", page: 1 });
  };

  useEffect(() => {
    if (selectedId) fetchDetail(selectedId);
  }, [selectedId, fetchDetail]);

  async function updateStatus(action) {
    if (!selectedId) return;
    setActionLoading(true);
    try {
      await apiReq(`/incidents/${selectedId}/${action}`, token, { method: "POST" });
      await Promise.all([fetchCounts(), fetchIncidents(true)]);
      const d = await apiReq(`/incidents/${selectedId}`, token).catch(() => null);
      if (d) setDetail(d);
      onRefresh?.();
    } catch { /**/ }
    setActionLoading(false);
  }

  async function askCopilot() {
    if (!copilotQ.trim() || !selectedId) return;
    setCopilotLoading(true); setCopilotResp(null);
    try {
      const r = await apiReq(`/incidents/copilot/${selectedId}`, token, {
        method: "POST",
        body: JSON.stringify({ question: copilotQ }),
      });
      setCopilotResp(r?.answer || r?.response || JSON.stringify(r));
    } catch (e) { setCopilotResp(`Error: ${e.message}`); }
    setCopilotLoading(false);
  }

  /* ── Left panel ─────────────────────────────────────────────────── */
  return (
    <div style={{ display: "flex", height: "calc(100vh - var(--topbar-h))", overflow: "hidden" }}>

      {/* Incident list */}
      <div style={{
        width: 330, minWidth: 330, display: "flex", flexDirection: "column",
        background: "var(--surface-1)", borderRight: "1px solid var(--border)",
        height: "100%", overflow: "hidden",
      }}>
        {/* Status tabs */}
        <div style={{ display: "flex", borderBottom: "1px solid var(--border)", flexShrink: 0 }}>
          {STATUS_TABS.map((tab, idx) => {
            const count = counts[tab.key] || 0;
            const active = statusTab === tab.key;
            return (
              <button key={tab.key} onClick={() => handleTabChange(tab.key)} style={{
                flex: 1, display: "flex", flexDirection: "column", alignItems: "center",
                padding: "10px 4px", background: active ? tab.dim : "transparent",
                border: "none", borderBottom: `2px solid ${active ? tab.color : "transparent"}`,
                borderRight: idx < STATUS_TABS.length - 1 ? "1px solid var(--border)" : "none",
                cursor: "pointer", transition: "all 0.14s",
              }}>
                <span style={{ fontSize: 18, fontWeight: 800, color: active ? tab.color : "var(--t3)", lineHeight: 1 }}>{count}</span>
                <span style={{ fontSize: 9, fontWeight: 700, color: active ? tab.color : "var(--t4)", textTransform: "uppercase", letterSpacing: "0.07em", marginTop: 2 }}>{tab.label}</span>
              </button>
            );
          })}
        </div>

        {/* Filters */}
        <div style={{ padding: "8px 10px", borderBottom: "1px solid var(--border)", display: "flex", gap: 6, flexShrink: 0 }}>
          <select className="form-select" value={filters.severity}
            onChange={e => setFilters(f => ({ ...f, severity: e.target.value, page: 1 }))}
            style={{ flex: 1, height: 30, fontSize: 11, padding: "0 26px 0 8px" }}>
            <option value="">All severity</option>
            {["critical","high","medium","low"].map(s => <option key={s} value={s}>{s[0].toUpperCase()+s.slice(1)}</option>)}
          </select>
          <select className="form-select" value={filters.service}
            onChange={e => setFilters(f => ({ ...f, service: e.target.value, page: 1 }))}
            style={{ flex: 1, height: 30, fontSize: 11, padding: "0 26px 0 8px" }}>
            <option value="">All services</option>
            {serviceOptions.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <button onClick={() => fetchIncidents(true)} title="Refresh"
            style={{ width: 30, height: 30, flexShrink: 0, border: "1px solid var(--border)", background: "var(--surface-2)", borderRadius: "var(--r2)", display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer", color: "var(--t3)" }}>
            <RefreshCw size={11} />
          </button>
        </div>

        {/* List */}
        <div style={{ flex: 1, overflowY: "auto" }}>
          {listLoading
            ? Array.from({ length: 5 }).map((_, i) => (
                <div key={i} style={{ padding: "11px 12px", borderBottom: "1px solid var(--border)" }}>
                  <Skel h={12} w="65%" mb={6} />
                  <Skel h={9} w="45%" />
                </div>
              ))
            : incidents.length === 0
            ? <div className="empty-state" style={{ padding: "40px 16px" }}>
                <div className="empty-icon"><CheckCircle2 size={20} color="var(--green)" /></div>
                <div className="empty-title">{statusTab === "open" ? "No open incidents" : statusTab === "acknowledged" ? "None in progress" : "No resolved"}</div>
                <div className="empty-desc">Your system looks good.</div>
              </div>
            : incidents.map(inc => {
                const active = selectedId === inc.id;
                return (
                  <button key={inc.id} onClick={() => setSelectedId(inc.id)} style={{
                    width: "100%", display: "flex", flexDirection: "column", gap: 4,
                    padding: "11px 12px", textAlign: "left", border: "none",
                    borderBottom: "1px solid var(--border)",
                    borderLeft: `3px solid ${active ? "var(--blue)" : "transparent"}`,
                    background: active ? "var(--blue-dim)" : "transparent",
                    cursor: "pointer", transition: "background 0.12s",
                  }}
                  onMouseEnter={e => { if (!active) e.currentTarget.style.background = "rgba(255,255,255,0.025)"; }}
                  onMouseLeave={e => { if (!active) e.currentTarget.style.background = "transparent"; }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "space-between" }}>
                      <SevBadge s={inc.severity} />
                      <span style={{ fontSize: 10, color: "var(--t4)" }}>{fmtAge(inc.last_event_time)}</span>
                    </div>
                    <div style={{ fontSize: 12, fontWeight: 600, color: active ? "var(--t1)" : "var(--t2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {inc.title || `Incident #${inc.id}`}
                    </div>
                    <div style={{ fontSize: 11, color: "var(--t3)", display: "flex", gap: 6, overflow: "hidden" }}>
                      <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{inc.service || "—"}</span>
                      {inc.seen_before && <span style={{ color: "var(--cyan)", flexShrink: 0 }}>recurring</span>}
                    </div>
                  </button>
                );
              })
          }
          {!listLoading && total > 30 && (
            <div style={{ display: "flex", gap: 6, padding: "8px 12px", borderTop: "1px solid var(--border)", justifyContent: "center" }}>
              <button className="btn btn-ghost btn-xs" disabled={filters.page <= 1} onClick={() => setFilters(f => ({ ...f, page: f.page - 1 }))}>Prev</button>
              <span style={{ fontSize: 11, color: "var(--t3)", alignSelf: "center" }}>pg {filters.page}</span>
              <button className="btn btn-ghost btn-xs" disabled={incidents.length < 30} onClick={() => setFilters(f => ({ ...f, page: f.page + 1 }))}>Next</button>
            </div>
          )}
        </div>

        <div style={{ padding: "6px 12px", borderTop: "1px solid var(--border)", flexShrink: 0, fontSize: 10, color: "var(--t4)" }}>
          {total} incident{total !== 1 ? "s" : ""} · {lastRefreshed ? lastRefreshed.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—"}
        </div>
      </div>

      {/* Detail panel */}
      {!selectedId ? (
        <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <div className="empty-state">
            <div className="empty-icon" style={{ width: 56, height: 56 }}><Flame size={24} color="var(--t4)" /></div>
            <div className="empty-title">Select an incident</div>
            <div className="empty-desc">Click any incident on the left to see details, AI analysis, and manage its lifecycle.</div>
          </div>
        </div>
      ) : (
        <div style={{ flex: 1, display: "flex", flexDirection: "column", minWidth: 0, overflow: "hidden" }}>
          {/* Header */}
          <div style={{ padding: "14px 22px", borderBottom: "1px solid var(--border)", background: "var(--surface-1)", flexShrink: 0 }}>
            {detailLoading
              ? <div style={{ display: "flex", flexDirection: "column", gap: 8 }}><Skel h={18} w="60%" /><Skel h={12} w="40%" /></div>
              : detail && (
                <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 12 }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 7, marginBottom: 6, flexWrap: "wrap" }}>
                      <SevBadge s={detail.severity} />
                      <StatusBadge s={detail.status} />
                      {detail.seen_before && <span style={{ fontSize: 10, color: "var(--cyan)", background: "var(--cyan-dim)", padding: "2px 7px", borderRadius: 99, fontWeight: 600, textTransform: "uppercase" }}>Recurring</span>}
                    </div>
                    <h2 style={{ fontSize: 15, fontWeight: 700, color: "var(--t1)", letterSpacing: "-0.02em", margin: 0, lineHeight: 1.4, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {detail.title || `Incident #${detail.id}`}
                    </h2>
                    <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 4 }}>
                      {detail.service || "—"} · First: {fmtTs(detail.first_event_time)} · Last: {fmtTs(detail.last_event_time)}
                    </div>
                  </div>
                  <div style={{ display: "flex", gap: 6, flexShrink: 0, alignItems: "center" }}>
                    {detail.status === "open" && (
                      <button className="btn btn-ghost btn-sm" onClick={() => updateStatus("ack")} disabled={actionLoading} style={{ gap: 5 }}>
                        {actionLoading ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} /> : <ThumbsUp size={12} />}
                        Acknowledge
                      </button>
                    )}
                    {(detail.status === "open" || detail.status === "acknowledged") && (
                      <button className="btn btn-success btn-sm" onClick={() => updateStatus("resolve")} disabled={actionLoading} style={{ gap: 5 }}>
                        {actionLoading ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} /> : <CheckCircle2 size={12} />}
                        Resolve
                      </button>
                    )}
                    {detail.status === "resolved" && (
                      <button className="btn btn-danger btn-sm" onClick={() => updateStatus("reopen")} disabled={actionLoading} style={{ gap: 5 }}>
                        {actionLoading ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} /> : <Undo2 size={12} />}
                        Reopen
                      </button>
                    )}
                    <button onClick={() => setSelectedId(null)} style={{ width: 26, height: 26, border: "1px solid var(--border)", background: "var(--surface-2)", borderRadius: "var(--r2)", display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer", color: "var(--t3)" }}>
                      <X size={12} />
                    </button>
                  </div>
                </div>
              )
            }
          </div>

          {/* Detail tabs */}
          <div style={{ display: "flex", borderBottom: "1px solid var(--border)", background: "var(--surface-1)", flexShrink: 0, padding: "0 22px" }}>
            {DETAIL_TABS.map(tab => {
              const Icon = tab.icon;
              const active = activeDetailTab === tab.key;
              return (
                <button key={tab.key} onClick={() => setActiveDetailTab(tab.key)} style={{
                  display: "flex", alignItems: "center", gap: 5,
                  padding: "9px 12px", fontSize: 12, fontWeight: active ? 600 : 500,
                  color: active ? "var(--t1)" : "var(--t3)",
                  background: "none", border: "none", cursor: "pointer",
                  borderBottom: `2px solid ${active ? "var(--blue)" : "transparent"}`,
                  marginBottom: -1, transition: "all 0.14s", whiteSpace: "nowrap",
                }}>
                  <Icon size={12} /> {tab.label}
                </button>
              );
            })}
          </div>

          {/* Tab body */}
          <div style={{ flex: 1, overflowY: "auto", padding: "18px 22px" }}>
            {detailLoading
              ? <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
                  <Skel h={80} r={10} /><Skel h={60} r={10} /><Skel h={100} r={10} />
                </div>
              : activeDetailTab === "overview"
              ? <OverviewTab detail={detail} explanation={explanation} provenance={provenance} onAnalyzed={handleAnalyzed} />
              : activeDetailTab === "copilot"
              ? <CopilotTab question={copilotQ} setQuestion={setCopilotQ} response={copilotResp} loading={copilotLoading} onAsk={askCopilot} />
              : activeDetailTab === "timeline"
              ? <TimelineTab activity={activity} />
              : activeDetailTab === "actions"
              ? <ActionsTab executions={executions} />
              : activeDetailTab === "postmortem"
              ? <PostMortemTab postmortem={postmortem} onGenerate={async () => {
                  try {
                    await apiReq(`/incidents/${selectedId}/postmortem/generate`, token, { method: "POST" });
                    const pm = await apiReq(`/incidents/${selectedId}/postmortem`, token);
                    setPostmortem(pm);
                  } catch { /**/ }
                }} />
              : null
            }
          </div>
        </div>
      )}
    </div>
  );
}

/* ── Sub-tab components ──────────────────────────────────────────── */

function OverviewTab({ detail, explanation, provenance, onAnalyzed }) {
  if (!detail) return null;

  const coreAttrs = [
    { label: "Incident ID",      value: detail.id || "—" },
    { label: "Service",          value: detail.service || "—" },
    { label: "Priority Score",   value: detail.priority_score != null ? detail.priority_score : "—" },
    { label: "Risk Score",       value: detail.risk_score != null ? `${detail.risk_score}/100` : "—" },
    { label: "Confidence",       value: detail.confidence != null ? `${detail.confidence}%` : "—" },
    { label: "Event Count",      value: detail.event_count != null ? detail.event_count : "—" },
    { label: "Impact Count",     value: detail.impact_count != null ? detail.impact_count : "—" },
    { label: "Correlation",      value: detail.correlation_pattern ? `${detail.correlation_pattern} (score: ${detail.correlation_score ?? "—"})` : "—" },
    { label: "First Event",      value: detail.first_event_time ? new Date(detail.first_event_time).toLocaleString() : "—" },
    { label: "Last Event",       value: detail.last_event_time  ? new Date(detail.last_event_time).toLocaleString()  : "—" },
    { label: "Recurrence",       value: detail.seen_before ? `Yes — seen ${detail.recurring_count || 1}× before` : "First occurrence" },
  ];

  const impacted  = Array.isArray(detail.impacted_services) ? detail.impacted_services.filter(Boolean) : [];
  const reasoning = Array.isArray(detail.reasoning) ? detail.reasoning.filter(Boolean) : [];
  const hasWhatChanged = !!(detail.what_changed_type || detail.what_changed_service || detail.what_changed_version);
  const hasRCA = explanation && (explanation.root_cause || explanation.narrative);
  const riskColor = (score) => score > 70 ? "var(--red)" : score > 40 ? "var(--amber)" : "var(--green)";

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>

      {/* ── Analysis: RCA when one exists, evidence + Analyse button when not ──
          Replaces the old unconditional "Root Cause Summary" block, which
          rendered detail.root_cause_summary regardless of whether anything had
          analysed the incident. On the observation tier that field holds the
          alert title, so the UI was presenting a symptom as a diagnosis. */}
      <AnalysisPanel
        incidentId={detail.id}
        detail={detail}
        explanation={explanation}
        provenance={provenance}
        onAnalyzed={onAnalyzed}
      />

      {/* ── Core incident details grid ── */}
      <div style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10, padding: "14px 16px" }}>
        <div style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 12 }}>
          Incident Details
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "10px 20px" }}>
          {coreAttrs.map(({ label, value }) => (
            <div key={label}>
              <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.07em" }}>{label}</div>
              <div style={{ fontSize: 12, color: "var(--t2)", fontWeight: 500, marginTop: 2, wordBreak: "break-word" }}>{String(value)}</div>
            </div>
          ))}
        </div>

        {/* Impacted services */}
        {impacted.length > 0 && (
          <div style={{ marginTop: 12, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
            <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.07em", marginBottom: 7 }}>
              Impacted Services ({impacted.length})
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 5 }}>
              {impacted.map(svc => (
                <span key={svc} style={{ fontSize: 11, padding: "2px 8px", borderRadius: 99, background: "var(--red-dim)", border: "1px solid rgba(255,80,80,0.2)", color: "var(--red)" }}>
                  {svc}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Correlation reasoning bullets */}
        {reasoning.length > 0 && (
          <div style={{ marginTop: 12, paddingTop: 10, borderTop: "1px solid var(--border)" }}>
            <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.07em", marginBottom: 7 }}>
              Correlation Reasoning
            </div>
            {reasoning.map((r, i) => (
              <div key={i} style={{ display: "flex", gap: 8, marginBottom: 5 }}>
                <span style={{ width: 14, height: 14, borderRadius: 3, background: "var(--surface-3)", color: "var(--t3)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 9, fontWeight: 700, flexShrink: 0, marginTop: 2 }}>
                  {i + 1}
                </span>
                <span style={{ fontSize: 12, color: "var(--t2)", lineHeight: 1.6 }}>{r}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── What Changed (Change Intelligence) ── */}
      {hasWhatChanged && (
        <div style={{ background: "rgba(0,210,160,0.06)", border: "1px solid rgba(0,210,160,0.22)", borderRadius: 10, padding: "12px 16px" }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: "var(--cyan)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 10 }}>
            What Changed
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "8px 16px" }}>
            {[
              { label: "Type",        value: detail.what_changed_type },
              { label: "Service",     value: detail.what_changed_service },
              { label: "Version",     value: detail.what_changed_version },
              { label: "Description", value: detail.what_changed_description },
            ].filter(f => f.value).map(({ label, value }) => (
              <div key={label}>
                <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 600, textTransform: "uppercase" }}>{label}</div>
                <div style={{ fontSize: 12, color: "var(--t2)", marginTop: 2 }}>{value}</div>
              </div>
            ))}
          </div>
          {detail.what_changed_confidence > 0 && (
            <div style={{ marginTop: 8, fontSize: 11, color: "var(--t3)" }}>
              Correlation confidence: <span style={{ color: "var(--cyan)", fontWeight: 700 }}>{detail.what_changed_confidence}%</span>
            </div>
          )}
        </div>
      )}

      {/* ── AI Root Cause Analysis (LLM-powered, from explain endpoint) ── */}
      {hasRCA && (
        <div style={{ background: "var(--blue-dim)", border: "1px solid rgba(0,102,255,0.18)", borderRadius: 10, padding: "14px 16px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 12 }}>
            <Zap size={12} color="var(--blue-lt)" />
            <div style={{ fontSize: 10, fontWeight: 700, color: "var(--blue-lt)", textTransform: "uppercase", letterSpacing: "0.08em" }}>
              AI Root Cause Analysis
            </div>
          </div>

          {explanation.root_cause && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 10, color: "rgba(100,160,255,0.7)", fontWeight: 600, marginBottom: 5, textTransform: "uppercase", letterSpacing: "0.06em" }}>Root Cause</div>
              <div style={{ fontSize: 13, color: "var(--t1)", lineHeight: 1.75 }}>{explanation.root_cause}</div>
            </div>
          )}

          {explanation.narrative && explanation.narrative !== explanation.root_cause && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 10, color: "rgba(100,160,255,0.7)", fontWeight: 600, marginBottom: 5, textTransform: "uppercase", letterSpacing: "0.06em" }}>Analysis Narrative</div>
              <div style={{ fontSize: 12, color: "var(--t2)", lineHeight: 1.75 }}>{explanation.narrative}</div>
            </div>
          )}

          {explanation.resolution_steps?.length > 0 && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ fontSize: 10, color: "rgba(100,160,255,0.7)", fontWeight: 600, marginBottom: 7, textTransform: "uppercase", letterSpacing: "0.06em" }}>
                Recommended Resolution Steps
              </div>
              {explanation.resolution_steps.map((step, i) => (
                <div key={i} style={{ display: "flex", gap: 9, marginBottom: 7 }}>
                  <span style={{ width: 18, height: 18, borderRadius: 4, background: "rgba(0,102,255,0.18)", color: "var(--blue-lt)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 9, fontWeight: 700, flexShrink: 0, marginTop: 1 }}>
                    {i + 1}
                  </span>
                  <span style={{ fontSize: 12, color: "var(--t2)", lineHeight: 1.65 }}>{step}</span>
                </div>
              ))}
            </div>
          )}

          {explanation.reasoning?.length > 0 && (
            <div style={{ marginBottom: 10 }}>
              <div style={{ fontSize: 10, color: "rgba(100,160,255,0.7)", fontWeight: 600, marginBottom: 7, textTransform: "uppercase", letterSpacing: "0.06em" }}>
                AI Reasoning Chain
              </div>
              {explanation.reasoning.map((r, i) => (
                <div key={i} style={{ display: "flex", gap: 9, marginBottom: 6 }}>
                  <span style={{ width: 18, height: 18, borderRadius: 4, background: "rgba(0,102,255,0.18)", color: "var(--blue-lt)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 9, fontWeight: 700, flexShrink: 0, marginTop: 1 }}>
                    {i + 1}
                  </span>
                  <span style={{ fontSize: 12, color: "var(--t2)", lineHeight: 1.65 }}>{r}</span>
                </div>
              ))}
            </div>
          )}

          <div style={{ display: "flex", gap: 16, flexWrap: "wrap", paddingTop: 8, borderTop: "1px solid rgba(0,102,255,0.12)" }}>
            {explanation.confidence_score != null && (
              <div style={{ fontSize: 11, color: "var(--t3)" }}>
                Confidence: <span style={{ color: "var(--blue-lt)", fontWeight: 700 }}>{Math.round(explanation.confidence_score * 100)}%</span>
              </div>
            )}
            {explanation.risk_score != null && (
              <div style={{ fontSize: 11, color: "var(--t3)" }}>
                Risk: <span style={{ color: riskColor(explanation.risk_score), fontWeight: 700 }}>{explanation.risk_score}/100</span>
              </div>
            )}
            {explanation.root_cause_type && (
              <div style={{ fontSize: 11, color: "var(--t3)" }}>
                Type: <span style={{ color: "var(--t2)", fontWeight: 600 }}>{explanation.root_cause_type}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Evidence References ── */}
      {explanation?.evidence_refs?.length > 0 && (
        <div style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10, padding: "12px 16px" }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 10 }}>
            Evidence References ({explanation.evidence_refs.length})
          </div>
          {explanation.evidence_refs.map((ref, i) => (
            <div key={i} style={{ display: "flex", gap: 10, marginBottom: 9, alignItems: "flex-start" }}>
              <span style={{ fontSize: 10, padding: "2px 6px", borderRadius: 4, background: "var(--surface-3)", color: "var(--t3)", fontWeight: 600, flexShrink: 0, marginTop: 1, whiteSpace: "nowrap" }}>
                {ref.type}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 12, color: "var(--t1)", fontWeight: 500 }}>{ref.label}</div>
                {ref.detail && <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 2 }}>{ref.detail}</div>}
              </div>
              {ref.timestamp && (
                <div style={{ fontSize: 10, color: "var(--t4)", flexShrink: 0, marginTop: 2 }}>
                  {new Date(ref.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* ── Empty state when no data at all ── */}
      {!detail.root_cause_summary && !hasRCA && !hasWhatChanged && impacted.length === 0 && (
        <div style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10, padding: "24px 16px", textAlign: "center" }}>
          <Zap size={18} color="var(--t4)" style={{ marginBottom: 8 }} />
          <div style={{ fontSize: 12, color: "var(--t3)", marginTop: 8 }}>AI analysis is running — refresh in a moment.</div>
        </div>
      )}
    </div>
  );
}

function CopilotTab({ question, setQuestion, response, loading, onAsk }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div style={{ background: "var(--blue-dim)", border: "1px solid rgba(0,102,255,0.15)", borderRadius: 9, padding: "10px 13px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 3 }}>
          <Zap size={12} color="var(--blue-lt)" />
          <span style={{ fontSize: 11, fontWeight: 600, color: "var(--blue-lt)" }}>AI Copilot</span>
        </div>
        <div style={{ fontSize: 12, color: "var(--t3)" }}>Ask anything about this incident — root cause, impact, remediation, or similar past incidents.</div>
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <input type="text" className="form-input" value={question}
          onChange={e => setQuestion(e.target.value)}
          onKeyDown={e => e.key === "Enter" && !loading && onAsk()}
          placeholder="What is the root cause? How do I fix it?"
          style={{ flex: 1 }}
        />
        <button className="btn btn-primary" onClick={onAsk} disabled={loading || !question.trim()} style={{ flexShrink: 0 }}>
          {loading ? <span className="spinner" style={{ width: 13, height: 13, borderWidth: 2 }} /> : <MessageSquare size={12} />}
          Ask
        </button>
      </div>
      {response && (
        <div style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 9, padding: "13px 14px", fontSize: 13, color: "var(--t1)", lineHeight: 1.75, whiteSpace: "pre-wrap" }}>
          {response}
        </div>
      )}
      {!response && !loading && (
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          <div style={{ fontSize: 10, color: "var(--t4)", fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.07em" }}>Suggested questions</div>
          {["What is the probable root cause?", "Which services are affected?", "What are the remediation steps?", "Have we seen this before?"].map(q => (
            <button key={q} onClick={() => setQuestion(q)} style={{
              display: "flex", alignItems: "center", gap: 8, padding: "8px 11px",
              background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 7,
              cursor: "pointer", fontSize: 12, color: "var(--t2)", textAlign: "left",
              transition: "border-color 0.14s, background 0.14s",
            }}
            onMouseEnter={e => { e.currentTarget.style.borderColor = "rgba(0,102,255,0.3)"; e.currentTarget.style.background = "var(--blue-dim)"; }}
            onMouseLeave={e => { e.currentTarget.style.borderColor = "var(--border)"; e.currentTarget.style.background = "var(--surface-2)"; }}>
              <ChevronRight size={11} color="var(--t4)" /> {q}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function TimelineTab({ activity }) {
  if (!activity.length) return (
    <div className="empty-state" style={{ padding: "28px 0" }}>
      <div className="empty-icon"><Activity size={18} color="var(--t4)" /></div>
      <div className="empty-title">No activity recorded</div>
      <div className="empty-desc">Events will appear as the incident progresses.</div>
    </div>
  );
  return (
    <div style={{ display: "flex", flexDirection: "column" }}>
      {activity.map((ev, i) => (
        <div key={ev.id || i} style={{ display: "flex", gap: 10, paddingBottom: 14 }}>
          <div style={{ display: "flex", flexDirection: "column", alignItems: "center", flexShrink: 0 }}>
            <div style={{ width: 7, height: 7, borderRadius: "50%", background: "var(--blue)", marginTop: 4 }} />
            {i < activity.length - 1 && <div style={{ flex: 1, width: 1, background: "var(--border)", margin: "3px 0" }} />}
          </div>
          <div style={{ flex: 1, paddingTop: 0 }}>
            <div style={{ fontSize: 12, fontWeight: 500, color: "var(--t1)", lineHeight: 1.5 }}>
              {ev.message || ev.description || ev.action || JSON.stringify(ev)}
            </div>
            <div style={{ fontSize: 10, color: "var(--t4)", marginTop: 3 }}>
              {ev.actor || ev.user || "system"} · {(ev.timestamp || ev.created_at) ? new Date(ev.timestamp || ev.created_at).toLocaleString() : "—"}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function ActionsTab({ executions }) {
  if (!executions.length) return (
    <div className="empty-state" style={{ padding: "28px 0" }}>
      <div className="empty-icon"><Activity size={18} color="var(--t4)" /></div>
      <div className="empty-title">No actions executed</div>
      <div className="empty-desc">Automated and manual action executions appear here.</div>
    </div>
  );
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {executions.map((ex, i) => {
        const sc = ex.status === "success" || ex.status === "completed" ? "var(--green)"
          : ex.status === "failed" ? "var(--red)"
          : ex.status === "running" ? "var(--amber)"
          : "var(--t3)";
        return (
          <div key={ex.id || i} style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 9, padding: "11px 13px" }}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 5 }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: "var(--t1)" }}>{ex.action_name || ex.type || `Action #${ex.id}`}</div>
              <span style={{ fontSize: 10, fontWeight: 700, color: sc, background: `${sc}18`, padding: "2px 7px", borderRadius: 99, textTransform: "uppercase" }}>{ex.status}</span>
            </div>
            {ex.output && <div style={{ fontSize: 11, color: "var(--t2)", lineHeight: 1.6, fontFamily: "monospace" }}>{ex.output}</div>}
            <div style={{ fontSize: 10, color: "var(--t4)", marginTop: 5 }}>{ex.executed_by || ex.actor || "agent"} · {ex.executed_at ? new Date(ex.executed_at).toLocaleString() : "—"}</div>
          </div>
        );
      })}
    </div>
  );
}

function PostMortemTab({ postmortem, onGenerate }) {
  const [generating, setGenerating] = useState(false);

  async function generate() {
    setGenerating(true);
    await onGenerate?.();
    setGenerating(false);
  }

  if (!postmortem || (!postmortem.summary && !postmortem.content)) {
    return (
      <div className="empty-state" style={{ padding: "28px 0" }}>
        <div className="empty-icon"><FileText size={18} color="var(--t4)" /></div>
        <div className="empty-title">No post-mortem yet</div>
        <div className="empty-desc">Generate an AI-powered post-mortem for this incident.</div>
        <button className="btn btn-primary btn-sm" onClick={generate} disabled={generating} style={{ marginTop: 12 }}>
          {generating ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} /> : <Zap size={12} />}
          Generate Post-Mortem
        </button>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <FileText size={13} color="var(--blue-lt)" />
          <span style={{ fontSize: 13, fontWeight: 700, color: "var(--t1)" }}>Post-Mortem Report</span>
        </div>
        <button className="btn btn-outline btn-sm" onClick={generate} disabled={generating}>
          {generating ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 2 }} /> : <RefreshCw size={11} />}
          Regenerate
        </button>
      </div>
      {[
        { label: "Summary",         value: postmortem.summary },
        { label: "Timeline",        value: postmortem.timeline },
        { label: "Root Cause",      value: postmortem.root_cause },
        { label: "Impact",          value: postmortem.impact },
        { label: "Action Items",    value: postmortem.action_items },
        { label: "Lessons Learned", value: postmortem.lessons_learned },
      ].filter(f => f.value).map(({ label, value }) => (
        <div key={label} style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 9, padding: "11px 14px" }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: 7 }}>{label}</div>
          <div style={{ fontSize: 12, color: "var(--t1)", lineHeight: 1.75, whiteSpace: "pre-wrap" }}>{value}</div>
        </div>
      ))}
    </div>
  );
}
