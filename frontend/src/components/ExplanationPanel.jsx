import React from "react";

export default function ExplanationPanel({ detail }) {
  if (!detail) return null;

  const summary = detail.summary || {};
  const insight = detail.insight || {};
  const evidence = detail.evidence || [];
  const narrative = detail.narrative || "";

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Explanation</h3>
      </div>

      <div className="explanation-block">
        <div className="explanation-title">Root Cause Summary</div>
        <div className="explanation-text">
          {summary.root_cause_summary || "No root cause summary available."}
        </div>
      </div>

      <div className="explanation-block">
        <div className="explanation-title">Narrative</div>
        <div className="explanation-text">
          {narrative || "No narrative available."}
        </div>
      </div>

      <div className="explanation-block">
        <div className="explanation-title">Why This Is Likely</div>
        {Array.isArray(insight.why_this_is_likely) && insight.why_this_is_likely.length > 0 ? (
          <ul className="bullet-list">
            {insight.why_this_is_likely.map((item, index) => (
              <li key={index}>{item}</li>
            ))}
          </ul>
        ) : (
          <div className="empty-inline">No reasoning points available.</div>
        )}
      </div>

      <div className="explanation-block">
        <div className="explanation-title">Evidence</div>
        {evidence.length > 0 ? (
          <ul className="bullet-list">
            {evidence.map((item, index) => (
              <li key={index}>{item}</li>
            ))}
          </ul>
        ) : (
          <div className="empty-inline">No evidence available.</div>
        )}
      </div>
    </div>
  );
}
