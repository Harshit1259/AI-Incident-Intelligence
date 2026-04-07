import React, { useEffect, useMemo, useState } from "react";

const API_BASE = "/api/v1";

async function request(path, options = {}) {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    const message =
      (typeof payload === "object" && (payload.error || payload.message)) ||
      (typeof payload === "string" && payload) ||
      `Request failed (${response.status})`;
    throw new Error(message);
  }

  return payload;
}

function extractIncidents(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.incidents)) return payload.incidents;
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.incidents)) return payload.data.incidents;
  if (payload.data && Array.isArray(payload.data.items)) return payload.data.items;
  return [];
}

function extractDetail(payload) {
  if (!payload || typeof payload !== "object") return null;
  if (payload.incident || payload.summary || payload.events) return payload;
  if (payload.data && (payload.data.incident || payload.data.summary || payload.data.events)) {
    return payload.data;
  }
  return payload;
}

function extractActivity(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.activities)) return payload.activities;
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.activities)) return payload.data.activities;
  return [];
}

function extractActionAudit(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.audit)) return payload.audit;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.items)) return payload.data.items;
  return [];
}

function severityClass(value) {
  return `severity-${String(value || "unknown").toLowerCase()}`;
}

function statusClass(value) {
  return `status-${String(value || "unknown").toLowerCase()}`;
}

function riskClass(value) {
  return `risk-${String(value || "low").toLowerCase()}`;
}

