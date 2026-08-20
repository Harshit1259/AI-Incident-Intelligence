// WeeklyDigestPanel.jsx — Phase 3, Week 9
// Weekly reliability digest. Top 3 recurring issues, MTTR trend, reliability score.

import { useState, useEffect, useCallback } from "react";
import { getWeeklyDigest, sendDigest } from "../api/phase3.js";

const TREND_CONFIG = {
  improving: { icon: "📉", color: "#00D084", label: "Improving" },
  stable:    { icon: "➡️", color: "#6b7280", label: "Stable" },
  degrading: { icon: "📈", color: "#FF3B3B", label: "Degrading" },
};

function ScoreRing({ score }) {
  const color = score >= 80 ? "#00D084" : score >= 60 ? "#FFBC00" : "#FF3B3B";
  const pct = Math.max(0, Math.min(100, score));
  return (
    <div className="digest-score-ring">
      <svg viewBox="0 0 100 100" className="digest-ring-svg">
        <circle cx="50" cy="50" r="42" fill="none" stroke="rgba(255,255,255,0.08)" strokeWidth="8" />
        <circle cx="50" cy="50" r="42" fill="none" stroke={color} strokeWidth="8"
          strokeDasharray={`${pct * 2.64} ${264 - pct * 2.64}`}
          strokeLinecap="round" transform="rotate(-90 50 50)" />
      </svg>
      <div className="digest-score-text" style={{ color }}>
        <div className="digest-score-num">{score.toFixed(0)}</div>
        <div className="digest-score-label">Reliability</div>
      </div>
    </div>
  );
}

export default function WeeklyDigestPanel() {
  const [digest, setDigest] = useState(null);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [sendMessage, setSendMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try { const d = await getWeeklyDigest(); setDigest(d); }
    catch { setDigest(null); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleSend() {
    setSending(true);
    setSendMessage("");
    try { await sendDigest(); setSendMessage("Digest logged/sent successfully."); }
    catch (e) { setSendMessage("Failed: " + e.message); }
    finally { setSending(false); }
  }

  const trend = TREND_CONFIG[digest?.mttr_trend] || TREND_CONFIG.stable;

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">WEEKLY RELIABILITY DIGEST</div>
          <h2 style={{ margin: "0.25rem 0" }}>Reliability Report</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            Auto-generated. Top 3 recurring issues, MTTR trend, reliability score. CTOs read this.
          </div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", alignItems: "flex-end" }}>
          <div style={{ display: "flex", gap: "0.5rem" }}>
            <button className="lux-secondary-btn" onClick={load}>Refresh</button>
            <button className="lux-primary-btn" onClick={handleSend} disabled={sending}>
              {sending ? "Sending..." : "Send Digest Email"}
            </button>
          </div>
          {sendMessage && (
            <div style={{ fontSize: "0.8rem", color: sendMessage.startsWith("Failed") ? "#FF3B3B" : "#00D084" }}>
              {sendMessage}
            </div>
          )}
        </div>
      </div>

      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Generating digest...</div>
      ) : !digest ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>📧</div>
          <p>Unable to generate digest. Incident data is needed to compute reliability metrics.</p>
        </div>
      ) : (
        <>
          {/* Header row: score + period */}
          <div className="digest-hero">
            <ScoreRing score={digest.reliability_score || 0} />
            <div className="digest-hero-info">
              <div className="digest-period">
                {digest.week_start ? new Date(digest.week_start).toLocaleDateString() : "—"}
                {" — "}
                {digest.week_end ? new Date(digest.week_end).toLocaleDateString() : "—"}
              </div>
              <div className="digest-hero-kpis">
                <div className="digest-hero-kpi">
                  <div className="digest-hero-kpi-val">{digest.total_incidents}</div>
                  <div className="slo-metric-label">Total Incidents</div>
                </div>
                <div className="digest-hero-kpi">
                  <div className="digest-hero-kpi-val" style={{ color: "#FF3B3B" }}>{digest.critical_count}</div>
                  <div className="slo-metric-label">Critical</div>
                </div>
                <div className="digest-hero-kpi">
                  <div className="digest-hero-kpi-val">
                    {digest.mttr_seconds > 0 ? `${(digest.mttr_seconds / 60).toFixed(0)}m` : "—"}
                  </div>
                  <div className="slo-metric-label">Avg MTTR</div>
                </div>
                <div className="digest-hero-kpi">
                  <div className="digest-hero-kpi-val" style={{ color: trend.color }}>
                    {trend.icon} {trend.label}
                  </div>
                  <div className="slo-metric-label">MTTR Trend</div>
                </div>
              </div>
            </div>
          </div>

          {/* Top 3 Recurring Issues */}
          {(digest.top_recurring || []).length > 0 && (
            <div className="digest-section">
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>TOP RECURRING ISSUES</div>
              <div className="digest-recurring-list">
                {digest.top_recurring.map((issue, i) => (
                  <div key={i} className="digest-recurring-card">
                    <div className="digest-rank">#{i + 1}</div>
                    <div>
                      <div style={{ fontWeight: 600 }}>{issue.pattern || "Unknown pattern"}</div>
                      <div className="slo-service">{issue.service} · {issue.count} occurrences · Last: {issue.last_seen || "—"}</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Highlights */}
          {(digest.highlights || []).length > 0 && (
            <div className="digest-section">
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>HIGHLIGHTS</div>
              <ul className="digest-highlights">
                {digest.highlights.map((h, i) => <li key={i}>{h}</li>)}
              </ul>
            </div>
          )}

          <div className="digest-footer">
            Generated {digest.generated_at ? new Date(digest.generated_at).toLocaleString() : "just now"}
          </div>
        </>
      )}
    </div>
  );
}
