import { useState, useEffect, useRef } from "react";
import {
  getWizardConfig,
  sendTestAlert,
  advanceStep,
  getProgress,
} from "../api/onboarding.js";

const STEPS = [
  {
    key: "signup",
    label: "Account Created",
    icon: "✓",
    desc: "Your NeuroOps account and tenant are provisioned.",
  },
  {
    key: "connect_source",
    label: "Connect Your First Data Source",
    icon: "🔌",
    desc: "Copy a webhook URL into your monitoring tool — Prometheus, PagerDuty, GitHub, or any generic source.",
  },
  {
    key: "first_alert",
    label: "Send a Test Alert",
    icon: "🔔",
    desc: "Fire one alert to confirm the pipeline is wired up.",
  },
  {
    key: "first_incident",
    label: "See Your First Incident",
    icon: "⚡",
    desc: "NeuroOps correlates incoming alerts and surfaces the first incident automatically.",
  },
  {
    key: "install_agent",
    label: "Install NeuroOps Agent",
    icon: "🤖",
    desc: "Run the one-line installer on any host — the agent streams logs, metrics, and topology directly into the platform.",
  },
  {
    key: "complete",
    label: "Onboarding Complete",
    icon: "🎉",
    desc: "Your AIOps platform is fully operational.",
  },
];

const DISPLAY_STEPS = STEPS.filter((s) => s.key !== "complete");

function CopyField({ label, value }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div className="ob-copy-field">
      <div className="ob-copy-label">{label}</div>
      <div className="ob-copy-row">
        <code className="ob-copy-url">{value}</code>
        <button className="lux-secondary-btn small" onClick={copy} style={{ minWidth: 65, flexShrink: 0 }}>
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>
    </div>
  );
}

function ProgressBadge({ completedCount, total }) {
  const pct = Math.round((completedCount / total) * 100);
  return (
    <div className="ob-progress">
      <div className="ob-progress-bar">
        <div className="ob-progress-fill" style={{ width: `${Math.max(4, pct)}%` }} />
      </div>
      <div className="ob-progress-label">{completedCount} of {total} steps complete</div>
    </div>
  );
}

