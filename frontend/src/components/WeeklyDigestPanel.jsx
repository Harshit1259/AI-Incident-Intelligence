/**
 * Weekly reliability digest — the report a CTO reads.
 *
 * Written as a report rather than a dashboard: one headline score, the period
 * it covers, then the findings. Emoji trend markers were replaced with real
 * icons so the trend reads at a glance and survives fonts that render emoji
 * inconsistently.
 */
import { useState, useEffect, useCallback } from "react";
import {
  AlertTriangle, ArrowRight, Clock, Flame, Mail, Minus,
  RefreshCw, TrendingDown, TrendingUp,
} from "lucide-react";

import { getWeeklyDigest, sendDigest } from "../api/phase3.js";
import {
  EmptyState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

/* MTTR trend. "Improving" means time-to-resolve is falling, so the arrow points
   down and the tone is positive — the opposite of a naive up-is-good mapping. */
const TREND_CONFIG = {
  improving: { icon: TrendingDown, tone: "tone-success", label: "Improving" },
  stable: { icon: Minus, tone: "tone-neutral", label: "Stable" },
  degrading: { icon: TrendingUp, tone: "tone-danger", label: "Degrading" },
};

function scoreTone(score) {
  if (score >= 80) return "tone-success";
  if (score >= 60) return "tone-warning";
  return "tone-danger";
}

function ScoreRing({ score }) {
  const pct = Math.max(0, Math.min(100, score));
  const circumference = 264; // 2πr, r = 42
  return (
    <div className={`score-ring ${scoreTone(score)}`}>
      <svg viewBox="0 0 100 100" className="score-ring-svg" aria-hidden="true">
        <circle cx="50" cy="50" r="42" className="score-ring-track" />
        <circle
          cx="50" cy="50" r="42"
          className="score-ring-value"
          strokeDasharray={`${(pct / 100) * circumference} ${circumference}`}
          transform="rotate(-90 50 50)"
        />
      </svg>
      <div className="score-ring-center">
        <span className="score-ring-num">{score.toFixed(0)}</span>
        <span className="score-ring-caption">reliability</span>
      </div>
    </div>
  );
}

export default function WeeklyDigestPanel() {
  const [digest, setDigest] = useState(null);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [sendResult, setSendResult] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setDigest(await getWeeklyDigest());
    } catch {
      setDigest(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleSend() {
    setSending(true);
    setSendResult(null);
    try {
      await sendDigest();
      setSendResult({ ok: true, message: "Digest sent." });
    } catch (e) {
      setSendResult({ ok: false, message: e.message || "Could not send digest" });
    } finally {
      setSending(false);
    }
  }

  const trend = TREND_CONFIG[digest?.mttr_trend] || TREND_CONFIG.stable;
  const TrendIcon = trend.icon;
  const period =
    digest?.week_start && digest?.week_end
      ? `${new Date(digest.week_start).toLocaleDateString()} — ${new Date(digest.week_end).toLocaleDateString()}`
      : "current week";

  return (
    <Page>
      <PageHeader
        title="Weekly reliability digest"
        meta={`Auto-generated · ${period}`}
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={load} disabled={loading}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
            </button>
            <button type="button" className="btn btn-primary" onClick={handleSend} disabled={sending}>
              {sending ? <span className="spinner spinner-xs" /> : <Mail size={13} />}
              {sending ? "Sending…" : "Send digest"}
            </button>
          </>
        }
      />

      {sendResult && (
        <div className={`callout callout-block ${sendResult.ok ? "tone-success" : "tone-danger"}`}>
          <AlertTriangle size={13} className="callout-icon" />
          <p className="callout-text">{sendResult.message}</p>
        </div>
      )}

      {loading ? (
        <SkeletonRows count={2} height={140} />
      ) : !digest ? (
        <EmptyState
          icon={Mail}
          title="No digest available"
          message="Incident data is needed before reliability metrics can be computed."
        />
      ) : (
        <>
          <Grid cols={4}>
            <StatTile label="Total incidents" icon={Flame} tone="neutral" value={digest.total_incidents ?? 0} sub="this period" />
            <StatTile
              label="Critical" icon={AlertTriangle}
              tone={(digest.critical_count || 0) > 0 ? "danger" : "success"}
              value={digest.critical_count ?? 0} sub="highest severity"
            />
            <StatTile
              label="Average MTTR" icon={Clock} tone="brand"
              value={digest.mttr_seconds > 0 ? `${(digest.mttr_seconds / 60).toFixed(0)}m` : "—"}
              sub="mean time to resolve"
            />
            <StatTile
              label="MTTR trend" icon={TrendIcon}
              tone={trend.tone.replace("tone-", "")}
              value={trend.label} sub="versus previous period"
            />
          </Grid>

          <div className="ui-split">
            <Panel title="Top recurring issues" sub="patterns that came back this period" flush>
              {(digest.top_recurring || []).length === 0 ? (
                <EmptyState
                  icon={ArrowRight}
                  tone="success"
                  title="Nothing recurring"
                  message="No repeated failure patterns in this period."
                />
              ) : (
                <ol className="rank-list">
                  {digest.top_recurring.map((issue, i) => (
                    <li key={i} className="rank-item">
                      <span className="rank-index">{i + 1}</span>
                      <span className="rank-text">
                        <span className="rank-title">{issue.pattern || "Unknown pattern"}</span>
                        <span className="rank-sub">
                          {issue.service} · {issue.count} occurrences · last {issue.last_seen || "—"}
                        </span>
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </Panel>

            <Panel title="Reliability score" sub="composite of volume, severity and MTTR">
              <div className="score-block">
                <ScoreRing score={digest.reliability_score || 0} />
              </div>
              {(digest.highlights || []).length > 0 && (
                <div className="detail-section is-divided">
                  <h4 className="detail-section-label">Highlights</h4>
                  <ul className="bullet-list">
                    {digest.highlights.map((h, i) => (
                      <li key={i}>{h}</li>
                    ))}
                  </ul>
                </div>
              )}
            </Panel>
          </div>

          <p className="page-footnote">
            Generated {digest.generated_at ? new Date(digest.generated_at).toLocaleString() : "just now"}
          </p>
        </>
      )}
    </Page>
  );
}
