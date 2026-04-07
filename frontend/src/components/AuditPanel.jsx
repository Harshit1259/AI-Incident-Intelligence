import { useEffect, useState } from "react";
import { apiRequest } from "../api/client";

const AUDIT_REFRESH_INTERVAL_MS = 10000;

function formatTime(value) {
  if (!value) {
    return "-";
  }

  return new Date(value).toLocaleString();
}

function AuditPanel({ incidentId }) {
  const [audits, setAudits] = useState([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!incidentId) {
      setAudits([]);
      setError("");
      return undefined;
    }

    let isActive = true;

    async function fetchAudits() {
      try {
        const data = await apiRequest(`/actions/audit?incident_id=${incidentId}`);

        if (!isActive) {
          return;
        }

        setAudits(Array.isArray(data) ? data : []);
        setError("");
      } catch (err) {
        if (!isActive) {
          return;
        }

        setAudits([]);
        setError(err.message || "Failed to load audit logs.");
      }
    }

    fetchAudits();

    const intervalId = window.setInterval(fetchAudits, AUDIT_REFRESH_INTERVAL_MS);

    return () => {
      isActive = false;
      window.clearInterval(intervalId);
    };
  }, [incidentId]);

  return (
    <div className="detail-section">
      <h3>Action Audit</h3>

      {error ? <p>{error}</p> : null}

      {!error && !audits.length ? <p>No actions executed yet.</p> : null}

      {audits.map((audit, index) => (
        <div key={`${audit.action_id || "action"}-${audit.executed_at || index}`} className="audit-item">
          <div className="action-item-top">
            <strong>{audit.action_id || "unknown action"}</strong>
            <span className="signal-chip">{(audit.status || "unknown").toUpperCase()}</span>
          </div>

          <p>{audit.message || "-"}</p>

          <p className="audit-meta-line">
            {audit.approved ? "Approved" : "Not approved"} · {formatTime(audit.executed_at)}
          </p>
        </div>
      ))}
    </div>
  );
}

export default AuditPanel;