function formatTimestamp(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function titleCase(value) {
  if (!value) return "-";
  return String(value)
    .replace(/[_-]/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function deriveIntelligence(detail, incidents, selectedIncidentId) {
  const incident = detail?.incident || {};
  const summary = detail?.summary || {};
  const insight = detail?.insight || {};
  const impact = detail?.impact || {};
  const whatChanged = detail?.what_changed || {};
  const primaryAction = detail?.primary_action || null;
  const actions = detail?.actions || [];
  const evidence = detail?.evidence || [];
  const reasoning = incident.reasoning || [];
  const correlationScore = Number(summary.correlation_score || 0);
  const eventCount = Number(summary.event_count || 0);
  const impactCount = Number(impact.impact_count || summary.impact_count || 0);
  const recurringCount = Number(summary.recurring_count || summary.recurringCount || 0);
  const confidence = Number(summary.confidence || incident.confidence || 0);
  const rootCauseType = summary.root_cause_type || incident.root_cause_type || "generic";
  const service = incident.service || summary.service || "";
  const severity = incident.severity || summary.severity || "medium";

  const causeCandidates = [];
  const rootScore = clamp(
    42 +
      Math.round(correlationScore * 0.25) +
      Math.round(eventCount * 4) +
      Math.round(impactCount * 3) +
      (whatChanged.type ? 10 : 0),
    35,
    95
  );

  causeCandidates.push({
    name: titleCase(rootCauseType),
    confidence: rootScore,
    rationale:
      summary.root_cause_summary ||
      "Signals grouped around the strongest incident pattern.",
  });

  if (whatChanged.type) {
    causeCandidates.push({
      name: `${titleCase(whatChanged.type)} Change`,
      confidence: clamp(rootScore - 2, 28, 92),
      rationale:
        `A recent ${whatChanged.type} is linked to the same service or incident window.`,
    });
  }

  if (impact.downstream?.length || impact.affected_services?.length) {
    causeCandidates.push({
      name: "Dependency Degradation",
      confidence: clamp(28 + impactCount * 8 + Math.round(correlationScore * 0.12), 25, 88),
      rationale:
        "Impact spread suggests a dependency or downstream propagation path.",
    });
  }

  if (recurringCount > 0) {
    causeCandidates.push({
      name: "Recurring Failure Pattern",
      confidence: clamp(24 + recurringCount * 8 + Math.round(confidence * 0.15), 22, 84),
      rationale:
        "A similar incident was seen before, which increases the probability of a repeated failure mode.",
    });
  }

  const dedupedCandidates = [];
  const seenNames = new Set();
  causeCandidates
    .sort((left, right) => right.confidence - left.confidence)
    .forEach((candidate) => {
      if (!seenNames.has(candidate.name)) {
        seenNames.add(candidate.name);
        dedupedCandidates.push(candidate);
      }
    });

  const confidenceBreakdown = [
    {
      label: "Correlation Strength",
      value: clamp(correlationScore, 0, 100),
      note: "Pattern, timing, and service matching",
    },
    {
      label: "Signal Volume",
      value: clamp(18 + eventCount * 10, 0, 100),
      note: "More grouped signals increase confidence",
    },
    {
      label: "Impact Spread",
      value: clamp(15 + impactCount * 12, 0, 100),
      note: "Blast radius across services",
    },
    {
      label: "Change Link",
      value: whatChanged.type ? 78 : 24,
      note: whatChanged.type
        ? `${titleCase(whatChanged.type)} linked`
        : "No linked change found",
    },
    {
      label: "Recurrence",
      value: recurringCount > 0 ? clamp(40 + recurringCount * 10, 0, 100) : 18,
      note: recurringCount > 0 ? "Seen before" : "No recurrence context",
    },
  ];

  const derivedNextBestAction =
    primaryAction?.label ||
    actions[0]?.label ||
    insight.suggested_action ||
    detail?.recommended_next_step ||
    "Validate the suspected failure path";

  const derivedNextBestActionDescription =
    primaryAction?.description ||
    actions[0]?.description ||
    "Start with the highest-confidence action to reduce time-to-restore.";

  const fallbackAction =
    actions[1]?.label ||
    (whatChanged.type ? "Validate or rollback recent change" : "Inspect dependency health and recent logs");

  const fallbackActionDescription =
    actions[1]?.description ||
    (whatChanged.type
      ? "If the primary action does not improve the situation, inspect the recent change path."
      : "If the primary action fails, verify dependencies and event evidence.");

  const relatedIncidents = incidents
    .filter((item) => item.id !== selectedIncidentId)
    .filter((item) => {
      const sameService = (item.service || "") === service;
      const samePattern =
        (item.root_cause_type || item.rootCauseType || "") === rootCauseType;
      const openOrAcknowledged =
        item.status === "open" || item.status === "acknowledged";
      return sameService || (samePattern && openOrAcknowledged);
    })
    .slice(0, 4)
    .map((item) => ({
      id: item.id,
      title: item.title,
      service: item.service,
      severity: item.severity,
      status: item.status,
      confidence: item.confidence,
      risk: item.risk_score,
    }));

  const correlationSignature = [
    service || "unknown-service",
    rootCauseType || "generic",
    severity || "medium",
    eventCount,
    impactCount,
  ].join(" · ");

  const evidenceHighlights = [
    ...reasoning.slice(0, 3),
    ...evidence.slice(0, 3),
  ].filter(Boolean);

  return {
    causeCandidates: dedupedCandidates.slice(0, 4),
    confidenceBreakdown,
    nextBestAction: {
      label: derivedNextBestAction,
      description: derivedNextBestActionDescription,
    },
    fallbackAction: {
      label: fallbackAction,
      description: fallbackActionDescription,
    },
    relatedIncidents,
    correlationSignature,
    evidenceHighlights,
  };
}

function deriveLiveOpsStats(incidents, selectedDetail, lastRefreshAt) {
  const open = incidents.filter((item) => item.status === "open").length;
  const acknowledged = incidents.filter((item) => item.status === "acknowledged").length;
  const critical = incidents.filter((item) => item.severity === "critical").length;

  const latestIncidentTime = incidents
    .map((item) => item.last_event_time || item.lastEventTime || item.first_event_time)
    .filter(Boolean)
    .sort()
    .reverse()[0];

  const selectedSummary = selectedDetail?.summary || {};
  const selectedIncident = selectedDetail?.incident || {};
  const freshnessSource =
    selectedSummary.latest_event_time ||
    selectedIncident.last_event_time ||
    selectedIncident.lastEventTime ||
    latestIncidentTime;

  let freshnessLabel = "No signal";
  if (freshnessSource) {
    const now = Date.now();
    const then = new Date(freshnessSource).getTime();
    if (!Number.isNaN(then)) {
      const diffMinutes = Math.max(0, Math.round((now - then) / 60000));
      if (diffMinutes <= 1) freshnessLabel = "Live";
      else if (diffMinutes <= 5) freshnessLabel = `${diffMinutes} min ago`;
      else if (diffMinutes <= 60) freshnessLabel = `${diffMinutes} min ago`;
      else freshnessLabel = `${Math.round(diffMinutes / 60)} hr ago`;
    }
  }

  return {
    open,
    acknowledged,
    critical,
    total: incidents.length,
    latestIncidentTime,
    freshnessLabel,
    lastRefreshAt,
  };
}

export default function IncidentCommandCenter() {
  const [incidents, setIncidents] = useState([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState("");
  const [detail, setDetail] = useState(null);
  const [explanation, setExplanation] = useState("");
  const [activity, setActivity] = useState([]);
  const [actionAudit, setActionAudit] = useState([]);
  const [copilotQuestion, setCopilotQuestion] = useState("Why is this happening?");
  const [copilotAnswer, setCopilotAnswer] = useState(null);
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState("");
  const [liveRefresh, setLiveRefresh] = useState(true);
  const [activeTab, setActiveTab] = useState("overview");
  const [workspaceMode, setWorkspaceMode] = useState("live");
  const [lastRefreshAt, setLastRefreshAt] = useState("");
  const [ingestStatus, setIngestStatus] = useState("");
  const [sendingIngest, setSendingIngest] = useState(false);

  const [filters, setFilters] = useState({
    search: "",
    status: "",
    severity: "",
    service: "",
    from: "",
    to: "",
  });

  const selectedIncident = useMemo(
    () => incidents.find((item) => item.id === selectedIncidentId) || null,
    [incidents, selectedIncidentId]
  );

  const services = useMemo(() => {
    const values = Array.from(new Set(incidents.map((item) => item.service).filter(Boolean)));
    return values.sort();
  }, [incidents]);

  const filteredIncidents = useMemo(() => {
    return incidents.filter((incident) => {
      const title = `${incident.title || ""} ${incident.service || ""}`.toLowerCase();
      const searchMatch =
        !filters.search || title.includes(filters.search.trim().toLowerCase());

      const statusMatch = !filters.status || incident.status === filters.status;
      const severityMatch = !filters.severity || incident.severity === filters.severity;
      const serviceMatch = !filters.service || incident.service === filters.service;

      const incidentTime = incident.last_event_time || incident.lastEventTime || incident.first_event_time;
      const time = incidentTime ? new Date(incidentTime).getTime() : null;
      const fromOk = !filters.from || (time && time >= new Date(filters.from).getTime());
      const toOk = !filters.to || (time && time <= new Date(filters.to).getTime() + 86400000);

      return searchMatch && statusMatch && severityMatch && serviceMatch && fromOk && toOk;
    });
  }, [incidents, filters]);

  const metrics = useMemo(() => {
    return {
      open: incidents.filter((item) => item.status === "open").length,
      acknowledged: incidents.filter((item) => item.status === "acknowledged").length,
      critical: incidents.filter((item) => item.severity === "critical").length,
      changeLinked: incidents.filter((item) => item.what_changed_type || item.whatChangedType).length,
    };
  }, [incidents]);

  async function loadIncidents(selectFirst = false) {
    setLoading(true);
    setError("");

    try {
      const payload = await request(
        `/incidents?page=1&page_size=20&sort_by=last_event_time&sort_order=desc`
      );
      const items = extractIncidents(payload);
      setIncidents(items);
      setLastRefreshAt(new Date().toISOString());

      const nextSelectedId =
        selectedIncidentId && items.some((item) => item.id === selectedIncidentId)
          ? selectedIncidentId
          : items[0]?.id || "";

      if (selectFirst || !selectedIncidentId || !items.some((item) => item.id === selectedIncidentId)) {
        setSelectedIncidentId(nextSelectedId);
      }
    } catch (err) {
      setError(err.message || "Failed to load incidents");
    } finally {
      setLoading(false);
    }
  }

  async function loadIncidentBundle(incidentId) {
    if (!incidentId) {
      setDetail(null);
      setExplanation("");
      setActivity([]);
      setActionAudit([]);
      return;
    }

    setDetailLoading(true);

    try {
      const [detailPayload, explainPayload, activityPayload, auditPayload] = await Promise.allSettled([
        request(`/incidents/${incidentId}`),
        request(`/incidents/explain/${incidentId}`),
        request(`/incidents/activity/${incidentId}`),
        request(`/actions/audit?incident_id=${incidentId}`),
      ]);

      if (detailPayload.status === "fulfilled") {
        setDetail(extractDetail(detailPayload.value));
      } else {
        setDetail(null);
      }

      if (explainPayload.status === "fulfilled") {
        const explainValue = explainPayload.value;
        setExplanation(
          explainValue?.explanation ||
            explainValue?.answer ||
            explainValue?.data?.explanation ||
            ""
        );
      } else {
        setExplanation("Failed to fetch");
      }

      if (activityPayload.status === "fulfilled") {
        setActivity(extractActivity(activityPayload.value));
      } else {
        setActivity([{ id: "activity-error", title: "Failed to fetch" }]);
      }

      if (auditPayload.status === "fulfilled") {
        setActionAudit(extractActionAudit(auditPayload.value));
      } else {
        setActionAudit([]);
      }
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    loadIncidents(true);
  }, []);

  useEffect(() => {
    loadIncidentBundle(selectedIncidentId);
  }, [selectedIncidentId]);

  useEffect(() => {
    if (!liveRefresh) return;

    const interval = setInterval(() => {
      loadIncidents(false);
      if (selectedIncidentId) {
        loadIncidentBundle(selectedIncidentId);
      }
    }, 10000);

    return () => clearInterval(interval);
  }, [liveRefresh, selectedIncidentId]);

  async function runScenario(name) {
    try {
      await request("/demo/scenario", {
        method: "POST",
        body: JSON.stringify({ scenario: name }),
      });
      await loadIncidents(true);
    } catch (err) {
      setError(err.message || "Failed to run scenario");
    }
  }

  async function resetSystem() {
    try {
      await request("/dev/reset", { method: "POST" });
      await loadIncidents(true);
      setDetail(null);
      setExplanation("");
      setActivity([]);
      setActionAudit([]);
      setCopilotAnswer(null);
      setIngestStatus("System reset complete");
    } catch (err) {
      setError(err.message || "Failed to reset system");
    }
  }

  async function updateStatus(action) {
    if (!selectedIncidentId) return;

    try {
      await request(`/incidents/${selectedIncidentId}/${action}`, {
        method: "POST",
        body: JSON.stringify({
          changed_by: "operator",
          note: `${action} requested from UI`,
        }),
      });
      await loadIncidents(false);
      await loadIncidentBundle(selectedIncidentId);
    } catch (err) {
      setError(err.message || `Failed to ${action} incident`);
    }
  }

  async function askCopilot(question) {
    if (!selectedIncidentId || !question.trim()) return;

    try {
      const payload = await request(`/incidents/copilot/${selectedIncidentId}`, {
        method: "POST",
        body: JSON.stringify({ question }),
      });
      setCopilotAnswer(payload);
    } catch (err) {
      setCopilotAnswer({
        answer: err.message || "Failed to fetch",
        suggested_followups: [],
      });
    }
  }

  async function executeAction(action) {
    if (!selectedIncidentId) return;

    try {
      const approved = action.requires_approval
        ? window.confirm(`Approve action "${action.label}"?`)
        : false;

      await request("/actions/execute", {
        method: "POST",
        body: JSON.stringify({
          incident_id: selectedIncidentId,
          action_id: action.id,
          approved,
        }),
      });

      await loadIncidentBundle(selectedIncidentId);
    } catch (err) {
      setError(err.message || "Failed to execute action");
    }
  }

  async function sendGenericTest() {
    setSendingIngest(true);
    setIngestStatus("");

    try {
      await request("/ingest/webhook", {
        method: "POST",
        body: JSON.stringify({
          tenant_id: "default",
          source: "generic",
          external_id: `frontend-generic-${Date.now()}`,
          service: "payments-api",
          resource: "pod/payments-api-1",
          environment: "prod",
          severity: "critical",
          signal_type: "alert",
          title: "Payments API database connectivity failure",
          message: "payments-api cannot connect to the primary database",
          labels: {
            cluster: "prod-cluster-1",
            namespace: "payments",
          },
          timestamp: new Date().toISOString(),
        }),
      });

      setIngestStatus("Generic webhook test sent");
      await loadIncidents(true);
    } catch (err) {
      setError(err.message || "Failed to send generic ingest");
    } finally {
      setSendingIngest(false);
    }
  }

  async function sendPrometheusTest() {
    setSendingIngest(true);
    setIngestStatus("");

    try {
      await request("/ingest/prometheus", {
        method: "POST",
        body: JSON.stringify({
          alerts: [
            {
              status: "firing",
              labels: {
                service: "checkout-api",
                severity: "critical",
                instance: "checkout-pod-3",
                environment: "prod",
              },
              annotations: {
                summary: "Checkout timeout spike",
                description: "checkout-api latency degraded and requests are timing out",
              },
              startsAt: new Date().toISOString(),
              fingerprint: `frontend-prom-${Date.now()}`,
            },
          ],
        }),
      });

      setIngestStatus("Prometheus test alert sent");
      await loadIncidents(true);
    } catch (err) {
      setError(err.message || "Failed to send Prometheus ingest");
    } finally {
      setSendingIngest(false);
    }
  }

  const incident = detail?.incident || selectedIncident || {};
  const summary = detail?.summary || {};
  const insight = detail?.insight || {};
  const impact = detail?.impact || {};
  const graph = detail?.graph || { nodes: [], edges: [] };
  const timeline = detail?.events || [];
  const actions = detail?.actions || [];
  const primaryAction = detail?.primary_action || null;
  const statusAudit = detail?.status_audit || [];
  const evidence = detail?.evidence || [];

  const intelligence = useMemo(
    () => deriveIntelligence(detail, incidents, selectedIncidentId),
    [detail, incidents, selectedIncidentId]
  );

  const liveOpsStats = useMemo(
    () => deriveLiveOpsStats(incidents, detail, lastRefreshAt),
    [incidents, detail, lastRefreshAt]
  );

  return (
    <div className="lux-shell">
      {error ? <div className="lux-global-error">{error}</div> : null}

      <div className="lux-topbar">
        <div className="lux-brand">
          <div className="lux-brand-mark">AI</div>
          <div>
            <div className="lux-eyebrow">AI INCIDENT INTELLIGENCE</div>
            <div className="lux-brand-title">Incident Command Center</div>
          </div>
        </div>

        <div className="lux-top-actions">
          <div className="lux-mode-switch">
            <button
              className={`lux-mode-btn ${workspaceMode === "live" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("live")}
            >
              Live Mode
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "demo" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("demo")}
            >
              Demo Mode
            </button>
          </div>
          <button className="lux-secondary-btn" onClick={() => setLiveRefresh((value) => !value)}>
            Live refresh: {liveRefresh ? "On" : "Off"}
          </button>
        </div>
      </div>

      <section className="lux-hero">
        <div className="lux-hero-copy">
          <div className="lux-eyebrow">{workspaceMode === "live" ? "LIVE OPERATIONS" : "COMMAND VIEW"}</div>
          <h1>{workspaceMode === "live" ? "From demo signals to live incident operations." : "Systems should explain themselves."}</h1>
          <p>
            {workspaceMode === "live"
              ? "Connect alert sources, send real payloads, and operate incidents from one console with cause, confidence, actioning, timelines, and audit."
              : "Replace dashboard hunting with one-screen incident understanding: cause, confidence, change context, impact, execution, timeline, audits, and next action."}
          </p>
        </div>

        <div className="lux-kpi-strip">
          <MetricCard label="Open" value={metrics.open} />
          <MetricCard label="Acknowledged" value={metrics.acknowledged} />
          <MetricCard label="Critical" value={metrics.critical} />
          <MetricCard label="Change-linked" value={metrics.changeLinked} />
        </div>
      </section>

      <section className="lux-filter-bar">
        <div className="lux-filter-title">
          <div className="lux-eyebrow">INCIDENT QUERY LAYER</div>
          <h2>Filter incidents</h2>
        </div>

        <div className="lux-filter-grid">
          <label>
            <span>Search</span>
            <input
              value={filters.search}
              onChange={(e) =>
                setFilters((current) => ({ ...current, search: e.target.value }))
              }
              placeholder="Title or service"
            />
          </label>

          <label>
            <span>Status</span>
            <select
              value={filters.status}
              onChange={(e) =>
                setFilters((current) => ({ ...current, status: e.target.value }))
              }
            >
              <option value="">All statuses</option>
              <option value="open">open</option>
              <option value="acknowledged">acknowledged</option>
              <option value="resolved">resolved</option>
            </select>
          </label>

          <label>
            <span>Severity</span>
            <select
              value={filters.severity}
              onChange={(e) =>
                setFilters((current) => ({ ...current, severity: e.target.value }))
              }
            >
              <option value="">All severities</option>
              <option value="critical">critical</option>
              <option value="high">high</option>
              <option value="medium">medium</option>
              <option value="low">low</option>
            </select>
          </label>

          <label>
            <span>Service</span>
            <select
              value={filters.service}
              onChange={(e) =>
                setFilters((current) => ({ ...current, service: e.target.value }))
              }
            >
              <option value="">All services</option>
              {services.map((service) => (
                <option key={service} value={service}>
                  {service}
                </option>
              ))}
            </select>
          </label>

          <label>
            <span>From</span>
            <input
              type="date"
              value={filters.from}
              onChange={(e) =>
                setFilters((current) => ({ ...current, from: e.target.value }))
              }
            />
          </label>

          <label>
            <span>To</span>
            <input
              type="date"
              value={filters.to}
              onChange={(e) =>
                setFilters((current) => ({ ...current, to: e.target.value }))
              }
            />
          </label>
        </div>

        <div className="lux-filter-actions">
          <button
            className="lux-secondary-btn"
            onClick={() =>
              setFilters({
                search: "",
                status: "",
                severity: "",
                service: "",
                from: "",
                to: "",
              })
            }
          >
            Clear filters
          </button>
        </div>
      </section>

      <div className="lux-layout">
        <aside className="lux-left-rail">
          {workspaceMode === "live" ? (
            <>
              <section className="lux-card">
                <div className="lux-section-head">
                  <div>
                    <div className="lux-eyebrow">SOURCE ONBOARDING</div>
                    <h3>Connect alert sources</h3>
                  </div>
                </div>

                <div className="lux-live-source-stack">
                  <div className="lux-live-source-card">
                    <div className="lux-source-card-top">
                      <strong>Generic Webhook</strong>
                      <span className="lux-mini-chip">POST</span>
                    </div>
                    <code className="lux-endpoint-code">/api/v1/ingest/webhook</code>
                    <p>Use this for custom systems, app hooks, or internal alert pipelines.</p>
                    <button
                      className="lux-primary-btn small"
                      onClick={sendGenericTest}
                      disabled={sendingIngest}
                    >
                      {sendingIngest ? "Sending..." : "Send generic test"}
                    </button>
                  </div>

                  <div className="lux-live-source-card">
                    <div className="lux-source-card-top">
                      <strong>Prometheus Alertmanager</strong>
                      <span className="lux-mini-chip">POST</span>
                    </div>
                    <code className="lux-endpoint-code">/api/v1/ingest/prometheus</code>
                    <p>Use this for Alertmanager payloads and Prometheus-backed alerting flows.</p>
                    <button
                      className="lux-primary-btn small"
                      onClick={sendPrometheusTest}
                      disabled={sendingIngest}
                    >
                      {sendingIngest ? "Sending..." : "Send Prometheus test"}
                    </button>
                  </div>
                </div>

                {ingestStatus ? <div className="lux-ingest-status">{ingestStatus}</div> : null}
              </section>

              <section className="lux-card lux-sticky-list">
                <div className="lux-section-head">
                  <div>
                    <div className="lux-eyebrow">LIVE HEALTH</div>
                    <h3>Source health snapshot</h3>
                  </div>
                </div>

                <div className="lux-source-health-grid">
                  <LiveHealthCard label="Total incidents" value={liveOpsStats.total} note="Current dataset" />
                  <LiveHealthCard label="Open" value={liveOpsStats.open} note="Needs response" />
                  <LiveHealthCard label="Critical" value={liveOpsStats.critical} note="Highest urgency" />
                  <LiveHealthCard label="Freshness" value={liveOpsStats.freshnessLabel} note="Latest signal age" />
                </div>

                <div className="lux-health-footer">
                  <div><strong>Last refresh:</strong> {formatTimestamp(liveOpsStats.lastRefreshAt)}</div>
                  <div><strong>Latest incident:</strong> {formatTimestamp(liveOpsStats.latestIncidentTime)}</div>
                </div>
              </section>
            </>
          ) : (
            <section className="lux-card">
              <div className="lux-section-head">
                <div>
                  <div className="lux-eyebrow">DEMO MODE</div>
                  <h3>Scenario Launcher</h3>
                </div>
              </div>

              <div className="lux-scenario-stack">
                <ScenarioCard
                  title="Checkout Timeout Cascade"
                  description="Latency → timeout → timeout spike on checkout-api"
                  onRun={() => runScenario("checkout_timeout_cascade")}
                  onReset={resetSystem}
                />
                <ScenarioCard
                  title="Payments Database Failure"
                  description="Database outage impacting payments-api"
                  onRun={() => runScenario("payments_database_failure")}
                  onReset={resetSystem}
                />
                <ScenarioCard
                  title="Inventory Service Degradation"
                  description="Latency → timeout → failure spike on inventory-api"
                  onRun={() => runScenario("inventory_service_degradation")}
                  onReset={resetSystem}
                />
              </div>
            </section>
          )}

          <section className="lux-card lux-sticky-list">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">INCIDENTS</div>
                <h3>Current incidents</h3>
              </div>
              <span className="lux-small-note">{filteredIncidents.length} visible</span>
            </div>

            {loading ? (
              <div className="lux-muted">Loading incidents...</div>
            ) : filteredIncidents.length === 0 ? (
              <div className="lux-muted">No incidents found.</div>
            ) : (
              <div className="lux-incident-list">
                {filteredIncidents.map((item) => {
                  const selected = item.id === selectedIncidentId;
                  return (
                    <button
                      key={item.id}
                      className={`lux-incident-item ${selected ? "selected" : ""}`}
                      onClick={() => setSelectedIncidentId(item.id)}
                    >
                      <div className="lux-incident-top">
                        <span className={`lux-pill ${severityClass(item.severity)}`}>
                          {item.severity}
                        </span>
                        <span className={`lux-pill ${statusClass(item.status)}`}>
                          {item.status}
                        </span>
                      </div>

                      <div className="lux-incident-title">{item.title}</div>
                      <div className="lux-incident-service">{item.service}</div>
                      <div className="lux-incident-cause">
                        {item.root_cause_summary || item.rootCauseSummary}
                      </div>

                      <div className="lux-incident-meta">
                        <span className="lux-mini-chip">Confidence {item.confidence}</span>
                        <span className="lux-mini-chip">Risk {item.risk_score}</span>
                        <span className="lux-mini-chip">Impact {item.impact_count}</span>
                      </div>

                      {(item.recurring_count || item.recurringCount) > 0 ? (
                        <div className="lux-recurring-line">
                          Seen before · {item.recurring_count || item.recurringCount}
                        </div>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            )}
          </section>
        </aside>

        <main className="lux-main">
          <section className="lux-card lux-detail-hero">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">INCIDENT DETAIL</div>
                <h2>{incident.title || "Select an incident"}</h2>
                <div className="lux-detail-subtitle">
                  {incident.service || "-"} · {incident.status || "-"} · {incident.severity || "-"}
                </div>
              </div>

              <div className="lux-detail-actions">
                {incident.severity ? (
                  <span className={`lux-pill ${severityClass(incident.severity)}`}>
                    {incident.severity}
                  </span>
                ) : null}

                {(summary.recurring_count || summary.recurringCount) > 0 ? (
                  <span className="lux-pill recurring-pill">
                    Seen before · {summary.recurring_count || summary.recurringCount}
                  </span>
                ) : null}

                <button className="lux-secondary-btn" onClick={() => updateStatus("ack")}>
                  Acknowledge
                </button>
                <button className="lux-primary-btn" onClick={() => updateStatus("resolve")}>
                  Resolve
                </button>
              </div>
            </div>

            <div className="lux-detail-hero-grid">
              <div className="lux-cause-panel">
                <div className="lux-eyebrow">CAUSE</div>
                <h3>{summary.root_cause_summary || "-"}</h3>
                <p>{detail?.narrative || "-"}</p>

                <div className="lux-detail-meta-wrap">
                  {(summary.recurring_count || summary.recurringCount) > 0 ? (
                    <span className="lux-mini-chip">
                      Recurring · {summary.recurring_count || summary.recurringCount}
                    </span>
                  ) : null}
                  <span className="lux-mini-chip">
                    Last seen {formatTimestamp(summary.last_seen_at || summary.lastSeenAt)}
                  </span>
                  {(summary.similar_incident_id || summary.similarIncidentID) ? (
                    <span className="lux-mini-chip">
                      Similar {summary.similar_incident_id || summary.similarIncidentID}
                    </span>
                  ) : null}
                  <span className="lux-mini-chip">
                    Signature {intelligence.correlationSignature}
                  </span>
                </div>
              </div>

              <div className="lux-kpi-grid">
                <DetailKpi label="Confidence" value={summary.confidence ?? incident.confidence ?? "-"} />
                <DetailKpi label="Risk Score" value={summary.risk_score ?? incident.risk_score ?? "-"} />
                <DetailKpi label="Impact" value={`${impact.impact_count ?? summary.impact_count ?? 0} services`} />
                <DetailKpi label="Pattern" value={summary.root_cause_type || "-"} />
              </div>
            </div>
          </section>

          <section className="lux-tabs-card">
            <div className="lux-tab-bar">
              {[
                { key: "overview", label: "Overview" },
                { key: "activity", label: "Activity" },
                { key: "actions", label: "Actions" },
                { key: "audit", label: "Audit" },
              ].map((tab) => (
                <button
                  key={tab.key}
                  className={`lux-tab ${activeTab === tab.key ? "active" : ""}`}
                  onClick={() => setActiveTab(tab.key)}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            {detailLoading ? (
              <div className="lux-muted">Loading incident detail...</div>
            ) : (
              <>
                {activeTab === "overview" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">AI LAYER</div>
                            <h3>AI Explanation</h3>
                          </div>
                          <span className="lux-pill">Plain English</span>
                        </div>
                        <div className="lux-long-text">{explanation || "Failed to fetch"}</div>
                      </section>

                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">RCA</div>
                            <h3>Ranked Root Causes</h3>
                          </div>
                        </div>
                        <div className="lux-cause-candidate-list">
                          {intelligence.causeCandidates.map((candidate) => (
                            <div key={candidate.name} className="lux-cause-candidate-card">
                              <div className="lux-cause-candidate-top">
                                <strong>{candidate.name}</strong>
                                <span className="lux-mini-chip">{candidate.confidence}% confidence</span>
                              </div>
                              <div className="lux-action-subtext">{candidate.rationale}</div>
                            </div>
                          ))}
                        </div>
                      </section>
                    </div>

                    <section className="lux-card">
                      <div className="lux-section-head">
                        <div>
                          <div className="lux-eyebrow">COPILOT</div>
                          <h3>Incident Copilot</h3>
                        </div>
                      </div>

                      <div className="lux-copilot-quick">
                        {[
                          "Why is this happening?",
                          "What should I do first?",
                          "What changed?",
                          "Has this happened before?",
                        ].map((question) => (
                          <button
                            key={question}
                            className="lux-secondary-btn small"
                            onClick={() => {
                              setCopilotQuestion(question);
                              askCopilot(question);
                            }}
                          >
                            {question}
                          </button>
                        ))}
                      </div>

                      <div className="lux-copilot-bar">
                        <input
                          value={copilotQuestion}
                          onChange={(e) => setCopilotQuestion(e.target.value)}
                          placeholder="Ask about this incident..."
                        />
                        <button className="lux-primary-btn" onClick={() => askCopilot(copilotQuestion)}>
                          Ask
                        </button>
                      </div>

                      {copilotAnswer ? (
                        <div className="lux-copilot-answer">
                          <div className="lux-long-text">{copilotAnswer.answer || "-"}</div>
                          {Array.isArray(copilotAnswer.suggested_followups) && copilotAnswer.suggested_followups.length > 0 ? (
                            <div className="lux-copilot-followups">
                              {copilotAnswer.suggested_followups.map((item) => (
                                <button
                                  key={item}
                                  className="lux-secondary-btn small"
                                  onClick={() => {
                                    setCopilotQuestion(item);
                                    askCopilot(item);
                                  }}
                                >
                                  {item}
                                </button>
                              ))}
                            </div>
                          ) : null}
                        </div>
                      ) : null}
                    </section>

                    <div className="lux-two-grid">
                      <section className="lux-card">
                        <div className="lux-eyebrow">ACTION PLANNER</div>
                        <div className="lux-week2-action-stack">
                          <div className="lux-week2-action-card">
                            <div className="lux-week2-action-label">Next Best Action</div>
                            <h3>{intelligence.nextBestAction.label}</h3>
                            <p>{intelligence.nextBestAction.description}</p>
                          </div>
                          <div className="lux-week2-action-card">
                            <div className="lux-week2-action-label">Fallback Action</div>
                            <h3>{intelligence.fallbackAction.label}</h3>
                            <p>{intelligence.fallbackAction.description}</p>
                          </div>
                        </div>
                      </section>

                      <section className="lux-card">
                        <div className="lux-eyebrow">CONFIDENCE MODEL</div>
                        <div className="lux-confidence-breakdown-list">
                          {intelligence.confidenceBreakdown.map((item) => (
                            <div key={item.label} className="lux-confidence-item">
                              <div className="lux-confidence-head">
                                <span>{item.label}</span>
                                <strong>{item.value}%</strong>
                              </div>
                              <div className="lux-confidence-bar">
                                <div className="lux-confidence-fill" style={{ width: `${item.value}%` }} />
                              </div>
                              <div className="lux-action-subtext">{item.note}</div>
                            </div>
                          ))}
                        </div>
                      </section>
                    </div>

                    <div className="lux-three-grid">
                      <InfoCard
                        eyebrow="DECISION SUMMARY"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            <div className="lux-chip-row">
                              <span className="lux-mini-chip">Pattern {summary.root_cause_type || "-"}</span>
                              <span className="lux-mini-chip">Events {summary.event_count || 0}</span>
                              <span className="lux-mini-chip">Score {summary.correlation_score || 0}</span>
                            </div>
                            <div><strong>Latest event</strong><br />{formatTimestamp(summary.latest_event_time)}</div>
                            <div><strong>Reasoning</strong><br />{(detail?.incident?.reasoning || []).join(", ") || "-"}</div>
                            <div><strong>Signature</strong><br />{intelligence.correlationSignature}</div>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow="WHAT CHANGED"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            {detail?.what_changed?.type ? (
                              <>
                                <div><strong>Type</strong><br />{detail.what_changed.type}</div>
                                <div><strong>Service</strong><br />{detail.what_changed.service || "-"}</div>
                                <div><strong>Version</strong><br />{detail.what_changed.version || "-"}</div>
                                <div><strong>Description</strong><br />{detail.what_changed.description || "-"}</div>
                                <div><strong>Timestamp</strong><br />{formatTimestamp(detail.what_changed.timestamp)}</div>
                              </>
                            ) : (
                              <div>No recent deployment, config, or infra change is currently linked.</div>
                            )}
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow="OPERATOR GUIDANCE"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            <span className="lux-pill guidance-pill">
                              {String(insight.confidence || "medium").toUpperCase()} CONFIDENCE
                            </span>
                            <div>{insight.suggested_action || detail?.recommended_next_step || "-"}</div>
                            <div><strong>Evidence highlights</strong></div>
                            <ul className="lux-bullet-list compact">
                              {intelligence.evidenceHighlights.length > 0 ? (
                                intelligence.evidenceHighlights.map((item, index) => (
                                  <li key={index}>{item}</li>
                                ))
                              ) : (
                                <li>No evidence highlights available.</li>
                              )}
                            </ul>
                          </div>
                        }
                      />
                    </div>

                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Why This Is Likely"
                        body={
                          Array.isArray(insight.why_this_is_likely) && insight.why_this_is_likely.length > 0 ? (
                            <ul className="lux-bullet-list">
                              {insight.why_this_is_likely.map((item, index) => (
                                <li key={index}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            <div>-</div>
                          )
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Recommended Checks"
                        body={
                          Array.isArray(insight.recommended_checks) && insight.recommended_checks.length > 0 ? (
                            <ul className="lux-bullet-list">
                              {insight.recommended_checks.map((item, index) => (
                                <li key={index}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            <div>-</div>
                          )
                        }
                      />
                    </div>

                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Service Graph"
                        body={
                          <div className="lux-graph-wrap">
                            <div className="lux-graph-col">
                              <h4>Nodes</h4>
                              <div className="lux-graph-stack">
                                {(graph.nodes || []).map((node) => (
                                  <div className="lux-graph-node" key={node.id}>
                                    <div className="lux-graph-node-title">{node.label || node.id}</div>
                                    <div className="lux-graph-node-meta">
                                      {node.node_type || "-"} {node.severity ? `· ${node.severity}` : ""}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>

                            <div className="lux-graph-col">
                              <h4>Relationships</h4>
                              <div className="lux-graph-stack">
                                {(graph.edges || []).map((edge, index) => (
                                  <div className="lux-graph-edge" key={`${edge.from}-${edge.to}-${index}`}>
                                    <strong>{edge.from}</strong>
                                    <span>→</span>
                                    <strong>{edge.to}</strong>
                                    <small>{edge.relation}</small>
                                  </div>
                                ))}
                              </div>
                            </div>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Related Incidents"
                        body={
                          intelligence.relatedIncidents.length > 0 ? (
                            <div className="lux-related-incident-list">
                              {intelligence.relatedIncidents.map((item) => (
                                <div key={item.id} className="lux-related-incident-card">
                                  <div className="lux-chip-row">
                                    <span className={`lux-pill ${severityClass(item.severity)}`}>{item.severity}</span>
                                    <span className={`lux-pill ${statusClass(item.status)}`}>{item.status}</span>
                                  </div>
                                  <strong>{item.title}</strong>
                                  <div className="lux-action-subtext">{item.service}</div>
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">Confidence {item.confidence}</span>
                                    <span className="lux-mini-chip">Risk {item.risk}</span>
                                  </div>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <div>No related incidents found in the current dataset.</div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}

                {activeTab === "activity" ? (
                  <div className="lux-overview-grid">
                    <section className="lux-card">
                      <div className="lux-section-head">
                        <h3>Unified Activity Timeline</h3>
                      </div>
                      {activity.length === 0 ? (
                        <div className="lux-muted">Failed to fetch</div>
                      ) : (
                        <div className="lux-activity-list">
                          {activity.map((item, index) => (
                            <div key={item.id || index} className="lux-activity-item">
                              <div className="lux-activity-title">
                                {item.title || item.label || item.type || "Activity"}
                              </div>
                              <div className="lux-activity-body">
                                {item.message || item.description || "-"}
                              </div>
                              <div className="lux-activity-time">
                                {formatTimestamp(item.timestamp || item.created_at)}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </section>

                    <section className="lux-card">
                      <div className="lux-section-head">
                        <h3>Timeline Story</h3>
                      </div>
                      {timeline.length === 0 ? (
                        <div className="lux-muted">No timeline available.</div>
                      ) : (
                        <div className="lux-story-list">
                          {timeline.map((item, index) => (
                            <div key={`${item.event?.id || index}-${index}`} className="lux-story-item">
                              <div className="lux-chip-row">
                                <span className="lux-mini-chip">{item.signal_type || "-"}</span>
                                <span className="lux-mini-chip">{formatTimestamp(item.event?.timestamp)}</span>
                                <span className="lux-mini-chip">{item.stage_type || "-"}</span>
                              </div>
                              <strong>{item.story_label || "-"}</strong>
                              <div>{item.event?.message || "-"}</div>
                            </div>
                          ))}
                        </div>
                      )}
                    </section>
                  </div>
                ) : null}

                {activeTab === "actions" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Impact Analysis"
                        body={
                          <div className="lux-dense-block">
                            <div><strong>Primary Service:</strong> {impact.primary_service || "-"}</div>
                            <div><strong>Impact Level:</strong> {String(impact.impact_level || "-").toUpperCase()}</div>
                            <div><strong>Downstream Services:</strong></div>
                            <ul className="lux-bullet-list compact">
                              {(impact.downstream || []).map((item) => <li key={item}>{item}</li>)}
                            </ul>
                            <div><strong>Affected Services:</strong></div>
                            <ul className="lux-bullet-list compact">
                              {(impact.affected_services || []).map((item) => <li key={item}>{item}</li>)}
                            </ul>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Recommended Actions"
                        body={
                          actions.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-action-stack">
                              {actions.map((action) => (
                                <div key={action.id} className="lux-action-card">
                                  <div className="lux-action-top">
                                    <strong>{action.label}</strong>
                                    <span className={`lux-risk-badge ${riskClass(action.risk_level)}`}>
                                      {String(action.risk_level || "low").toUpperCase()} RISK
                                    </span>
                                  </div>
                                  <div>{action.description}</div>
                                  <div className="lux-action-subtext">
                                    Type: {action.type} · Approval: {action.requires_approval ? "Required" : "Not required"}
                                  </div>
                                  <button className="lux-primary-btn small" onClick={() => executeAction(action)}>
                                    Execute
                                  </button>
                                </div>
                              ))}
                            </div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}

                {activeTab === "audit" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Status Audit"
                        body={
                          statusAudit.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-audit-stack">
                              {statusAudit.map((item) => (
                                <div key={item.id} className="lux-audit-card">
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">
                                      {item.previous_status} → {item.new_status}
                                    </span>
                                    <span className="lux-mini-chip">{formatTimestamp(item.changed_at)}</span>
                                  </div>
                                  <div>{item.note || "-"}</div>
                                  <div className="lux-action-subtext">Changed by {item.changed_by || "-"}</div>
                                </div>
                              ))}
                            </div>
                          )
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Action Audit"
                        body={
                          actionAudit.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-audit-stack">
                              {actionAudit.map((item, index) => (
                                <div key={item.id || index} className="lux-audit-card">
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">{item.action_id || item.action || "-"}</span>
                                    <span className="lux-mini-chip">{String(item.status || "-").toUpperCase()}</span>
                                  </div>
                                  <div>{item.message || item.note || "-"}</div>
                                  <div className="lux-action-subtext">
                                    {item.approved_at
                                      ? `Approved ${formatTimestamp(item.approved_at)}`
                                      : formatTimestamp(item.created_at)}
                                  </div>
                                </div>
                              ))}
                            </div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}
              </>
            )}
          </section>
        </main>

        <aside className="lux-right-rail">
          <section className="lux-card lux-sticky-rail">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">OPERATOR RAIL</div>
                <h3>Live execution context</h3>
              </div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Selected incident</div>
              <div className="lux-rail-title">{incident.title || "-"}</div>
              <div className="lux-rail-subtext">{incident.service || "-"} · {incident.status || "-"} · {incident.severity || "-"}</div>
            </div>

            <div className="lux-rail-grid">
              <RailStat label="Confidence" value={summary.confidence ?? incident.confidence ?? "-"} />
              <RailStat label="Risk" value={summary.risk_score ?? incident.risk_score ?? "-"} />
              <RailStat label="Impact" value={impact.impact_count ?? summary.impact_count ?? "-"} />
              <RailStat label="Events" value={summary.event_count ?? 0} />
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Next best action</div>
              <div className="lux-rail-title">{intelligence.nextBestAction.label}</div>
              <div className="lux-rail-subtext">{intelligence.nextBestAction.description}</div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Fallback action</div>
              <div className="lux-rail-subtext">{intelligence.fallbackAction.label}</div>
              <div className="lux-rail-subtext">{intelligence.fallbackAction.description}</div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Quick actions</div>
              <div className="lux-rail-actions">
                <button className="lux-secondary-btn" onClick={() => updateStatus("ack")}>Acknowledge</button>
                <button className="lux-primary-btn" onClick={() => updateStatus("resolve")}>Resolve</button>
                <button className="lux-secondary-btn" onClick={() => updateStatus("reopen")}>Reopen</button>
              </div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Operator guidance</div>
              <div className="lux-guidance-card">
                <span className="lux-pill guidance-pill">
                  {String(insight.confidence || "medium").toUpperCase()} CONFIDENCE
                </span>
                <p>{insight.suggested_action || detail?.recommended_next_step || "-"}</p>
              </div>
            </div>
          </section>
        </aside>
      </div>
    </div>
  );
}

function MetricCard({ label, value }) {
  return (
    <div className="lux-metric-card">
      <div className="lux-metric-label">{label}</div>
      <div className="lux-metric-value">{value}</div>
    </div>
  );
}

function ScenarioCard({ title, description, onRun, onReset }) {
  return (
    <div className="lux-scenario-card">
      <h4>{title}</h4>
      <p>{description}</p>
      <div className="lux-scenario-actions">
        <button className="lux-primary-btn small" onClick={onRun}>Run Scenario</button>
        <button className="lux-secondary-btn small" onClick={onReset}>Reset</button>
      </div>
    </div>
  );
}

function DetailKpi({ label, value }) {
  return (
    <div className="lux-detail-kpi">
      <div className="lux-metric-label">{label}</div>
      <div className="lux-detail-kpi-value">{value}</div>
    </div>
  );
}

function InfoCard({ eyebrow, title, body }) {
  return (
    <div className="lux-card">
      {eyebrow ? <div className="lux-eyebrow">{eyebrow}</div> : null}
      {title ? <h3>{title}</h3> : null}
      <div>{body}</div>
    </div>
  );
}

function RailStat({ label, value }) {
  return (
    <div className="lux-rail-stat">
      <div className="lux-rail-label">{label}</div>
      <div className="lux-rail-stat-value">{value}</div>
    </div>
  );
}

function LiveHealthCard({ label, value, note }) {
  return (
    <div className="lux-live-health-card">
      <div className="lux-rail-label">{label}</div>
      <div className="lux-live-health-value">{value}</div>
      <div className="lux-action-subtext">{note}</div>
    </div>
  );
}
