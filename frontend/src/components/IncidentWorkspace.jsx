/**
 * IncidentWorkspace — Full incident management view.
 *
 * Master–detail: a triaged queue on the left, the selected incident on the
 * right. That shape is right for the job and is kept; what changed is that it
 * now draws from the shared UI kit instead of ~150 inline style objects, so
 * severity colours, rows, pills and empty states match the rest of the product.
 *
 * Detail hierarchy, in the order an on-call engineer needs it:
 *   1. What is broken and how bad          (header: severity, status, title)
 *   2. Why — the analysis                  (AnalysisPanel, first in Overview)
 *   3. What changed just before it broke   (change intelligence)
 *   4. Supporting facts                    (attributes, evidence)
 * All data from live API. No mocks.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity, BellOff, CheckCircle2, ChevronRight, FileText, Flame, MessageSquare,
  RefreshCw, ThumbsUp, Undo2, X, Zap,
} from "lucide-react";

import AnalysisPanel from "./AnalysisPanel";
import MuteDialog from "./MuteDialog.jsx";
import { roleFromToken } from "../api/auth.js";
import {
  EmptyState, ErrorState, SeverityPill, SkeletonRows, StatusPill,
} from "./ui/Primitives.jsx";

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
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function fmtTs(iso) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString([], {
    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit",
  });
}

const STATUS_TABS = [
  { key: "open", label: "New", tone: "tone-danger" },
  { key: "acknowledged", label: "In progress", tone: "tone-warning" },
  { key: "resolved", label: "Resolved", tone: "tone-success" },
];

const DETAIL_TABS = [
  { key: "overview", label: "Overview", icon: Flame },
  { key: "copilot", label: "AI Copilot", icon: Zap },
  { key: "timeline", label: "Timeline", icon: Activity },
  { key: "actions", label: "Actions", icon: Activity },
  { key: "postmortem", label: "Post-mortem", icon: FileText },
];

/* A titled block used throughout the detail tabs. `tone` tints the frame for
   blocks that carry meaning (analysis = brand, change = info). */
function DetailBlock({ title, icon: Icon, tone = "", count, children }) {
  return (
    <section className={`detail-block ${tone}`}>
      <h3 className="detail-block-title">
        {Icon && <Icon size={12} />}
        {title}
        {count != null && <span className="detail-block-count">{count}</span>}
      </h3>
      {children}
    </section>
  );
}

/* An ordered list of steps or reasoning bullets. */
function StepList({ items, tone = "" }) {
  return (
    <ol className={`step-list ${tone}`}>
      {items.map((item, i) => (
        <li key={i} className="step-list-item">
          <span className="step-list-index">{i + 1}</span>
          <span className="step-list-text">{item}</span>
        </li>
      ))}
    </ol>
  );
}

