import { useEffect, useMemo, useState } from "react";
import { getIncidentExplanation } from "../api/incidents";
import ImpactPanel from "./ImpactPanel";
import ActionPanel from "./ActionPanel";
import AuditPanel from "./AuditPanel";
import CopilotPanel from "./CopilotPanel";
import ActivityTimeline from "./ActivityTimeline";

function formatTime(value) {
  if (!value) {
    return "-";
  }

  return new Date(value).toLocaleString();
}

function formatPercentage(value) {
  if (value === undefined || value === null || value === "") {
    return "-";
  }

  const numericValue = Number(value);
  if (Number.isNaN(numericValue)) {
    return "-";
  }

  return `${Math.round(numericValue)}%`;
}

function cleanIncidentTitle(title) {
  if (!title) {
    return "Untitled incident";
  }

  return title.replace(/\s*\(\d+\s+events?\)\s*$/i, "").trim();
}

function getInsightConfidenceClass(confidence) {
  const normalized = String(confidence || "").toLowerCase();

  if (normalized === "high") {
    return "pill-high";
  }

  if (normalized === "medium") {
    return "pill-medium";
  }

  return "pill-low";
}

function formatRiskLabel(value) {
  const riskScore = Number(value || 0);

  if (riskScore >= 80) {
    return "High";
  }

  if (riskScore >= 50) {
    return "Medium";
  }

  return "Low";
}

