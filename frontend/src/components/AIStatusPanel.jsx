/**
 * AI Engine — provider, data mode, and usage.
 *
 * The page answers one question first: is AI actually working, and where is my
 * data going? Configuration state and data mode lead; usage counters follow;
 * the deployment matrix sits last as reference material rather than competing
 * with live status for attention.
 */
import { useEffect, useState } from "react";
import { AlertTriangle, Brain, Cloud, Server, ShieldOff } from "lucide-react";

import { getAIStatus } from "../api/phase3.js";
import {
  EmptyState, ErrorState, Grid, Page, PageHeader, Panel,
  SkeletonRows, StatTile, StatusPulse,
} from "./ui/Primitives.jsx";

/* Deployment modes. Tone carries the privacy posture: cloud leaves your
   network (brand), private stays inside it (success), offline sends nothing
   at all (warning — capability is reduced, which the operator should know). */
const MODE_INFO = {
  cloud: {
    title: "Cloud mode",
    icon: Cloud,
    tone: "tone-brand",
    description:
      "AI calls are sent to the OpenAI or Anthropic public API. PII (emails, IPs, tokens) is scrubbed from prompts before transmission.",
    env: "LLM_DATA_MODE=cloud (default) — requires LLM_API_KEY",
  },
  private: {
    title: "Private mode — BYOC",
    icon: Server,
    tone: "tone-success",
    description:
      "AI calls are routed to your self-hosted LLM_BASE_URL endpoint (Ollama, vLLM, LM Studio, or any OpenAI-compatible server). Data never leaves your infrastructure.",
    env: "LLM_DATA_MODE=private and LLM_BASE_URL=http://your-llm:11434",
  },
  offline: {
    title: "Offline mode",
    icon: ShieldOff,
    tone: "tone-warning",
    description:
      "All LLM calls are disabled. The platform uses rule-based templates for explanations and RCA. Zero data transmission — suitable for air-gapped environments.",
    env: "LLM_DATA_MODE=offline — no API key required",
  },
};

const PROVIDER_LABELS = {
  openai: "OpenAI",
  anthropic: "Anthropic (Claude)",
  local: "Local / BYOC endpoint",
};

function ModeRow({ info, isCurrent }) {
  const Icon = info.icon;
  return (
    <li className={`mode-row ${info.tone}${isCurrent ? " is-current" : ""}`}>
      <span className="mode-row-icon" aria-hidden="true">
        {Icon && <Icon size={15} />}
      </span>
      <div className="mode-row-body">
        <h3 className="mode-row-title">
          {info.title}
          {isCurrent && <span className="pill is-plain mode-row-active">active</span>}
        </h3>
        <p className="mode-row-desc">{info.description}</p>
        <code className="mode-row-env">{info.env}</code>
      </div>
    </li>
  );
}

export default function AIStatusPanel() {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      setStatus(await getAIStatus());
    } catch (err) {
      setError(err.message || "Failed to fetch AI status");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  const mode = status?.data_mode || "cloud";
  const modeInfo = MODE_INFO[mode] || MODE_INFO.cloud;
  const provider = status?.provider || "openai";
  const providerLabel = PROVIDER_LABELS[provider] || provider;
  const configured = Boolean(status?.configured);
  const totalCalls = status?.total_calls || 0;
  const failedCalls = status?.failed_calls || 0;
  const failedPct = totalCalls > 0 ? ((failedCalls / totalCalls) * 100).toFixed(1) : "0";

  return (
    <Page>
      <PageHeader
        eyebrow={
          <StatusPulse
            tone={configured ? "success" : "danger"}
            label={configured ? "Configured" : "Not configured"}
            detail={configured ? providerLabel : "falling back to templates"}
          />
        }
        title="AI Engine"
        meta="Provider, deployment mode and usage"
      />

      {error ? (
        <ErrorState message={error} onRetry={load} />
      ) : loading ? (
        <Grid cols={3}>
          <SkeletonRows count={1} height={104} />
          <SkeletonRows count={1} height={104} />
          <SkeletonRows count={1} height={104} />
        </Grid>
      ) : !status ? (
        <EmptyState icon={Brain} title="No AI status available" message="The engine has not reported in yet." />
      ) : (
        <>
          {/* Usage at a glance. */}
          <Grid cols={3}>
            <StatTile
              label="Provider" icon={Brain} tone={configured ? "brand" : "neutral"}
              value={providerLabel} sub={configured ? `${mode} mode` : "not configured"}
            />
            <StatTile
              label="Total calls" icon={Cloud} tone="info"
              value={totalCalls.toLocaleString()} sub="since start"
            />
            <StatTile
              label="Failed calls" icon={AlertTriangle}
              tone={failedCalls > 0 ? "danger" : "success"}
              value={failedCalls.toLocaleString()} sub={`${failedPct}% failure rate`}
            />
          </Grid>

          {!configured && (
            <div className="callout tone-warning callout-block">
              <AlertTriangle size={13} className="callout-icon" />
              <div>
                <p className="callout-title">AI provider is not configured</p>
                <p className="callout-text">
                  Copilot and explain endpoints fall back to rule-based templates. Set{" "}
                  <code className="code-inline">LLM_API_KEY</code> for cloud mode, or{" "}
                  <code className="code-inline">LLM_BASE_URL</code> for a local model, in your{" "}
                  <code className="code-inline">.env</code>.
                </p>
              </div>
            </div>
          )}

          <Grid cols={2}>
            <Panel title="Current configuration" sub="what is live right now">
              <dl className="attr-grid">
                <div className="attr">
                  <dt className="attr-label">Provider</dt>
                  <dd className="attr-value">{providerLabel}</dd>
                </div>
                <div className="attr">
                  <dt className="attr-label">Data mode</dt>
                  <dd className="attr-value">{modeInfo.title}</dd>
                </div>
                <div className="attr">
                  <dt className="attr-label">Status</dt>
                  <dd className="attr-value">{configured ? "Configured" : "Not configured"}</dd>
                </div>
                <div className="attr">
                  <dt className="attr-label">Last call</dt>
                  <dd className="attr-value">
                    {status.last_call_at ? new Date(status.last_call_at).toLocaleString() : "—"}
                  </dd>
                </div>
              </dl>
              <div className={`callout ${modeInfo.tone} callout-block`}>
                <modeInfo.icon size={13} className="callout-icon" />
                <div>
                  <p className="callout-title">{modeInfo.title}</p>
                  <p className="callout-text">{modeInfo.description}</p>
                </div>
              </div>
            </Panel>

            <Panel title="Deployment matrix" sub="how to switch modes" flush>
              <ul className="mode-list">
                {Object.entries(MODE_INFO).map(([key, info]) => (
                  <ModeRow key={key} info={info} isCurrent={key === mode} />
                ))}
              </ul>
            </Panel>
          </Grid>
        </>
      )}
    </Page>
  );
}
