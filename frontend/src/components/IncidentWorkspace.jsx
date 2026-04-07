import React from "react";
import OverviewCards from "./OverviewCards";
import GraphPanel from "./GraphPanel";
import ExplanationPanel from "./ExplanationPanel";
import ActionPanel from "./ActionPanel";
import TimelinePanel from "./TimelinePanel";
import CopilotPanel from "./CopilotPanel";

export default function IncidentWorkspace({ detail }) {
  if (!detail) {
    return (
      <div className="workspace-empty">
        Select an incident to view details.
      </div>
    );
  }

  const incident = detail.incident || {};

  return (
    <div className="workspace">
      <div className="workspace-header">
        <div>
          <h1 className="workspace-title">
            {incident.title || incident.service || incident.id || "Incident"}
          </h1>
          <div className="workspace-subtitle">
            {incident.service || "-"} • {incident.status || "-"} • {incident.severity || "-"}
          </div>
        </div>
      </div>

      <OverviewCards detail={detail} />

      <div className="workspace-grid">
        <div className="workspace-column">
          <GraphPanel graph={detail.graph} />
          <TimelinePanel events={detail.events} />
        </div>

        <div className="workspace-column">
          <ExplanationPanel detail={detail} />
          <ActionPanel detail={detail} />
          <CopilotPanel incidentId={incident.id} />
        </div>
      </div>
    </div>
  );
}
