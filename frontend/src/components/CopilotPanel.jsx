import React, { useState } from "react";

export default function CopilotPanel({ incidentId }) {
  const [question, setQuestion] = useState("Why is this happening?");
  const [loading, setLoading] = useState(false);
  const [answer, setAnswer] = useState(null);
  const [error, setError] = useState("");

  const askCopilot = async (input) => {
    if (!incidentId || !input.trim()) return;

    setLoading(true);
    setError("");

    try {
      const response = await fetch(`/api/v1/incidents/copilot/${incidentId}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ question: input }),
      });

      if (!response.ok) {
        throw new Error("Copilot request failed");
      }

      const data = await response.json();
      setAnswer(data);
    } catch (err) {
      setError(err.message || "Failed to fetch copilot response");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Copilot</h3>
      </div>

      <div className="copilot-input-row">
        <input
          className="copilot-input"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="Ask about this incident"
        />
        <button className="copilot-button" onClick={() => askCopilot(question)} disabled={loading}>
          {loading ? "Asking..." : "Ask"}
        </button>
      </div>

      {error ? <div className="error-text">{error}</div> : null}

      {answer ? (
        <div className="copilot-answer">
          <div className="copilot-answer-label">Intent: {answer.intent || "-"}</div>
          <div className="copilot-answer-text">{answer.answer || "-"}</div>

          {Array.isArray(answer.suggested_followups) && answer.suggested_followups.length > 0 ? (
            <div className="copilot-followups">
              {answer.suggested_followups.map((item) => (
                <button
                  key={item}
                  className="followup-chip"
                  onClick={() => {
                    setQuestion(item);
                    askCopilot(item);
                  }}
                >
                  {item}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : (
        <div className="empty-state">Ask the copilot why this is happening, what changed, or what to do first.</div>
      )}
    </div>
  );
}
