// IntegrationsHub.jsx — Phase 2, Week 4-5
// Shows all webhook endpoints, integration setup guides, and live status.

import { useState } from "react";
import { getWebhookURLs } from "../api/integrations.js";

function CopyButton({ value }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <button className="lux-secondary-btn small" onClick={copy} style={{ minWidth: "70px" }}>
      {copied ? "Copied!" : "Copy"}
    </button>
  );
}

function WebhookRow({ label, url, method = "POST", desc }) {
  return (
    <div className="int-row">
      <div className="int-row-left">
        <div className="int-row-label">{label}</div>
        {desc && <div className="int-row-desc">{desc}</div>}
        <div className="int-row-method">
          <span className="int-method-badge">{method}</span>
          <code className="int-url">{url}</code>
        </div>
      </div>
      <CopyButton value={url} />
    </div>
  );
}

function Section({ title, icon, children, defaultOpen = false }) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="int-section">
      <button className="int-section-head" onClick={() => setOpen(!open)}>
        <span>{icon} {title}</span>
        <span className="int-chevron">{open ? "▲" : "▼"}</span>
      </button>
      {open && <div className="int-section-body">{children}</div>}
    </div>
  );
}

export default function IntegrationsHub() {
  const urls = getWebhookURLs();

  return (
    <div className="int-root">
      <div className="int-hero">
        <div className="lux-eyebrow">PHASE 2 — INTEGRATIONS + WORKFLOW</div>
        <h2 style={{ margin: "0.25rem 0 0.5rem" }}>Connect your tools</h2>
        <p className="lux-muted" style={{ margin: 0 }}>
          Paste these webhook URLs into each tool. Incidents flow automatically — no polling, no scripts.
        </p>
      </div>

      {/* ─── Week 4: GitHub + GitLab ─── */}
      <Section title="GitHub Webhook" icon="🐙" defaultOpen={true}>
        <WebhookRow
          label="GitHub Ingest Endpoint"
          url={urls.github}
          desc="Configure in repo → Settings → Webhooks. Select: push, deployments, releases."
        />
        <div className="int-guide">
          <div className="int-guide-title">Setup steps</div>
          <ol className="int-guide-steps">
            <li>Go to your GitHub repo → <strong>Settings → Webhooks → Add webhook</strong></li>
            <li>Paste the URL above into <strong>Payload URL</strong></li>
            <li>Set Content type to <code>application/json</code></li>
            <li>Set a <strong>Secret</strong> and add <code>GITHUB_WEBHOOK_SECRET=&lt;same secret&gt;</code> to your backend env</li>
            <li>Select events: <strong>Pushes, Deployments, Releases</strong></li>
            <li>Deploys to the same service as an open incident will auto-populate the <strong>"What Changed"</strong> panel</li>
          </ol>
        </div>
      </Section>

      <Section title="GitLab Webhook" icon="🦊">
        <WebhookRow
          label="GitLab Ingest Endpoint"
          url={urls.gitlab}
          desc="Configure in project → Settings → Webhooks. Select: Push events, Deployment events, Releases."
        />
        <div className="int-guide">
          <div className="int-guide-title">Setup steps</div>
          <ol className="int-guide-steps">
            <li>Go to <strong>Settings → Webhooks</strong> in your GitLab project</li>
            <li>Paste the URL above into <strong>URL</strong></li>
            <li>Set <strong>Secret Token</strong> and add <code>GITLAB_WEBHOOK_TOKEN=&lt;token&gt;</code> to backend env</li>
            <li>Check: <strong>Push events, Deployment events, Releases events</strong></li>
            <li>Click <strong>Add webhook</strong></li>
          </ol>
        </div>
      </Section>

      {/* ─── Week 4: PagerDuty ─── */}
      <Section title="PagerDuty" icon="📟">
        <WebhookRow
          label="PagerDuty Webhook v3 Endpoint"
          url={urls.pagerduty}
          desc="Add as a generic webhook subscription in PagerDuty. Triggers auto-create incidents from PD alerts."
        />
        <div className="int-guide">
          <div className="int-guide-title">Setup steps</div>
          <ol className="int-guide-steps">
            <li>In PagerDuty → <strong>Integrations → Generic Webhooks (V3)</strong></li>
            <li>Click <strong>New Webhook</strong></li>
            <li>Paste the URL above</li>
            <li>Select scope: <strong>Account</strong> or specific services</li>
            <li>Select event types: <strong>incident.triggered, incident.acknowledged, incident.resolved</strong></li>
            <li>PagerDuty alerts will appear as correlated incidents in your dashboard</li>
          </ol>
        </div>
      </Section>

      {/* ─── Week 5: Datadog ─── */}
      <Section title="Datadog Monitor Webhooks" icon="🐕">
        <WebhookRow
          label="Datadog Ingest Endpoint"
          url={urls.datadog}
          desc="Add in Datadog → Integrations → Webhooks. Use @webhook-aiops in monitor message."
        />
        <div className="int-guide">
          <div className="int-guide-title">Setup steps</div>
          <ol className="int-guide-steps">
            <li>In Datadog → <strong>Integrations → Webhooks</strong> → New</li>
            <li>Name: <code>aiops</code> · URL: paste above</li>
            <li>In any monitor's <strong>Message</strong> field, add: <code>@webhook-aiops</code></li>
            <li>Severity mapping: P1→critical, P2→high, P3→medium, P4/P5→low</li>
            <li>Add <code>service:&lt;name&gt;</code> tag to monitors for accurate service matching</li>
          </ol>
        </div>
      </Section>

      {/* ─── Week 5: Slack ─── */}
      <Section title="Slack Bot" icon="💬">
        <WebhookRow
          label="Slack Slash Command URL"
          url={urls.slackCmd}
          desc="Register in your Slack App under Features → Slash Commands."
        />
        <WebhookRow
          label="Slack Interactivity URL"
          url={urls.slackInteract}
          desc="Register in your Slack App under Features → Interactivity & Shortcuts."
        />
        <div className="int-guide">
          <div className="int-guide-title">Setup steps</div>
          <ol className="int-guide-steps">
            <li>Create a Slack App at <strong>api.slack.com/apps</strong></li>
            <li>Under <strong>OAuth & Permissions</strong>, add scopes: <code>channels:manage</code>, <code>chat:write</code>, <code>commands</code></li>
            <li>Install the app to your workspace. Copy the <strong>Bot User OAuth Token</strong></li>
            <li>Add to backend env: <code>SLACK_BOT_TOKEN=xoxb-...</code></li>
            <li>Under <strong>Basic Information</strong> copy the <strong>Signing Secret</strong> → <code>SLACK_SIGNING_SECRET=...</code></li>
            <li>Under <strong>Slash Commands</strong> → Create command <code>/aiops</code> → point to the Slash Command URL above</li>
            <li>Under <strong>Interactivity</strong> → enable and paste the Interactivity URL</li>
            <li>P1 (critical) incidents will auto-create a Slack channel, post AI RCA, and show Ack/Resolve buttons</li>
            <li>Use <code>/aiops ack &lt;incident-id&gt;</code> or <code>/aiops resolve &lt;incident-id&gt;</code> from Slack</li>
          </ol>
        </div>
      </Section>

      {/* ─── Prometheus (existing) ─── */}
      <Section title="Prometheus / Alertmanager" icon="🔥">
        <WebhookRow
          label="Prometheus Alertmanager Webhook"
          url={urls.prometheus}
          desc="Use as a webhook receiver in alertmanager.yml"
        />
        <div className="int-guide">
          <div className="int-guide-title">alertmanager.yml snippet</div>
          <pre className="int-code">{`receivers:
  - name: aiops
    webhook_configs:
      - url: '${urls.prometheus}'
        send_resolved: false`}</pre>
        </div>
      </Section>

      {/* ─── Generic Webhook ─── */}
      <Section title="Generic Webhook (any tool)" icon="🔗">
        <WebhookRow
          label="Generic JSON Webhook"
          url={urls.generic}
          desc="Send any JSON with: id, source, service, severity, title, message, timestamp."
        />
        <div className="int-guide">
          <div className="int-guide-title">Example payload</div>
          <pre className="int-code">{JSON.stringify({
            id: "evt-optional",
            source: "my-tool",
            service: "checkout-api",
            severity: "critical",
            title: "High error rate",
            message: "Error rate exceeded 5% threshold",
            timestamp: new Date().toISOString(),
          }, null, 2)}</pre>
        </div>
      </Section>

      {/* ─── Week 6: Status Page ─── */}
      <Section title="Public Status Page" icon="🟢">
        <WebhookRow
          label="Status API (public, no auth)"
          url={urls.statusPage}
          method="GET"
          desc="Returns operational/degraded/outage status and all active incidents. Embed in your status page."
        />
        <div className="int-guide">
          <div className="int-guide-title">Response format</div>
          <pre className="int-code">{JSON.stringify({
            tenant: "default",
            status: "operational",
            incidents: [],
            updated_at: new Date().toISOString()
          }, null, 2)}</pre>
          <p style={{ marginTop: "0.75rem", fontSize: "0.8rem", color: "var(--muted)" }}>
            Status values: <code>operational</code> (no open incidents), <code>degraded</code> (non-critical open), <code>outage</code> (critical open).
            Filter by tenant: <code>{urls.statusPage}/default</code>
          </p>
        </div>
      </Section>
    </div>
  );
}
