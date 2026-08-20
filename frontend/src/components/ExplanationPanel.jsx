import { useState } from "react";
import EvidenceGraphPanel from "./EvidenceGraphPanel.jsx";

export default function ExplanationPanel({ detail }) {
  const [showEvidence, setShowEvidence] = useState(true);

  if (!detail) return null;

  const summary   = detail.summary || {};
  const insight   = detail.insight || {};
  const evidence  = detail.evidence || [];
  const narrative = detail.narrative || "";
  const eg        = detail.evidence_graph || null;

  return (
    <div className="panel">
      <div className="panel-header" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h3>Explanation</h3>
        {eg && (
          <button
            style={{ fontSize: "0.7rem", padding: "3px 10px", borderRadius: 6, cursor: "pointer", background: showEvidence ? "rgba(58,167,255,0.12)" : "transparent", color: showEvidence ? "#3aa7ff" : "#64748b", border: `1px solid ${showEvidence ? "rgba(58,167,255,0.28)" : "rgba(90,123,186,0.2)"}` }}
            onClick={() => setShowEvidence(v => !v)}
          >
            {showEvidence ? "Hide evidence" : "Show evidence"}
          </button>
        )}
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

      {/* Evidence graph — sources, per-claim confidence, conflicting signals, falsification */}
      {eg && showEvidence && <EvidenceGraphPanel graph={eg} />}
    </div>
  );
}
