import { useState, useRef, useEffect } from "react";
import EvidenceGraphPanel from "./EvidenceGraphPanel.jsx";

const INTENT_LABEL = {
  why:          { label: "Root Cause",   color: "#f87171" },
  first_action: { label: "Action",       color: "#34d399" },
  change:       { label: "Change",       color: "#a78bfa" },
  history:      { label: "History",      color: "#38bdf8" },
  general:      { label: "General",      color: "#94a3b8" },
};

const PRESET_QUESTIONS = [
  "Why is this happening?",
  "What should I do first?",
  "What changed?",
  "Has this happened before?",
];

export default function CopilotPanel({ incidentId }) {
  const [question, setQuestion] = useState("");
  const [loading, setLoading]   = useState(false);
  const [answer, setAnswer]     = useState(null);
  const [error, setError]       = useState("");
  const [showEvidence, setShowEvidence] = useState(false);
  const inputRef = useRef(null);

  // Auto-focus input when panel opens.
  useEffect(() => { inputRef.current?.focus(); }, [incidentId]);

  const askCopilot = async (input) => {
    const q = (input || question).trim();
    if (!incidentId || !q) return;

    setLoading(true);
    setError("");
    setAnswer(null);

    try {
      const token = localStorage.getItem("authToken") || "";
      const response = await fetch(`/api/v1/incidents/copilot/${incidentId}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ question: q }),
      });

      if (!response.ok) throw new Error("Copilot request failed");
      const data = await response.json();
      setAnswer(data);
      setShowEvidence(false); // collapse evidence graph on new answer
    } catch (err) {
      setError(err.message || "Failed to fetch copilot response");
    } finally {
      setLoading(false);
    }
  };

  const handleKey = (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      askCopilot();
    }
  };

  const intentCfg = answer ? (INTENT_LABEL[answer.intent] || INTENT_LABEL.general) : null;

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>AI Copilot</h3>
      </div>

      {/* Preset questions */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 12 }}>
        {PRESET_QUESTIONS.map(q => (
          <button
            key={q}
            className="followup-chip"
            style={{ opacity: loading ? 0.5 : 1 }}
            disabled={loading}
            onClick={() => { setQuestion(q); askCopilot(q); }}
          >
            {q}
          </button>
        ))}
      </div>

      {/* Input row */}
      <div className="copilot-input-row">
        <input
          ref={inputRef}
          className="copilot-input"
          value={question}
          onChange={e => setQuestion(e.target.value)}
          onKeyDown={handleKey}
          placeholder="Ask anything about this incident…"
          disabled={loading}
        />
        <button className="copilot-button" onClick={() => askCopilot()} disabled={loading || !question.trim()}>
          {loading ? "Thinking…" : "Ask"}
        </button>
      </div>

      {error && <div className="error-text" style={{ marginTop: 8 }}>{error}</div>}

      {answer && (
        <div className="copilot-answer" style={{ marginTop: 14 }}>
          {/* Intent badge + answer */}
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6, flexWrap: "wrap" }}>
            {intentCfg && (
              <span style={{ fontSize: "0.62rem", padding: "1px 8px", borderRadius: 5, fontWeight: 700, background: "rgba(255,255,255,0.05)", color: intentCfg.color, border: `1px solid ${intentCfg.color}33`, textTransform: "uppercase", letterSpacing: "0.06em" }}>
                {intentCfg.label}
              </span>
            )}
            {answer.evidence_graph && (
              <button
                style={{ marginLeft: "auto", fontSize: "0.65rem", padding: "2px 9px", borderRadius: 5, cursor: "pointer", background: showEvidence ? "rgba(58,167,255,0.10)" : "transparent", color: showEvidence ? "#3aa7ff" : "#64748b", border: `1px solid ${showEvidence ? "rgba(58,167,255,0.28)" : "rgba(90,123,186,0.18)"}` }}
                onClick={() => setShowEvidence(v => !v)}
              >
                {showEvidence ? "Hide evidence" : "Show evidence"}
                {answer.evidence_graph?.overall_confidence > 0 && (
                  <span style={{ marginLeft: 5, color: answer.evidence_graph.overall_confidence >= 70 ? "#34d399" : "#f59e0b" }}>
                    {answer.evidence_graph.overall_confidence}%
                  </span>
                )}
              </button>
            )}
          </div>

          <div className="copilot-answer-text" style={{ lineHeight: 1.65 }}>{answer.answer}</div>

          {/* Evidence graph — collapsed by default, toggle to expand */}
          {answer.evidence_graph && showEvidence && (
            <EvidenceGraphPanel graph={answer.evidence_graph} />
          )}

          {/* Follow-up chips */}
          {Array.isArray(answer.suggested_followups) && answer.suggested_followups.length > 0 && (
            <div className="copilot-followups" style={{ marginTop: 12 }}>
              {answer.suggested_followups.map(item => (
                <button
                  key={item}
                  className="followup-chip"
                  onClick={() => { setQuestion(item); askCopilot(item); }}
                >
                  {item}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {!answer && !loading && !error && (
        <div className="empty-state">Ask the copilot about this incident. Every answer comes with evidence sources, confidence scores, and what would falsify the diagnosis.</div>
      )}
    </div>
  );
}
