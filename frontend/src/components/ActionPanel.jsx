import { useState } from "react";
import { apiRequest } from "../api/client";

function ActionPanel({ actions, incidentId }) {
  const [results, setResults] = useState({});
  const [loadingActionId, setLoadingActionId] = useState("");
  const [pendingApprovalId, setPendingApprovalId] = useState(null);

  async function executeWithApproval(action, approved) {
    try {
      setLoadingActionId(action.id);
      setPendingApprovalId(null);

      const payload = await apiRequest("/actions/execute", {
        method: "POST",
        body: JSON.stringify({
          action_id: action.id,
          incident_id: incidentId,
          approved,
        }),
      });

      setResults((currentResults) => ({
        ...currentResults,
        [action.id]: payload,
      }));
    } catch (error) {
      setResults((currentResults) => ({
        ...currentResults,
        [action.id]: {
          status: "failed",
          message: error.message || "Action request failed",
        },
      }));
    } finally {
      setLoadingActionId("");
    }
  }

  function handleExecute(action) {
    if (action.requires_approval) {
      setPendingApprovalId(action.id);
    } else {
      executeWithApproval(action, false);
    }
  }

  if (!actions || actions.length === 0) {
    return null;
  }

  return (
    <div className="detail-section">
      <h3>Recommended Actions</h3>

      {actions.map((action) => {
        const result = results[action.id];
        const awaitingApproval = pendingApprovalId === action.id;

        return (
          <div key={action.id} className="action-item">
            <div className="action-item-top">
              <strong>{action.label}</strong>
              <span className={`risk-${action.risk_level}`}>
                {String(action.risk_level || "low").toUpperCase()} RISK
              </span>
            </div>

            <p>{action.description}</p>

            <p className="action-meta">
              Type: {action.type} · Approval: {action.requires_approval ? "Required" : "Not required"}
            </p>

            {awaitingApproval ? (
              <div style={{ display: "flex", gap: "0.5rem", alignItems: "center", marginTop: "0.5rem" }}>
                <span style={{ fontSize: "0.85rem", color: "#f59e0b" }}>
                  Approve action &ldquo;{action.label}&rdquo;?
                </span>
                <button onClick={() => executeWithApproval(action, true)}>Approve</button>
                <button onClick={() => setPendingApprovalId(null)}>Cancel</button>
              </div>
            ) : (
              <button
                onClick={() => handleExecute(action)}
                disabled={loadingActionId === action.id}
              >
                {loadingActionId === action.id ? "Executing..." : "Execute"}
              </button>
            )}

            {result ? (
              <p className={`action-result action-result-${result.status}`}>
                {(result.status || "unknown").toUpperCase()}: {result.message}
              </p>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

export default ActionPanel;
