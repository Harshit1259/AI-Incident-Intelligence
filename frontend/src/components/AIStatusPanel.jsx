import { useEffect, useState } from "react";
import { getAIStatus } from "../api/phase3.js";

const MODE_INFO = {
  cloud: {
    title: "Cloud Mode",
    description:
      "AI calls are sent to the OpenAI or Anthropic public API. PII (emails, IPs, tokens) is automatically scrubbed from prompts before transmission.",
    color: "#3aa7ff",
    bg: "rgba(58,167,255,0.08)",
    border: "rgba(58,167,255,0.24)",
  },
  private: {
    title: "Private Mode — BYOC",
    description:
      "AI calls are routed to your self-hosted LLM_BASE_URL endpoint (Ollama, vLLM, LM Studio, or any OpenAI-compatible server). Data never leaves your infrastructure.",
    color: "#34d399",
    bg: "rgba(52,211,153,0.08)",
    border: "rgba(52,211,153,0.24)",
  },
  offline: {
    title: "Offline Mode",
    description:
      "All LLM calls are disabled. The platform uses rule-based templates for explanations and RCA. Zero data transmission — suitable for fully air-gapped environments.",
    color: "#f59e0b",
    bg: "rgba(245,158,11,0.08)",
    border: "rgba(245,158,11,0.24)",
  },
};

const PROVIDER_LABELS = {
  openai: "OpenAI",
  anthropic: "Anthropic (Claude)",
  local: "Local / BYOC Endpoint",
};
const PROVIDER_COLORS = {
  openai: "#10b981",
  anthropic: "#a78bfa",
  local: "#34d399",
};

