/**
 * Setup guide — from empty account to first AI-correlated incident.
 *
 * A vertical stepper where only the active step is expanded. The previous
 * version rendered every step's description at equal weight with emoji
 * markers, so nothing signalled what to do next. Now: done steps collapse to
 * a check, the active step carries the whole action area, and future steps are
 * dimmed previews.
 */
import { useState, useEffect, useRef } from "react";
import {
  Bot, Check, Copy, Bell, Plug, Sparkles, UserCheck, Zap,
} from "lucide-react";

import {
  getWizardConfig, sendTestAlert, advanceStep, getProgress,
} from "../api/onboarding.js";
import { EmptyState, Page, PageHeader, Panel, SkeletonRows } from "./ui/Primitives.jsx";

const STEPS = [
  {
    key: "signup",
    label: "Account created",
    icon: UserCheck,
    desc: "Your NeuroOps account and tenant are provisioned.",
  },
  {
    key: "connect_source",
    label: "Connect your first data source",
    icon: Plug,
    desc: "Copy a webhook URL into your monitoring tool — Prometheus, PagerDuty, GitHub, or any generic source.",
  },
  {
    key: "first_alert",
    label: "Send a test alert",
    icon: Bell,
    desc: "Fire one alert to confirm the pipeline is wired up end to end.",
  },
  {
    key: "first_incident",
    label: "See your first incident",
    icon: Zap,
    desc: "NeuroOps correlates incoming alerts and surfaces the first incident automatically.",
  },
  {
    key: "install_agent",
    label: "Install the NeuroOps agent",
    icon: Bot,
    desc: "One line on any host streams logs, metrics and topology into the platform.",
  },
  { key: "complete", label: "Onboarding complete", icon: Sparkles, desc: "" },
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
    <div className="copy-field">
      <span className="copy-field-label">{label}</span>
      <div className="copy-field-row">
        <code className="copy-field-value">{value}</code>
        <button type="button" className="btn btn-outline btn-xs copy-btn" onClick={copy}>
          {copied ? <Check size={11} /> : <Copy size={11} />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
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
    return null;
  }

  useEffect(() => {
    load();
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  // Poll while waiting for the first incident to appear.
  useEffect(() => {
    if (!config) return undefined;
    const step = config?.progress?.step;
    if (step !== "first_alert" && step !== "first_incident") return undefined;

    pollRef.current = setInterval(async () => {
      try {
        const p = await getProgress();
        if (p?.first_incident_created || p?.step === "first_incident" || p?.step === "install_agent") {
          clearInterval(pollRef.current);
          setConfig((prev) => (prev ? { ...prev, progress: p } : prev));
        }
      } catch {
        /* transient poll failures are expected while the pipeline warms up */
      }
    }, 4000);

    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [config]);

  async function handleAdvance(step) {
    try {
      await advanceStep(step);
      await load();
      if (step === "complete" && onComplete) onComplete();
    } catch {
      /* non-critical; progress reloads on the next interaction */
    }
  }

  async function handleSendTest() {
    setTestSending(true);
    setTestError("");
    try {
      await sendTestAlert();
      setTestSent(true);
      await handleAdvance("first_alert");
    } catch (err) {
      setTestError(err?.message || "Could not send the test alert — is the backend reachable?");
    } finally {
      setTestSending(false);
    }
  }

  const progress = config?.progress;
  const completedSet = new Set(progress?.completed_steps || []);
  const currentIdx = progress ? STEPS.findIndex((s) => s.key === progress.step) : 0;
  const completedCount = DISPLAY_STEPS.filter(
    (s) => completedSet.has(s.key) || STEPS.indexOf(s) < currentIdx,
  ).length;
  const pct = Math.round((completedCount / DISPLAY_STEPS.length) * 100);

  if (loading) {
    return (
      <Page>
        <SkeletonRows count={5} height={72} />
      </Page>
    );
  }

  if (progress?.aha_moment_reached || progress?.step === "complete") {
    return (
      <Page>
        <EmptyState
          icon={Sparkles}
          tone="success"
          title="You’re all set"
          message="NeuroOps is fully operational. Open Incidents to see AI-powered correlation in action."
          action="Go to dashboard"
          onAction={onComplete}
        />
      </Page>
    );
  }

  const origin = window.location.origin;

  return (
    <Page>
      <PageHeader
        title="Get to your first AI incident"
        meta="Connect a source, fire one alert, and watch NeuroOps build the incident automatically"
        actions={
          <button type="button" className="btn btn-ghost" onClick={() => handleAdvance("complete")}>
            Skip setup
          </button>
        }
      />

      <div className="wizard-progress">
        <div className="wizard-progress-track">
          <div className="wizard-progress-fill" style={{ width: `${Math.max(4, pct)}%` }} />
        </div>
        <span className="wizard-progress-label">
          {completedCount} of {DISPLAY_STEPS.length} steps complete
        </span>
      </div>

      <Panel flush>
        <ol className="wizard-steps">
          {DISPLAY_STEPS.map((step) => {
            const stepIdx = STEPS.indexOf(step);
            const done = completedSet.has(step.key) || stepIdx < currentIdx;
            const active = stepIdx === currentIdx;
            const Icon = step.icon;

            return (
              <li
                key={step.key}
                className={`wizard-step${done ? " is-done" : ""}${active ? " is-active" : ""}`}
                aria-current={active ? "step" : undefined}
              >
                <span className="wizard-step-marker" aria-hidden="true">
                  {done ? <Check size={13} /> : Icon && <Icon size={13} />}
                </span>

                <div className="wizard-step-body">
                  <h3 className="wizard-step-label">{step.label}</h3>
                  <p className="wizard-step-desc">{step.desc}</p>

                  {active && step.key === "signup" && (
                    <div className="wizard-actions">
                      <button type="button" className="btn btn-primary btn-sm" onClick={() => handleAdvance("connect_source")}>
                        Continue
                      </button>
                    </div>
                  )}

                  {active && step.key === "connect_source" && (
                    <div className="wizard-actions">
                      <CopyField label="Prometheus Alertmanager / Grafana Alerting" value={config?.prometheus_url || `${origin}/api/v1/ingest/prometheus`} />
                      <CopyField label="OpenTelemetry (OTLP/HTTP, add /logs, /metrics or /traces)" value={config?.otel_url || `${origin}/api/v1/otel`} />
                      <div className="wizard-button-row">
                        <button type="button" className="btn btn-primary btn-sm" onClick={() => handleAdvance("first_alert")}>
                          I’ve connected a source
                        </button>
                        <button type="button" className="btn btn-ghost btn-sm" onClick={handleSendTest} disabled={testSending}>
                          {testSending ? "Sending…" : "Send a test alert instead"}
                        </button>
                      </div>
                      {testError && <p className="form-error">{testError}</p>}
                    </div>
                  )}

                  {active && step.key === "first_alert" && (
                    <div className="wizard-actions">
                      <button
                        type="button" className="btn btn-primary btn-sm"
                        onClick={handleSendTest} disabled={testSending || testSent}
                      >
                        {testSending ? "Sending…" : testSent ? "Test sent" : "Send test alert"}
                      </button>
                      {testError && <p className="form-error">{testError}</p>}
                    </div>
                  )}

                  {active && step.key === "first_incident" && (
                    <div className="wizard-actions">
                      <span className="stream-badge tone-success">
                        <span className="stream-dot" aria-hidden="true" />
                        Watching for your first incident…
                      </span>
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => handleAdvance("install_agent")}>
                        I can see it — continue
                      </button>
                    </div>
                  )}

                  {active && step.key === "install_agent" && (
                    <div className="wizard-actions">
                      <CopyField
                        label="Agent installer (bash)"
                        value={
                          config?.agent_install_cmd ||
                          `curl -fsSL ${origin}/install.sh | TENANT_ID=${config?.tenant_id || "your-tenant"} SERVER_URL=${origin} sh`
                        }
                      />
                      <div className="wizard-button-row">
                        <button type="button" className="btn btn-primary btn-sm" onClick={() => handleAdvance("complete")}>
                          Agent installed — finish
                        </button>
                        <button type="button" className="btn btn-ghost btn-sm" onClick={() => handleAdvance("complete")}>
                          Skip for now
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              </li>
            );
          })}
        </ol>
      </Panel>
    </Page>
  );
}