function IncidentDetailPanel({ detail, actionLoading, onAction }) {
  const [explanation, setExplanation] = useState("");
  const [explanationLoading, setExplanationLoading] = useState(false);
  const [explanationError, setExplanationError] = useState("");

  useEffect(() => {
    if (!detail?.incident?.id) {
      setExplanation("");
      setExplanationError("");
      setExplanationLoading(false);
      return;
    }

    let isActive = true;

    async function loadExplanation() {
      setExplanationLoading(true);
      setExplanationError("");

      try {
        const payload = await getIncidentExplanation(detail.incident.id);

        if (!isActive) {
          return;
        }

        setExplanation(payload?.explanation || "");
      } catch (error) {
        if (!isActive) {
          return;
        }

        setExplanation("");
        setExplanationError(error.message || "Failed to load AI explanation.");
      } finally {
        if (isActive) {
          setExplanationLoading(false);
        }
      }
    }

    loadExplanation();

    return () => {
      isActive = false;
    };
  }, [detail?.incident?.id]);

  const primaryAction = useMemo(() => {
    if (!detail) {
      return null;
    }

    if (detail.primary_action) {
      return detail.primary_action;
    }

    if (Array.isArray(detail.actions) && detail.actions.length > 0) {
      return detail.actions[0];
    }

    return null;
  }, [detail]);

  if (!detail?.incident) {
    return (
      <section className="panel detail-panel empty-panel">
        <p>Select an incident to inspect its timeline, RCA, and lifecycle actions.</p>
      </section>
    );
  }

  const {
    incident,
    summary,
    insight,
    events = [],
    impact,
    actions = [],
    decision_card: decisionCard = {},
    what_changed: whatChanged = {},
    narrative,
    status_audit: statusAudit = [],
  } = detail;

  return (
    <section className="panel detail-panel">
      <div className="detail-header">
        <div>
          <div className="detail-eyebrow-row">
            <p className="panel-eyebrow">Incident Detail</p>
            <span className={`detail-severity-pill severity-${incident.severity}`}>
              {incident.severity}
            </span>
            {summary?.seen_before ? (
              <span className="recurring-badge detail-recurring-badge">
                Seen before{summary?.recurring_count > 0 ? ` · ${summary.recurring_count}` : ""}
              </span>
            ) : null}
          </div>

          <h2 className="panel-title">{cleanIncidentTitle(decisionCard.title || incident.title)}</h2>
          <p className="detail-subtitle">
            {incident.service} · {incident.severity} · {incident.status}
          </p>
        </div>

        <div className="action-row">
          {incident.status === "open" && (
            <>
              <button
                type="button"
                className="secondary-button"
                onClick={() => onAction("ack")}
                disabled={actionLoading}
              >
                Acknowledge
              </button>
              <button
                type="button"
                className="primary-button"
                onClick={() => onAction("resolve")}
                disabled={actionLoading}
              >
                Resolve
              </button>
            </>
          )}

          {incident.status === "acknowledged" && (
            <>
              <button
                type="button"
                className="secondary-button"
                onClick={() => onAction("reopen")}
                disabled={actionLoading}
              >
                Reopen
              </button>
              <button
                type="button"
                className="primary-button"
                onClick={() => onAction("resolve")}
                disabled={actionLoading}
              >
                Resolve
              </button>
            </>
          )}

          {incident.status === "resolved" && (
            <button
              type="button"
              className="secondary-button"
              onClick={() => onAction("reopen")}
              disabled={actionLoading}
            >
              Reopen
            </button>
          )}
        </div>
      </div>

      <div className="decision-card">
        <div className="decision-main">
          <p className="decision-label">Cause</p>
          <h3>{decisionCard.cause || summary?.root_cause_summary || "No cause available yet."}</h3>
          <p className="decision-narrative">{narrative || "No narrative available yet."}</p>

          {decisionCard.seen_before ? (
            <div className="decision-recurring-row">
              <span className="recurring-badge">
                Recurring incident{decisionCard.recurring_count > 0 ? ` · ${decisionCard.recurring_count}` : ""}
              </span>
              {summary?.last_seen_at ? (
                <span className="signal-chip">Last seen {formatTime(summary.last_seen_at)}</span>
              ) : null}
              {summary?.similar_incident_id ? (
                <span className="signal-chip">Similar {summary.similar_incident_id}</span>
              ) : null}
            </div>
          ) : null}
        </div>

        <div className="decision-metrics">
          <div className="decision-metric">
            <span>Confidence</span>
            <strong>{formatPercentage(decisionCard.confidence ?? summary?.confidence)}</strong>
          </div>
          <div className="decision-metric">
            <span>Risk Score</span>
            <strong>{decisionCard.risk_score ?? summary?.risk_score ?? 0}</strong>
          </div>
          <div className="decision-metric">
            <span>Impact</span>
            <strong>{decisionCard.impact_count ?? summary?.impact_count ?? 0} services</strong>
          </div>
        </div>
      </div>

      <div className="detail-section ai-explanation-card">
        <div className="ai-explanation-header">
          <div>
            <p className="panel-eyebrow">AI Layer</p>
            <h3>AI Explanation</h3>
          </div>
          <span className="signal-chip">Plain English</span>
        </div>

        {explanationLoading ? <p>Generating explanation...</p> : null}
        {!explanationLoading && explanationError ? <p>{explanationError}</p> : null}
        {!explanationLoading && !explanationError ? (
          <p className="ai-explanation-text">{explanation || "No explanation available yet."}</p>
        ) : null}
      </div>

      <CopilotPanel incidentId={incident.id} />

      {primaryAction ? (
        <div className="detail-section primary-action-card">
          <div className="primary-action-header">
            <div>
              <p className="panel-eyebrow">Recommended First Move</p>
              <h3>{primaryAction.label}</h3>
            </div>
            <div className="primary-action-badges">
              <span className="signal-chip">{primaryAction.type || "action"}</span>
              <span className="signal-chip">Risk {formatRiskLabel(primaryAction.risk_level)}</span>
              <span className="signal-chip">
                {primaryAction.requires_approval ? "Approval required" : "No approval"}
              </span>
            </div>
          </div>

          <p className="primary-action-description">
            {primaryAction.description || "No action description available."}
          </p>
        </div>
      ) : null}

      <div className="detail-grid detail-grid-three">
        <div className="detail-section">
          <h3>Decision Summary</h3>

          <div className="summary-chip-row">
            <span className="signal-chip">
              Pattern {summary?.correlation_pattern || incident.correlation_pattern || "-"}
            </span>
            <span className="signal-chip">
              Events {summary?.event_count || incident.event_count || 0}
            </span>
            <span className="signal-chip">
              Score {summary?.correlation_score || incident.correlation_score || 0}
            </span>
          </div>

          <div className="summary-stack">
            <div>
              <span className="summary-label">Latest Event</span>
              <p>{formatTime(summary?.latest_event_time || incident.last_event_time)}</p>
            </div>
            <div>
              <span className="summary-label">Reasoning</span>
              <p>{summary?.correlation_reason || incident.correlation_reason || "-"}</p>
            </div>
            <div>
              <span className="summary-label">Root Cause Type</span>
              <p>{summary?.root_cause_type || incident.root_cause_type || "-"}</p>
            </div>
          </div>
        </div>

        <div className="detail-section">
          <h3>What Changed</h3>

          {whatChanged?.type ? (
            <div className="change-card">
              <div className="change-pill-row">
                <span className="change-badge">{whatChanged.type}</span>
                <span className="signal-chip">{whatChanged.version || "-"}</span>
              </div>

              <p className="change-title">{whatChanged.service || incident.service}</p>
              <p>{whatChanged.description || "Recent change linked to the incident window."}</p>

              <div className="summary-stack compact">
                <div>
                  <span className="summary-label">Timestamp</span>
                  <p>{formatTime(whatChanged.timestamp)}</p>
                </div>
              </div>
            </div>
          ) : (
            <p>No recent deployment, config, or infra change is currently linked.</p>
          )}
        </div>

        <div className="detail-section">
          <h3>Operator Guidance</h3>

          <div className="guidance-pill-row">
            <span className={`confidence-pill ${getInsightConfidenceClass(insight?.confidence)}`}>
              {String(insight?.confidence || "low").toUpperCase()} CONFIDENCE
            </span>
          </div>

          <p className="detail-callout">
            {insight?.suggested_action || "No suggested action available yet."}
          </p>
        </div>
      </div>

      <div className="detail-grid detail-grid-two">
        <div className="detail-section">
          <h3>Why This Is Likely</h3>
          <ul className="detail-list">
            {(insight?.why_this_is_likely || incident.reasoning || []).map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>

        <div className="detail-section">
          <h3>Recommended Checks</h3>
          <ul className="detail-list">
            {(insight?.recommended_checks || []).map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
      </div>

      <ActivityTimeline incidentId={incident.id} />

      <div className="detail-section">
        <h3>Timeline Story</h3>

        <div className="timeline-list">
          {events.map((item, index) => (
            <div
              key={item?.event?.id || `${item?.event?.timestamp || "timeline"}-${index}`}
              className="timeline-item"
            >
              <div className="timeline-item-top">
                <div>
                  <span className="timeline-step-label">{item.story_label || `Step ${index + 1}`}</span>
                  <h4>{item?.event?.message || "Unknown event"}</h4>
                </div>

                <div className="timeline-item-badges">
                  <span className="signal-chip">{item.signal_type || "generic"}</span>
                  <span className="signal-chip">{item.stage_type || "progression"}</span>
                </div>
              </div>

              <p className="timeline-meta">
                {formatTime(item?.event?.timestamp)}
                {item.gap_from_previous ? ` · ${item.gap_from_previous}` : ""}
              </p>
            </div>
          ))}
        </div>
      </div>

      <div className="detail-grid detail-grid-two">
        <ImpactPanel impact={impact} />
        <ActionPanel actions={actions} incidentId={incident.id} />
      </div>

      <div className="detail-grid detail-grid-two">
        <div className="detail-section">
          <h3>Status Audit</h3>

          {statusAudit.length === 0 ? (
            <p>No lifecycle changes recorded yet.</p>
          ) : (
            <div className="status-audit-list">
              {statusAudit.map((entry) => (
                <div key={entry.id} className="status-audit-item">
                  <div className="status-audit-top">
                    <span className="signal-chip">
                      {entry.previous_status} → {entry.new_status}
                    </span>
                    <span className="signal-chip">{formatTime(entry.changed_at)}</span>
                  </div>
                  <p className="status-audit-note">{entry.note || "No note recorded."}</p>
                  <p className="status-audit-meta">Changed by {entry.changed_by || "system"}</p>
                </div>
              ))}
            </div>
          )}
        </div>

        <AuditPanel incidentId={incident.id} />
      </div>
    </section>
  );
}

export default IncidentDetailPanel;