function StatCard({ label, value, sub, highlight }) {
  return (
    <div style={{
      padding: "14px 16px", borderRadius: 12,
      background: "rgba(255,255,255,0.04)", border: "1px solid rgba(90,123,186,0.14)",
    }}>
      <div style={{ fontSize: "0.67rem", color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.1em", marginBottom: 5 }}>
        {label}
      </div>
      <div style={{ fontSize: "1.6rem", fontWeight: 800, lineHeight: 1, color: highlight || "#edf4ff" }}>
        {value}
      </div>
      {sub && <div style={{ fontSize: "0.69rem", color: "#64748b", marginTop: 3 }}>{sub}</div>}
    </div>
  );
}

function DeployRow({ modeKey, info, isCurrent }) {
  return (
    <div style={{
      display: "flex", alignItems: "flex-start", gap: 14,
      padding: "14px 16px", borderRadius: 12,
      background: isCurrent ? info.bg : "rgba(255,255,255,0.03)",
      border: `1px solid ${isCurrent ? info.border : "rgba(90,123,186,0.10)"}`,
    }}>
      <div style={{
        width: 10, height: 10, borderRadius: "50%", flexShrink: 0, marginTop: 4,
        background: isCurrent ? info.color : "#374151",
      }} />
      <div>
        <div style={{
          fontWeight: 700, fontSize: "0.85rem", marginBottom: 4,
          color: isCurrent ? info.color : "#9eb5da",
        }}>
          {info.title}
          {isCurrent && (
            <span style={{
              marginLeft: 8, fontSize: "0.65rem", fontWeight: 600, padding: "2px 8px",
              borderRadius: 20, background: info.border, color: info.color,
            }}>
              ← active
            </span>
          )}
        </div>
        <div style={{ fontSize: "0.78rem", color: "#64748b", lineHeight: 1.55 }}>
          {info.description}
        </div>
        <div style={{ marginTop: 6, fontSize: "0.7rem", color: "#4b5563" }}>
          {modeKey === "cloud" && "Set LLM_DATA_MODE=cloud (default) — requires LLM_API_KEY"}
          {modeKey === "private" && "Set LLM_DATA_MODE=private and LLM_BASE_URL=http://your-llm:11434"}
          {modeKey === "offline" && "Set LLM_DATA_MODE=offline — no API key required"}
        </div>
      </div>
    </div>
  );
}

export default function AIStatusPanel() {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    try { setStatus(await getAIStatus()); }
    catch (err) { setError(err.message || "Failed to fetch AI status"); }
    finally { setLoading(false); }
  }

  useEffect(() => { load(); }, []);

  if (loading) return <div className="lux-muted" style={{ padding: "1.5rem" }}>Fetching AI status…</div>;
  if (error) return <div style={{ color: "#fca5a5", padding: "1rem" }}>{error}</div>;
  if (!status) return <div className="lux-muted">No AI status data.</div>;

  const mode = status.data_mode || "cloud";
  const provider = status.provider || "openai";
  const modeInfo = MODE_INFO[mode] || MODE_INFO.cloud;
  const providerLabel = PROVIDER_LABELS[provider] || provider;
  const providerColor = PROVIDER_COLORS[provider] || "#9eb5da";
  const failedPct = status.total_calls > 0 ? ((status.failed_calls / status.total_calls) * 100).toFixed(1) : "0";

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">AI DEPLOYMENT</div>
          <h3>AI Provider &amp; Deployment Configuration</h3>
        </div>
        <div style={{
          padding: "6px 16px", borderRadius: 20,
          background: status.configured ? "rgba(52,211,153,0.10)" : "rgba(248,113,113,0.10)",
          border: `1px solid ${status.configured ? "rgba(52,211,153,0.24)" : "rgba(248,113,113,0.24)"}`,
          color: status.configured ? "#6ee7b7" : "#fca5a5",
          fontSize: "0.8rem", fontWeight: 700,
        }}>
          {status.configured ? "Configured" : "Not Configured"}
        </div>
      </div>

      {/* Provider + Mode summary */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, marginBottom: 20 }}>
        <div style={{
          padding: "16px", borderRadius: 12,
          background: "rgba(255,255,255,0.04)", border: "1px solid rgba(90,123,186,0.14)",
        }}>
          <div style={{ fontSize: "0.67rem", color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.1em", marginBottom: 8 }}>
            Provider
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <div style={{ width: 10, height: 10, borderRadius: "50%", background: providerColor, flexShrink: 0 }} />
            <span style={{ fontWeight: 700, fontSize: "0.95rem", color: providerColor }}>{providerLabel}</span>
          </div>
        </div>

        <div style={{
          padding: "16px", borderRadius: 12,
          background: modeInfo.bg, border: `1px solid ${modeInfo.border}`,
        }}>
          <div style={{ fontSize: "0.67rem", color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.1em", marginBottom: 8 }}>
            Data Mode
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <div style={{ width: 10, height: 10, borderRadius: "50%", background: modeInfo.color, flexShrink: 0 }} />
            <span style={{ fontWeight: 700, fontSize: "0.95rem", color: modeInfo.color }}>{modeInfo.title}</span>
          </div>
        </div>
      </div>

      {/* Call stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))", gap: 12, marginBottom: 24 }}>
        <StatCard label="Total Calls" value={status.total_calls || 0} />
        <StatCard
          label="Failed Calls"
          value={status.failed_calls || 0}
          sub={`${failedPct}% failure rate`}
          highlight={(status.failed_calls || 0) > 0 ? "#f87171" : "#6ee7b7"}
        />
        {status.last_call_at && (
          <StatCard
            label="Last Call"
            value={new Date(status.last_call_at).toLocaleTimeString()}
            sub={new Date(status.last_call_at).toLocaleDateString()}
          />
        )}
      </div>

      {/* Not-configured warning */}
      {!status.configured && (
        <div style={{
          padding: "14px 16px", borderRadius: 12, marginBottom: 24,
          background: "rgba(245,158,11,0.07)", border: "1px solid rgba(245,158,11,0.22)",
          fontSize: "0.82rem", color: "#fcd34d", lineHeight: 1.65,
        }}>
          <strong>AI provider is not configured.</strong> Copilot and explain endpoints fall back to rule-based templates.
          <br />
          Set <code style={{ background: "rgba(0,0,0,0.3)", padding: "1px 5px", borderRadius: 4 }}>LLM_API_KEY</code> for cloud mode,
          or <code style={{ background: "rgba(0,0,0,0.3)", padding: "1px 5px", borderRadius: 4 }}>LLM_BASE_URL</code> for a local model in your <code style={{ background: "rgba(0,0,0,0.3)", padding: "1px 5px", borderRadius: 4 }}>.env</code>.
        </div>
      )}

      {/* Deployment matrix */}
      <div className="lux-eyebrow" style={{ marginBottom: 12 }}>DEPLOYMENT MATRIX</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {Object.entries(MODE_INFO).map(([key, info]) => (
          <DeployRow key={key} modeKey={key} info={info} isCurrent={key === mode} />
        ))}
      </div>
    </div>
  );
}