export default function OnboardingWizard({ onComplete }) {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(true);
  const [testSending, setTestSending] = useState(false);
  const [testSent, setTestSent] = useState(false);
  const [testError, setTestError] = useState("");
  const pollRef = useRef(null);

  async function load() {
    try {
      const cfg = await getWizardConfig();
      setConfig(cfg);
      return cfg;
    } catch {
      setConfig(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, []);

  // Poll every 4 s while waiting for the first incident to appear
  useEffect(() => {
    if (!config) return;
    const step = config?.progress?.step;
    if (step !== "first_alert" && step !== "first_incident") return;

    pollRef.current = setInterval(async () => {
      try {
        const p = await getProgress();
        if (p?.first_incident_created || p?.step === "first_incident" || p?.step === "install_agent") {
          clearInterval(pollRef.current);
          setConfig((prev) => prev ? { ...prev, progress: p } : prev);
        }
      } catch { /* ignore poll errors */ }
    }, 4000);

    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, [config]); // config covers config?.progress?.step

  async function handleAdvance(step) {
    try {
      await advanceStep(step);
      await load();
      if (step === "complete" && onComplete) onComplete();
    } catch { /* non-critical; progress reloads on retry */ }
  }

  async function handleSendTest() {
    setTestSending(true);
    setTestError("");
    try {
      await sendTestAlert();
      setTestSent(true);
      // Optimistically advance; the poll loop will catch the real incident
      await handleAdvance("first_alert");
    } catch (err) {
      setTestError(err?.message || "Failed to send test alert — is the backend reachable?");
    } finally {
      setTestSending(false);
    }
  }

  const progress = config?.progress;
  const completedSet = new Set(progress?.completed_steps || []);
  const currentIdx = progress ? STEPS.findIndex((s) => s.key === progress.step) : 0;
  const completedCount = DISPLAY_STEPS.filter((s) => completedSet.has(s.key) || STEPS.indexOf(s) < currentIdx).length;

  if (loading) {
    return <div className="lux-muted" style={{ padding: "2rem" }}>Loading setup guide…</div>;
  }

  if (progress?.aha_moment_reached || progress?.step === "complete") {
    return (
      <div className="ob-complete">
        <div style={{ fontSize: "3rem" }}>🎉</div>
        <h2>You're all set!</h2>
        <p className="lux-muted">
          NeuroOps is fully operational. Switch to Incidents to see AI-powered correlation in action.
        </p>
        <button className="lux-primary-btn" onClick={onComplete}>Go to Dashboard</button>
      </div>
    );
  }

  return (
    <div className="ob-root">
      <div className="ob-header">
        <div className="lux-eyebrow">SETUP GUIDE</div>
        <h2 style={{ margin: "0.25rem 0 0.5rem" }}>Get to your first AI incident in 10 minutes</h2>
        <p className="lux-muted" style={{ margin: 0 }}>
          Connect a data source, fire one alert, and watch NeuroOps build your first incident automatically.
        </p>
      </div>

      <ProgressBadge completedCount={completedCount} total={DISPLAY_STEPS.length} />

      <div className="ob-steps">
        {DISPLAY_STEPS.map((step, _i) => {
          const stepIdx = STEPS.indexOf(step);
          const done = completedSet.has(step.key) || stepIdx < currentIdx;
          const active = stepIdx === currentIdx;
          return (
            <div key={step.key} className={`ob-step ${done ? "done" : ""} ${active ? "active" : ""}`}>
              <div className="ob-step-icon">{done ? "✅" : step.icon}</div>
              <div className="ob-step-content">
                <div className="ob-step-label">{step.label}</div>
                <div className="ob-step-desc">{step.desc}</div>

                {/* ── Step actions ── */}

                {active && step.key === "signup" && (
                  <div className="ob-step-action">
                    <button className="lux-primary-btn small" onClick={() => handleAdvance("connect_source")}>
                      Continue →
                    </button>
                  </div>
                )}

                {active && step.key === "connect_source" && (
                  <div className="ob-step-action">
                    <CopyField label="Generic Webhook" value={config?.webhook_url || `${window.location.origin}/api/v1/ingest/webhook`} />
                    <CopyField label="Prometheus AlertManager" value={config?.prometheus_url || `${window.location.origin}/api/v1/ingest/prometheus`} />
                    <CopyField label="GitHub Webhooks" value={config?.github_url || `${window.location.origin}/api/v1/ingest/github`} />
                    <div style={{ marginTop: "0.5rem", display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
                      <button className="lux-primary-btn small" onClick={() => handleAdvance("first_alert")}>
                        I've connected a source
                      </button>
                      <button
                        className="lux-secondary-btn small"
                        onClick={handleSendTest}
                        disabled={testSending}
                      >
                        {testSending ? "Sending…" : "Skip: send test alert instead"}
                      </button>
                    </div>
                    {testError && <div style={{ color: "#f87171", fontSize: "0.78rem" }}>{testError}</div>}
                  </div>
                )}

                {active && step.key === "first_alert" && (
                  <div className="ob-step-action">
                    <p style={{ fontSize: "0.82rem", margin: 0 }}>
                      Your source is connected. Send one alert to confirm end-to-end delivery.
                    </p>
                    <button
                      className="lux-primary-btn small"
                      onClick={handleSendTest}
                      disabled={testSending || testSent}
                      style={{ width: "fit-content" }}
                    >
                      {testSending ? "Sending…" : testSent ? "Test sent ✓" : "Send Test Alert (1-click)"}
                    </button>
                    {testError && <div style={{ color: "#f87171", fontSize: "0.78rem" }}>{testError}</div>}
                  </div>
                )}

                {active && step.key === "first_incident" && (
                  <div className="ob-step-action">
                    <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", fontSize: "0.82rem" }}>
                      <span style={{
                        display: "inline-block", width: 8, height: 8, borderRadius: "50%",
                        background: "#10b981", animation: "pulse 1.5s infinite",
                      }} />
                      Watching for your first incident… (auto-detects in a few seconds)
                    </div>
                    <button className="lux-secondary-btn small" style={{ marginTop: "0.25rem" }}
                      onClick={() => handleAdvance("install_agent")}>
                      I can see the incident — continue
                    </button>
                  </div>
                )}

                {active && step.key === "install_agent" && (
                  <div className="ob-step-action">
                    <p style={{ fontSize: "0.82rem", margin: "0 0 0.5rem" }}>
                      Run this one-liner on any Linux/macOS host to stream logs and metrics directly into NeuroOps:
                    </p>
                    <CopyField
                      label="Agent Installer (bash)"
                      value={config?.agent_install_cmd || `curl -fsSL ${window.location.origin}/install.sh | TENANT_ID=${config?.tenant_id || "your-tenant"} SERVER_URL=${window.location.origin} sh`}
                    />
                    <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
                      <button className="lux-primary-btn small" onClick={() => handleAdvance("complete")}>
                        Agent installed — finish setup
                      </button>
                      <button className="lux-secondary-btn small" onClick={() => handleAdvance("complete")}>
                        Skip for now
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>

      <div className="ob-skip">
        <button className="lux-secondary-btn small" onClick={() => handleAdvance("complete")}>
          Skip setup guide
        </button>
      </div>
    </div>
  );
}