export default function IncidentWorkspace({ token, onRefresh }) {
  const [statusTab, setStatusTab] = useState("open");
  const [counts, setCounts] = useState({ open: 0, acknowledged: 0, resolved: 0 });
  const [incidents, setIncidents] = useState([]);
  // Loading states are derived, not stored: the list is loading until a fetch
  // for the current tab + filters has finished, the detail until one for the
  // selected incident has. The refs drop responses that arrive after the user
  // has moved on, so a slow reply never overwrites what is on screen.
  const [loadedListKey, setLoadedListKey] = useState(null);
  const listRequest = useRef(0);
  const [listError, setListError] = useState("");
  const [filters, setFilters] = useState({ severity: "", service: "", page: 1 });
  const [total, setTotal] = useState(0);
  const [selectedId, setSelectedId] = useState(null);
  const [detail, setDetail] = useState(null);
  const [explanation, setExplanation] = useState(null);
  // Which tier produced the causal claim, if any. Drives whether the UI shows
  // an RCA or an "Analyse with AI" button. Null until the incident loads.
  const [provenance, setProvenance] = useState(null);
  const [activity, setActivity] = useState([]);
  const [executions, setExecutions] = useState([]);
  const [postmortem, setPostmortem] = useState(null);
  const [loadedDetailId, setLoadedDetailId] = useState(null);
  const detailRequest = useRef(null);
  const [activeDetailTab, setActiveDetailTab] = useState("overview");
  const [copilotQ, setCopilotQ] = useState("");
  const [copilotResp, setCopilotResp] = useState(null);
  const [copilotLoading, setCopilotLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [lastRefreshed, setLastRefreshed] = useState(null);
  const [serviceOptions, setServiceOptions] = useState([]);
  // { eventId } or { incidentId } while the mute form is open.
  const [muteTarget, setMuteTarget] = useState(null);
  const [muteNotice, setMuteNotice] = useState("");
  const isAdmin = roleFromToken(token) === "admin";
  const listLoading = loadedListKey !== JSON.stringify([statusTab, filters]);
  const detailLoading = selectedId !== null && loadedDetailId !== selectedId;

  const handleMuted = useCallback((mute) => {
    const ends = new Date(mute.ends_at).toLocaleString([], { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
    setMuteNotice(`Muted. Matching alerts will be logged instead of reaching incidents until ${ends}.`);
    setTimeout(() => setMuteNotice(""), 8000);
  }, []);

  // Called when an explicit AI analysis completes. Swaps in the new narrative
  // and provenance without refetching the incident, so the panel updates in
  // place and the badge flips from "Observed only" to "AI analysis".
  const handleAnalyzed = useCallback((result) => {
    if (!result) return;
    setExplanation(buildExplanationState(result));
    setProvenance(result?.provenance || result?.detail?.provenance || null);
  }, []);

  // The loaders are promise chains rather than async/await on purpose: state
  // is only set in .then() callbacks, which is what lets the effects below
  // call them (react-hooks/set-state-in-effect treats code after an await as
  // synchronous).
  const fetchCounts = useCallback(
    () =>
      apiReq("/incidents/counts", token)
        .then((d) => {
          if (d?.counts) setCounts(d.counts);
        })
        .catch(() => {
          /* counts are decorative — a failure must not blank the queue */
        }),
    [token],
  );

  const fetchIncidents = useCallback(() => {
    const request = ++listRequest.current;
    const key = JSON.stringify([statusTab, filters]);
    const params = new URLSearchParams({
      status: statusTab,
      page: filters.page,
      page_size: 30,
      sort_by: "last_event_time",
      sort_order: "desc",
      ...(filters.severity ? { severity: filters.severity } : {}),
      ...(filters.service ? { service: filters.service } : {}),
    });
    return apiReq(`/incidents?${params}`, token).then(
      (d) => {
        if (request !== listRequest.current) return;
        const items = Array.isArray(d) ? d : d?.incidents || d?.items || d?.data || [];
        setIncidents(items);
        setTotal(d?.total || items.length);
        setServiceOptions([...new Set(items.map((i) => i.service).filter(Boolean))]);
        setListError("");
        setLoadedListKey(key);
        setLastRefreshed(new Date());
      },
      (e) => {
        if (request !== listRequest.current) return;
        // Previously swallowed: a failed fetch left an empty list that read as
        // "no incidents", which is the most dangerous possible misreport here.
        setListError(e.message || "Could not load incidents");
        setLoadedListKey(key);
        setLastRefreshed(new Date());
      },
    );
  }, [token, statusTab, filters]);

  const fetchDetail = useCallback((id) => {
    detailRequest.current = id;
    return Promise.allSettled([
      apiReq(`/incidents/${id}`, token),
      apiReq(`/incidents/explain/${id}`, token),
      apiReq(`/incidents/activity/${id}`, token),
      apiReq(`/incidents/${id}/executions`, token),
      apiReq(`/incidents/${id}/postmortem`, token),
    ]).then(([dR, exR, acR, execR, pmR]) => {
      if (detailRequest.current !== id) return;

      // Every piece is set, including the ones that failed, so nothing from
      // the previously selected incident survives.
      const raw = dR.status === "fulfilled" ? dR.value : null;
      // Backend returns IncidentDetail: { incident: {...}, events: [...], summary: {...} }
      // Flatten the nested 'incident' object so all detail.X accessors work.
      setDetail(raw?.incident ? { ...raw, ...raw.incident } : raw);
      const ex = exR.status === "fulfilled" ? exR.value : null;
      setExplanation(ex ? buildExplanationState(ex) : null);
      setProvenance(ex?.provenance || ex?.detail?.provenance || null);
      setActivity(acR.status === "fulfilled" ? acR.value?.items || acR.value?.activity || [] : []);
      setExecutions(execR.status === "fulfilled" ? execR.value?.executions || execR.value?.items || [] : []);
      setPostmortem(pmR.status === "fulfilled" ? pmR.value : null);
      setLoadedDetailId(id);
    });
  }, [token]);

  useEffect(() => {
    fetchCounts();
    fetchIncidents();
  }, [fetchCounts, fetchIncidents]);

  useEffect(() => {
    if (selectedId) fetchDetail(selectedId);
  }, [selectedId, fetchDetail]);

  const handleTabChange = (tab) => {
    setStatusTab(tab);
    setSelectedId(null);
    setFilters({ severity: "", service: "", page: 1 });
  };

  async function updateStatus(action) {
    if (!selectedId) return;
    setActionLoading(true);
    try {
      await apiReq(`/incidents/${selectedId}/${action}`, token, { method: "POST" });
      await Promise.all([fetchCounts(), fetchIncidents()]);
      const d = await apiReq(`/incidents/${selectedId}`, token).catch(() => null);
      if (d) setDetail(d?.incident ? { ...d, ...d.incident } : d);
      onRefresh?.();
    } catch {
      /* the list refresh above surfaces any real problem */
    }
    setActionLoading(false);
  }

  async function askCopilot() {
    if (!copilotQ.trim() || !selectedId) return;
    setCopilotLoading(true);
    setCopilotResp(null);
    try {
      const r = await apiReq(`/incidents/copilot/${selectedId}`, token, {
        method: "POST",
        body: JSON.stringify({ question: copilotQ }),
      });
      setCopilotResp(r?.answer || r?.response || JSON.stringify(r));
    } catch (e) {
      setCopilotResp(`Error: ${e.message}`);
    }
    setCopilotLoading(false);
  }

  const emptyLabel = {
    open: "No open incidents",
    acknowledged: "Nothing in progress",
    resolved: "Nothing resolved yet",
  }[statusTab];

  return (
    <div className="workspace">
      {/* ── Queue ────────────────────────────────────────────────────── */}
      <aside className="workspace-queue">
        <div className="queue-tabs" role="tablist">
          {STATUS_TABS.map((tab) => {
            const active = statusTab === tab.key;
            return (
              <button
                key={tab.key}
                type="button"
                role="tab"
                aria-selected={active}
                className={`queue-tab ${tab.tone}${active ? " is-active" : ""}`}
                onClick={() => handleTabChange(tab.key)}
              >
                <span className="queue-tab-count">{counts[tab.key] || 0}</span>
                <span className="queue-tab-label">{tab.label}</span>
              </button>
            );
          })}
        </div>

        <div className="queue-filters">
          <select
            className="form-select form-select-sm"
            value={filters.severity}
            aria-label="Filter by severity"
            onChange={(e) => setFilters((f) => ({ ...f, severity: e.target.value, page: 1 }))}
          >
            <option value="">All severity</option>
            {["critical", "high", "medium", "low", "info", "unknown"].map((s) => (
              <option key={s} value={s}>{s[0].toUpperCase() + s.slice(1)}</option>
            ))}
          </select>
          <select
            className="form-select form-select-sm"
            value={filters.service}
            aria-label="Filter by service"
            onChange={(e) => setFilters((f) => ({ ...f, service: e.target.value, page: 1 }))}
          >
            <option value="">All services</option>
            {serviceOptions.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <button
            type="button"
            className="icon-btn queue-refresh"
            title="Refresh queue"
            onClick={() => fetchIncidents()}
          >
            <RefreshCw size={11} />
          </button>
        </div>

        <div className="queue-list">
          {listLoading ? (
            <div className="queue-skeletons">
              <SkeletonRows count={6} height={48} />
            </div>
          ) : listError ? (
            <ErrorState
              message={listError}
              onRetry={() => {
                setLoadedListKey(null);
                fetchIncidents();
              }}
            />
          ) : incidents.length === 0 ? (
            <EmptyState icon={CheckCircle2} tone="success" title={emptyLabel} message="Your system looks good." />
          ) : (
            incidents.map((inc) => (
              <button
                key={inc.id}
                type="button"
                className={`queue-row${selectedId === inc.id ? " is-active" : ""}`}
                onClick={() => {
                  setSelectedId(inc.id);
                  setActiveDetailTab("overview");
                }}
              >
                <span className="queue-row-top">
                  <SeverityPill severity={inc.severity} />
                  <span className="queue-row-age">{fmtAge(inc.last_event_time)}</span>
                </span>
                <span className="queue-row-title">{inc.title || `Incident #${inc.id}`}</span>
                <span className="queue-row-meta">
                  <span className="queue-row-service">{inc.service || "—"}</span>
                  {inc.seen_before && <span className="tag-recurring">recurring</span>}
                </span>
              </button>
            ))
          )}

          {!listLoading && !listError && total > 30 && (
            <div className="queue-pager">
              <button
                type="button" className="btn btn-ghost btn-xs"
                disabled={filters.page <= 1}
                onClick={() => setFilters((f) => ({ ...f, page: f.page - 1 }))}
              >
                Prev
              </button>
              <span className="queue-pager-label">Page {filters.page}</span>
              <button
                type="button" className="btn btn-ghost btn-xs"
                disabled={incidents.length < 30}
                onClick={() => setFilters((f) => ({ ...f, page: f.page + 1 }))}
              >
                Next
              </button>
            </div>
          )}
        </div>

        <footer className="queue-footer">
          {total} incident{total !== 1 ? "s" : ""}
          {lastRefreshed && ` · ${lastRefreshed.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`}
        </footer>
      </aside>

      {/* ── Detail ───────────────────────────────────────────────────── */}
      {!selectedId ? (
        <div className="workspace-detail is-empty">
          <EmptyState
            icon={Flame}
            title="Select an incident"
            message="Pick anything from the queue to see its analysis, timeline, and lifecycle actions."
          />
        </div>
      ) : (
        <div className="workspace-detail">
          <header className="detail-header">
            {detailLoading ? (
              <SkeletonRows count={2} height={16} />
            ) : detail ? (
              <>
                <div className="detail-header-text">
                  <div className="detail-header-pills">
                    <SeverityPill severity={detail.severity} />
                    <StatusPill status={detail.status} />
                    {detail.seen_before && <span className="pill is-plain tone-info">recurring</span>}
                    {detail.auto_close_at && detail.status !== "resolved" && (
                      <span
                        className="pill is-plain tone-success"
                        title="Every alert in this incident has recovered. It closes automatically unless an alert fires again."
                      >
                        All alerts recovered · closes automatically at {fmtTs(detail.auto_close_at)}
                      </span>
                    )}
                  </div>
                  <h1 className="detail-title">{detail.title || `Incident #${detail.id}`}</h1>
                  <p className="detail-subtitle">
                    {detail.service || "—"} · first {fmtTs(detail.first_event_time)} · last{" "}
                    {fmtTs(detail.last_event_time)}
                  </p>
                </div>

                <div className="detail-header-actions">
                  {detail.status === "open" && (
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => updateStatus("ack")} disabled={actionLoading}>
                      {actionLoading ? <span className="spinner spinner-xs" /> : <ThumbsUp size={12} />}
                      Acknowledge
                    </button>
                  )}
                  {(detail.status === "open" || detail.status === "acknowledged") && (
                    <button type="button" className="btn btn-success btn-sm" onClick={() => updateStatus("resolve")} disabled={actionLoading}>
                      {actionLoading ? <span className="spinner spinner-xs" /> : <CheckCircle2 size={12} />}
                      Resolve
                    </button>
                  )}
                  {detail.status === "resolved" && (
                    <button type="button" className="btn btn-danger btn-sm" onClick={() => updateStatus("reopen")} disabled={actionLoading}>
                      {actionLoading ? <span className="spinner spinner-xs" /> : <Undo2 size={12} />}
                      Reopen
                    </button>
                  )}
                  {isAdmin && (
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => setMuteTarget({ incidentId: detail.id })}>
                      <BellOff size={12} />
                      Mute incident
                    </button>
                  )}
                  <button type="button" className="icon-btn" onClick={() => setSelectedId(null)} title="Close incident">
                    <X size={13} />
                  </button>
                </div>
              </>
            ) : (
              <ErrorState
                message="Could not load this incident."
                onRetry={() => {
                  setLoadedDetailId(null);
                  fetchDetail(selectedId);
                }}
              />
            )}
          </header>

          {muteNotice && (
            <div className="mute-toast" role="status">
              <BellOff size={12} /> {muteNotice}
            </div>
          )}

          <nav className="tabs detail-tabs" role="tablist">
            {DETAIL_TABS.map((tab) => {
              const Icon = tab.icon;
              const active = activeDetailTab === tab.key;
              return (
                <button
                  key={tab.key}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  className={`tab-item${active ? " active" : ""}`}
                  onClick={() => setActiveDetailTab(tab.key)}
                >
                  <Icon size={12} /> {tab.label}
                </button>
              );
            })}
          </nav>

          <div className="detail-body">
            {detailLoading ? (
              <SkeletonRows count={3} height={90} />
            ) : activeDetailTab === "overview" ? (
              <OverviewTab detail={detail} explanation={explanation} provenance={provenance} onAnalyzed={handleAnalyzed} />
            ) : activeDetailTab === "copilot" ? (
              <CopilotTab
                question={copilotQ} setQuestion={setCopilotQ} response={copilotResp}
                loading={copilotLoading} onAsk={askCopilot}
              />
            ) : activeDetailTab === "timeline" ? (
              <TimelineTab
                activity={activity}
                events={Array.isArray(detail?.events) ? detail.events : []}
                onMute={isAdmin ? (eventId) => setMuteTarget({ eventId }) : null}
              />
            ) : activeDetailTab === "actions" ? (
              <ActionsTab executions={executions} />
            ) : (
              <PostMortemTab
                postmortem={postmortem}
                onGenerate={async () => {
                  try {
                    await apiReq(`/incidents/${selectedId}/postmortem/generate`, token, { method: "POST" });
                    const pm = await apiReq(`/incidents/${selectedId}/postmortem`, token);
                    setPostmortem(pm);
                  } catch {
                    /* the tab keeps its empty state and the button stays available */
                  }
                }}
              />
            )}
          </div>
        </div>
      )}

      {muteTarget && (
        <MuteDialog
          token={token}
          eventId={muteTarget.eventId}
          incidentId={muteTarget.incidentId}
          onClose={() => setMuteTarget(null)}
          onCreated={handleMuted}
        />
      )}
    </div>
  );
}

/* ── Overview ────────────────────────────────────────────────────────────*/
function OverviewTab({ detail, explanation, provenance, onAnalyzed }) {
  if (!detail) return null;

  const attrs = [
    { label: "Incident ID", value: detail.id },
    { label: "Service", value: detail.service },
    { label: "Priority score", value: detail.priority_score },
    { label: "Risk score", value: detail.risk_score != null ? `${detail.risk_score}/100` : null },
    { label: "Confidence", value: detail.confidence != null ? `${detail.confidence}%` : null },
    { label: "Event count", value: detail.event_count },
    { label: "Impact count", value: detail.impact_count },
    {
      label: "Correlation",
      value: detail.correlation_pattern
        ? `${detail.correlation_pattern} (score ${detail.correlation_score ?? "—"})`
        : null,
    },
    { label: "First event", value: detail.first_event_time ? new Date(detail.first_event_time).toLocaleString() : null },
    { label: "Last event", value: detail.last_event_time ? new Date(detail.last_event_time).toLocaleString() : null },
    {
      label: "Recurrence",
      value: detail.seen_before ? `Seen ${detail.recurring_count || 1}× before` : "First occurrence",
    },
  ];

  const impacted = Array.isArray(detail.impacted_services) ? detail.impacted_services.filter(Boolean) : [];
  const reasoning = Array.isArray(detail.reasoning) ? detail.reasoning.filter(Boolean) : [];
  const hasWhatChanged = !!(detail.what_changed_type || detail.what_changed_service || detail.what_changed_version);
  const hasRCA = explanation && (explanation.root_cause || explanation.narrative);
  const riskTone = (s) => (s > 70 ? "tone-danger" : s > 40 ? "tone-warning" : "tone-success");

  return (
    <div className="detail-stack">
      {/* Analysis first: RCA when one exists, evidence + Analyse button when not.
          Replaces the old unconditional "Root Cause Summary" block, which
          rendered detail.root_cause_summary regardless of whether anything had
          analysed the incident — presenting a symptom as a diagnosis. */}
      <AnalysisPanel
        incidentId={detail.id}
        detail={detail}
        explanation={explanation}
        provenance={provenance}
        onAnalyzed={onAnalyzed}
      />

      {hasWhatChanged && (
        <DetailBlock title="What changed" tone="tone-info">
          <dl className="attr-grid">
            {[
              { label: "Type", value: detail.what_changed_type },
              { label: "Service", value: detail.what_changed_service },
              { label: "Version", value: detail.what_changed_version },
              { label: "Description", value: detail.what_changed_description },
            ]
              .filter((f) => f.value)
              .map(({ label, value }) => (
                <div key={label} className="attr">
                  <dt className="attr-label">{label}</dt>
                  <dd className="attr-value">{value}</dd>
                </div>
              ))}
          </dl>
          {detail.what_changed_confidence > 0 && (
            <p className="detail-footnote">
              Correlation confidence <strong className="tone-info accent">{detail.what_changed_confidence}%</strong>
            </p>
          )}
        </DetailBlock>
      )}

      {hasRCA && (
        <DetailBlock title="AI root cause analysis" icon={Zap} tone="tone-brand">
          {explanation.root_cause && (
            <div className="detail-section">
              <h4 className="detail-section-label">Root cause</h4>
              <p className="detail-prose is-lead">{explanation.root_cause}</p>
            </div>
          )}
          {explanation.narrative && explanation.narrative !== explanation.root_cause && (
            <div className="detail-section">
              <h4 className="detail-section-label">Narrative</h4>
              <p className="detail-prose">{explanation.narrative}</p>
            </div>
          )}
          {explanation.resolution_steps?.length > 0 && (
            <div className="detail-section">
              <h4 className="detail-section-label">Recommended steps</h4>
              <StepList items={explanation.resolution_steps} tone="tone-brand" />
            </div>
          )}
          {explanation.reasoning?.length > 0 && (
            <div className="detail-section">
              <h4 className="detail-section-label">Reasoning chain</h4>
              <StepList items={explanation.reasoning} tone="tone-brand" />
            </div>
          )}
          <div className="detail-meta-row">
            {explanation.risk_score != null && (
              <span className="detail-meta">
                Risk <strong className={`${riskTone(explanation.risk_score)} accent`}>{explanation.risk_score}/100</strong>
              </span>
            )}
            {explanation.root_cause_type && (
              <span className="detail-meta">
                Type <strong>{explanation.root_cause_type}</strong>
              </span>
            )}
          </div>
        </DetailBlock>
      )}

      <DetailBlock title="Incident details">
        <dl className="attr-grid">
          {attrs.map(({ label, value }) => (
            <div key={label} className="attr">
              <dt className="attr-label">{label}</dt>
              <dd className="attr-value">{value == null || value === "" ? "—" : String(value)}</dd>
            </div>
          ))}
        </dl>

        {impacted.length > 0 && (
          <div className="detail-section is-divided">
            <h4 className="detail-section-label">Impacted services ({impacted.length})</h4>
            <div className="chip-row">
              {impacted.map((svc) => (
                <span key={svc} className="chip tone-danger">{svc}</span>
              ))}
            </div>
          </div>
        )}

        {reasoning.length > 0 && (
          <div className="detail-section is-divided">
            <h4 className="detail-section-label">Correlation reasoning</h4>
            <StepList items={reasoning} />
          </div>
        )}
      </DetailBlock>

      {explanation?.evidence_refs?.length > 0 && (
        <DetailBlock title="Evidence" count={explanation.evidence_refs.length}>
          <ul className="evidence-list">
            {explanation.evidence_refs.map((ref, i) => (
              <li key={i} className="evidence-item">
                <span className="evidence-type">{ref.type}</span>
                <span className="evidence-text">
                  <span className="evidence-label">{ref.label}</span>
                  {ref.detail && <span className="evidence-detail">{ref.detail}</span>}
                </span>
                {ref.timestamp && (
                  <time className="evidence-time">
                    {new Date(ref.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </time>
                )}
              </li>
            ))}
          </ul>
        </DetailBlock>
      )}

      {!detail.root_cause_summary && !hasRCA && !hasWhatChanged && impacted.length === 0 && (
        <EmptyState
          icon={Zap}
          title="Nothing analysed yet"
          message="Run an analysis from the panel above to get a root cause and remediation steps."
        />
      )}
    </div>
  );
}

/* ── Copilot ─────────────────────────────────────────────────────────────*/
const SUGGESTED_QUESTIONS = [
  "What is the probable root cause?",
  "Which services are affected?",
  "What are the remediation steps?",
  "Have we seen this before?",
];

function CopilotTab({ question, setQuestion, response, loading, onAsk }) {
  return (
    <div className="detail-stack">
      <div className="callout tone-brand">
        <Zap size={12} className="callout-icon" />
        <div>
          <p className="callout-title">AI Copilot</p>
          <p className="callout-text">
            Ask anything about this incident — root cause, impact, remediation, or similar past incidents.
          </p>
        </div>
      </div>

      <div className="copilot-ask">
        <input
          type="text"
          className="form-input"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && !loading && onAsk()}
          placeholder="What is the root cause? How do I fix it?"
          aria-label="Ask the copilot"
        />
        <button type="button" className="btn btn-primary" onClick={onAsk} disabled={loading || !question.trim()}>
          {loading ? <span className="spinner spinner-xs" /> : <MessageSquare size={12} />}
          Ask
        </button>
      </div>

      {response ? (
        <div className="copilot-answer">{response}</div>
      ) : (
        !loading && (
          <div className="detail-section">
            <h4 className="detail-section-label">Suggested questions</h4>
            <div className="suggestion-list">
              {SUGGESTED_QUESTIONS.map((q) => (
                <button key={q} type="button" className="suggestion" onClick={() => setQuestion(q)}>
                  <ChevronRight size={11} /> {q}
                </button>
              ))}
            </div>
          </div>
        )
      )}
    </div>
  );
}

/* ── Timeline ────────────────────────────────────────────────────────────*/
/* Alerts are listed newest first, capped so a storm cannot swamp the tab. */
const MAX_LISTED_ALERTS = 50;

// Links in alert labels come from the customer's tools; only http(s) is rendered.
function httpLink(url) {
  return /^https?:\/\//i.test(url || "") ? url : null;
}

function TimelineTab({ activity, events, onMute }) {
  // Incident detail wraps each alert as { event, story_label, … }.
  const alerts = events
    .map((item) => item?.event || item)
    .filter((ev) => ev?.id)
    .slice(-MAX_LISTED_ALERTS)
    .reverse();
  return (
    <div className="timeline-tab">
      {alerts.length > 0 && (
        <DetailBlock title="Alerts in this incident" icon={Flame} count={events.length}>
          <ul className="incident-alerts">
            {alerts.map((ev) => {
              const labels = ev.labels || {};
              const where = labels.instance || labels.host || labels.pod || ev.resource || ev.service;
              // Runbook link written by the alert rule's author (Prometheus/Grafana
              // runbook_url annotation). Only http(s) links are rendered.
              const runbook = httpLink(labels["annotation.runbook_url"]);
              const zabbixLink = httpLink(labels["zabbix.url"]);
              const grafanaRule = httpLink(labels["grafana.rule_url"]);
              const grafanaView = httpLink(labels["grafana.panel_url"]) || httpLink(labels["grafana.dashboard_url"]);
              const value = labels["grafana.value"] || labels["zabbix.item_value"] || labels["annotation.value"];
              const gap = labels["grafana.monitoring_gap"];
              return (
                <li key={ev.id} className="incident-alert">
                  <SeverityPill severity={ev.severity} />
                  <div className="incident-alert-text">
                    <span className="incident-alert-title">
                      {ev.title || ev.message || ev.id}
                      {labels.neuroops_test === "true" && <span className="pill is-plain tone-info">test</span>}
                      {gap && (
                        <span
                          className="pill is-plain tone-neutral"
                          title="The alert rule could not evaluate, so NeuroOps cannot tell whether the service is healthy."
                        >
                          monitoring gap · {gap === "no_data" ? "no data" : "query error"}
                        </span>
                      )}
                    </span>
                    <span className="incident-alert-meta">
                      {[labels.alertname, where, value && `value ${value}`].filter(Boolean).join(" · ")}
                      {ev.timestamp && ` · ${new Date(ev.timestamp).toLocaleString()}`}
                    </span>
                    {labels["service.derived_from"] && (
                      <span
                        className="incident-alert-meta"
                        title="Set a service label on your Prometheus alert rules to control this."
                      >
                        Service “{ev.service}” taken from {labels["service.derived_from"]} — the service label named the
                        exporter ({labels["service.original"]})
                      </span>
                    )}
                  </div>
                  {grafanaRule && (
                    <a className="btn btn-ghost btn-xs" href={grafanaRule} target="_blank" rel="noopener noreferrer">
                      Open rule in Grafana ↗
                    </a>
                  )}
                  {grafanaView && (
                    <a className="btn btn-ghost btn-xs" href={grafanaView} target="_blank" rel="noopener noreferrer">
                      Dashboard ↗
                    </a>
                  )}
                  {zabbixLink && (
                    <a className="btn btn-ghost btn-xs" href={zabbixLink} target="_blank" rel="noopener noreferrer">
                      Open in Zabbix ↗
                    </a>
                  )}
                  {runbook && (
                    <a className="btn btn-ghost btn-xs" href={runbook} target="_blank" rel="noopener noreferrer">
                      <FileText size={11} /> Runbook ↗
                    </a>
                  )}
                  {onMute && (
                    <button type="button" className="btn btn-ghost btn-xs" onClick={() => onMute(ev.id)}>
                      <BellOff size={11} /> Mute
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        </DetailBlock>
      )}

      {!activity.length ? (
        <EmptyState
          icon={Activity}
          title="No activity recorded"
          message="Events appear here as the incident progresses."
        />
      ) : (
        <ol className="timeline">
          {activity.map((ev, i) => (
            <li key={ev.id || i} className="timeline-item">
              <span className="timeline-marker" aria-hidden="true" />
              <div className="timeline-content">
                <p className="timeline-message">
                  {ev.message || ev.description || ev.action || JSON.stringify(ev)}
                </p>
                <p className="timeline-meta">
                  {ev.actor || ev.user || "system"} ·{" "}
                  {ev.timestamp || ev.created_at
                    ? new Date(ev.timestamp || ev.created_at).toLocaleString()
                    : "—"}
                </p>
              </div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

/* ── Actions ─────────────────────────────────────────────────────────────*/
const EXEC_TONE = {
  success: "tone-success",
  completed: "tone-success",
  failed: "tone-danger",
  running: "tone-warning",
};

function ActionsTab({ executions }) {
  if (!executions.length) {
    return (
      <EmptyState
        icon={Activity}
        title="No actions executed"
        message="Automated and manual action executions appear here."
      />
    );
  }
  return (
    <div className="detail-stack is-tight">
      {executions.map((ex, i) => (
        <article key={ex.id || i} className="exec-card">
          <header className="exec-head">
            <h3 className="exec-title">{ex.action_name || ex.type || `Action #${ex.id}`}</h3>
            <span className={`pill is-plain ${EXEC_TONE[ex.status] || "tone-neutral"}`}>{ex.status}</span>
          </header>
          {ex.output && <pre className="exec-output">{ex.output}</pre>}
          <footer className="exec-meta">
            {ex.executed_by || ex.actor || "agent"} ·{" "}
            {ex.executed_at ? new Date(ex.executed_at).toLocaleString() : "—"}
          </footer>
        </article>
      ))}
    </div>
  );
}

/* ── Post-mortem ─────────────────────────────────────────────────────────*/
function PostMortemTab({ postmortem, onGenerate }) {
  const [generating, setGenerating] = useState(false);

  async function generate() {
    setGenerating(true);
    await onGenerate?.();
    setGenerating(false);
  }

  if (!postmortem || (!postmortem.summary && !postmortem.content)) {
    return (
      <EmptyState
        icon={FileText}
        title="No post-mortem yet"
        message="Generate an AI-written post-mortem for this incident."
        action={generating ? "Generating…" : "Generate post-mortem"}
        onAction={generate}
      />
    );
  }

  return (
    <div className="detail-stack is-tight">
      <div className="detail-toolbar">
        <h3 className="detail-toolbar-title">
          <FileText size={13} /> Post-mortem report
        </h3>
        <button type="button" className="btn btn-outline btn-sm" onClick={generate} disabled={generating}>
          {generating ? <span className="spinner spinner-xs" /> : <RefreshCw size={11} />}
          Regenerate
        </button>
      </div>
      {[
        { label: "Summary", value: postmortem.summary },
        { label: "Timeline", value: postmortem.timeline },
        { label: "Root cause", value: postmortem.root_cause },
        { label: "Impact", value: postmortem.impact },
        { label: "Action items", value: postmortem.action_items },
        { label: "Lessons learned", value: postmortem.lessons_learned },
      ]
        .filter((f) => f.value)
        .map(({ label, value }) => (
          <DetailBlock key={label} title={label}>
            <p className="detail-prose is-preserve">{value}</p>
          </DetailBlock>
        ))}
    </div>
  );
}
