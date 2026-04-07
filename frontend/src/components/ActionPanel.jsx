import { useState } from "react";
import { apiRequest } from "../api/client";

function ActionPanel({ actions, incidentId }) {
  const [results, setResults] = useState({});
  const [loadingActionId, setLoadingActionId] = useState("");

  async function execute(action) {
    try {
      setLoadingActionId(action.id);

      const approved = action.requires_approval
        ? window.confirm(`Approve action "${action.label}"?`)
        : false;

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

  if (!actions || actions.length === 0) {
    return null;
  }

  return (
    <div className="detail-section">
      <h3>Recommended Actions</h3>

      {actions.map((action) => {
        const result = results[action.id];

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

            <button
              onClick={() => execute(action)}
              disabled={loadingActionId === action.id}
            >
              {loadingActionId === action.id ? "Executing..." : "Execute"}
            </button>

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
