import React, { useState } from "react";
import { Zap, Eye, BookOpen, Loader2, AlertTriangle, RefreshCw, ChevronRight } from "lucide-react";
import { analyzeIncident } from "../api/incidents";

/**
 * AnalysisPanel renders an incident's causal analysis — or honestly reports
 * that none exists.
 *
 * The platform can describe an incident at three levels of epistemic strength,
 * and this component's whole job is to make them visually distinguishable:
 *
 *   observed       — facts we collected. No cause asserted, no confidence shown.
 *   knowledge_base — a pre-authored pattern matched. Deterministic.
 *   llm            — a model reasoned over the evidence. Attributed and dated.
 *
 * Before provenance existed, all three rendered identically: a "Root Cause"
 * heading with a confidence percentage. On the observed tier that heading held
 * a restated alert title and the percentage was derived from severity — so the
 * UI presented a symptom as a diagnosis and scored its own certainty about it.
 * That is the bug this component exists to prevent.
 */

const SOURCE_STYLES = {
  llm: {
    label: "AI analysis",
    Icon: Zap,
    accent: "var(--blue-lt, #4d9fff)",
    bg: "rgba(0,102,255,0.07)",
    border: "rgba(0,102,255,0.20)",
  },
  knowledge_base: {
    label: "Known pattern",
    Icon: BookOpen,
    accent: "var(--green, #35c759)",
    bg: "rgba(53,199,89,0.07)",
    border: "rgba(53,199,89,0.22)",
  },
  observed: {
    label: "Observed only",
    Icon: Eye,
    accent: "var(--t2, #8b95a5)",
    bg: "rgba(140,150,165,0.06)",
    border: "rgba(140,150,165,0.20)",
  },
};

function ProvenanceBadge({ provenance }) {
  const style = SOURCE_STYLES[provenance.source] || SOURCE_STYLES.observed;
  const { Icon } = style;

  const detailBits = [];
  if (provenance.model) detailBits.push(provenance.model);
  if (provenance.analyzed_by) detailBits.push(`by ${provenance.analyzed_by}`);
  if (provenance.analyzed_at) {
    detailBits.push(new Date(provenance.analyzed_at).toLocaleString());
  }
  if (provenance.kb_entry_id) detailBits.push(provenance.kb_entry_id);

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
      <span
        style={{
          display: "inline-flex", alignItems: "center", gap: 5,
          fontSize: 10, fontWeight: 700, letterSpacing: "0.07em",
          textTransform: "uppercase", color: style.accent,
          border: `1px solid ${style.accent}`, borderRadius: 4, padding: "2px 7px",
        }}
      >
        <Icon size={11} />
        {style.label}
      </span>

      {/* Confidence is shown ONLY when something actually analysed the cause. */}
      {provenance.rca_confidence != null && (
        <span style={{ fontSize: 11, color: "var(--t2)", fontVariantNumeric: "tabular-nums" }}>
          {provenance.rca_confidence}% confident in cause
        </span>
      )}

      {detailBits.length > 0 && (
        <span style={{ fontSize: 10.5, color: "var(--t3, #6b7482)" }}>
          {detailBits.join(" · ")}
        </span>
      )}
    </div>
  );
}

/** What will be sent to the model, so the user can see it before it leaves. */
function EvidencePreview({ detail, explanation }) {
  const [open, setOpen] = useState(false);

  const counts = [
    ["alerts", detail?.event_count || 0],
    ["log lines", explanation?.context_logs?.length || detail?.context_logs?.length || 0],
    ["impacted services", (detail?.impacted_services || []).filter(Boolean).length],
    ["linked change", detail?.what_changed_type ? 1 : 0],
  ].filter(([, n]) => n > 0);

  return (
    <div style={{ marginTop: 10 }}>
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          display: "inline-flex", alignItems: "center", gap: 4,
          background: "none", border: "none", padding: 0, cursor: "pointer",
          fontSize: 11, color: "var(--t2)", fontFamily: "inherit",
        }}
      >
        <ChevronRight
          size={12}
          style={{ transform: open ? "rotate(90deg)" : "none", transition: "transform 0.15s" }}
        />
        What gets sent to the model
      </button>

      {open && (
        <div
          style={{
            marginTop: 8, padding: "10px 12px", borderRadius: 7,
            background: "rgba(0,0,0,0.16)", border: "1px solid var(--line, rgba(255,255,255,0.08))",
            fontSize: 11.5, color: "var(--t2)", lineHeight: 1.7,
          }}
        >
          <div style={{ marginBottom: 6 }}>
            {counts.length > 0
              ? counts.map(([label, n]) => `${n} ${label}`).join(", ")
              : "Incident metadata only"}
            {" — plus the alert timeline and incident title."}
          </div>
          <div style={{ fontSize: 10.5, color: "var(--t3, #6b7482)" }}>
            Email addresses, IP addresses and bearer tokens are masked before the
            request leaves this server. Set <code>LLM_DATA_MODE=private</code> to
            route this to a self-hosted model instead, or <code>offline</code> to
            disable AI analysis entirely.
          </div>
        </div>
      )}
    </div>
  );
}

