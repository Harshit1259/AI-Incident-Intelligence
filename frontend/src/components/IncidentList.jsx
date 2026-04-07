import React from "react";

export default function IncidentList({
  incidents,
  selectedIncidentId,
  onSelectIncident,
}) {
  return (
    <div className="panel incident-list-panel">
      <div className="panel-header">
        <h2>Active Incidents</h2>
      </div>

      <div className="incident-list">
        {incidents.length === 0 ? (
          <div className="empty-state">No incidents found.</div>
        ) : (
          incidents.map((incident) => {
            const isSelected = selectedIncidentId === incident.id;

            return (
              <button
                key={incident.id}
                className={`incident-list-item ${isSelected ? "selected" : ""}`}
                onClick={() => onSelectIncident(incident)}
              >
                <div className="incident-list-top">
                  <span className={`severity-badge severity-${String(incident.severity || "").toLowerCase()}`}>
                    {incident.severity || "unknown"}
                  </span>
                  <span className="incident-status">{incident.status || "open"}</span>
                </div>

                <div className="incident-title">
                  {incident.title || incident.service || incident.id}
                </div>

                <div className="incident-meta">
                  <span>{incident.service || "unknown service"}</span>
                  <span>Confidence: {incident.confidence ?? "-"}</span>
                </div>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}
