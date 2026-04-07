import React from "react";

export default function OverviewCards({ detail }) {
  if (!detail) return null;

  const incident = detail.incident || {};
  const summary = detail.summary || {};
  const impact = detail.impact || {};

  const cards = [
    {
      label: "Service",
      value: incident.service || summary.service || "-",
    },
    {
      label: "Severity",
      value: incident.severity || summary.severity || "-",
    },
    {
      label: "Confidence",
      value: summary.confidence ?? incident.confidence ?? "-",
    },
    {
      label: "Risk",
      value: summary.risk_score ?? incident.risk_score ?? "-",
    },
    {
      label: "Impact Count",
      value: impact.impact_count ?? summary.impact_count ?? "-",
    },
    {
      label: "Pattern",
      value: summary.root_cause_type || incident.root_cause_type || "-",
    },
  ];

  return (
    <div className="overview-grid">
      {cards.map((card) => (
        <div className="overview-card" key={card.label}>
          <div className="overview-label">{card.label}</div>
          <div className="overview-value">{card.value}</div>
        </div>
      ))}
    </div>
  );
}