export default function AnalysisPanel({ incidentId, detail, explanation, provenance, onAnalyzed }) {
  const [state, setState] = useState("idle"); // idle | running | error
  const [error, setError] = useState(null);

  const prov = provenance || { source: "observed", has_causal_claim: false };
  const style = SOURCE_STYLES[prov.source] || SOURCE_STYLES.observed;

  async function runAnalysis() {
    if (state === "running") return; // debounce — one call in flight at a time
    setState("running");
    setError(null);
    try {
      const result = await analyzeIncident(incidentId);
      onAnalyzed?.(result);
      setState("idle");
    } catch (e) {
      setError(e.message || "Analysis failed");
      setState("error");
    }
  }

  // ── Tier 1 & 2: a real causal claim exists ────────────────────────────────
  if (prov.has_causal_claim) {
    return (
      <div style={{ background: style.bg, border: `1px solid ${style.border}`, borderRadius: 10, padding: "14px 16px" }}>
        <ProvenanceBadge provenance={prov} />

        {explanation?.root_cause && (
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: style.accent, fontWeight: 600, marginBottom: 5, textTransform: "uppercase", letterSpacing: "0.06em" }}>
              Root Cause
            </div>
            <div style={{ fontSize: 13, color: "var(--t1)", lineHeight: 1.75 }}>{explanation.root_cause}</div>
          </div>
        )}

        {explanation?.narrative && explanation.narrative !== explanation.root_cause && (
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, color: style.accent, fontWeight: 600, marginBottom: 5, textTransform: "uppercase", letterSpacing: "0.06em" }}>
              Analysis
            </div>
            <div style={{ fontSize: 12, color: "var(--t2)", lineHeight: 1.75 }}>{explanation.narrative}</div>
          </div>
        )}

        {/* Re-analysis is always available: more alerts may have merged in since. */}
        <button
          onClick={runAnalysis}
          disabled={state === "running"}
          style={{
            display: "inline-flex", alignItems: "center", gap: 6, marginTop: 4,
            padding: "5px 11px", borderRadius: 6, cursor: state === "running" ? "wait" : "pointer",
            background: "transparent", border: "1px solid var(--line, rgba(255,255,255,0.14))",
            color: "var(--t2)", fontSize: 11.5, fontWeight: 600, fontFamily: "inherit",
          }}
        >
          {state === "running"
            ? <><Loader2 size={12} className="spin" /> Re-analysing…</>
            : <><RefreshCw size={12} /> Re-analyse</>}
        </button>
        {error && (
          <div style={{ marginTop: 8, fontSize: 11.5, color: "var(--red, #ff5c5c)" }}>{error}</div>
        )}
      </div>
    );
  }

  // ── Tier 3: observation only. State the facts, offer the analysis. ────────
  return (
    <div style={{ background: style.bg, border: `1px solid ${style.border}`, borderRadius: 10, padding: "14px 16px" }}>
      <ProvenanceBadge provenance={prov} />

      {explanation?.narrative && (
        <div style={{ fontSize: 12.5, color: "var(--t1)", lineHeight: 1.75, marginBottom: 10 }}>
          {explanation.narrative}
        </div>
      )}

      {prov.unavailable && (
        <div
          style={{
            display: "flex", gap: 7, alignItems: "flex-start",
            fontSize: 11.5, color: "var(--t2)", lineHeight: 1.6, marginBottom: 12,
          }}
        >
          <AlertTriangle size={13} style={{ flexShrink: 0, marginTop: 2 }} />
          <span>{prov.unavailable}</span>
        </div>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <button
          onClick={runAnalysis}
          disabled={state === "running"}
          style={{
            display: "inline-flex", alignItems: "center", gap: 7,
            padding: "7px 14px", borderRadius: 7, cursor: state === "running" ? "wait" : "pointer",
            background: state === "running" ? "rgba(0,102,255,0.35)" : "var(--blue, #0066ff)",
            border: "none", color: "#fff", fontSize: 12, fontWeight: 600, fontFamily: "inherit",
          }}
        >
          {state === "running"
            ? <><Loader2 size={13} className="spin" /> Analysing… (up to 30s)</>
            : <><Zap size={13} /> Analyse with AI</>}
        </button>

        {state !== "running" && (
          <span style={{ fontSize: 11, color: "var(--t3, #6b7482)" }}>
            Sends this incident's evidence to the configured model
          </span>
        )}
      </div>

      {error && (
        <div style={{ marginTop: 10, fontSize: 11.5, color: "var(--red, #ff5c5c)", lineHeight: 1.6 }}>
          {error} — the evidence above is unaffected.
        </div>
      )}

      <EvidencePreview detail={detail} explanation={explanation} />
    </div>
  );
}
