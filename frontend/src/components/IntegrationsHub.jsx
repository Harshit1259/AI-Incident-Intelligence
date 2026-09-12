/**
 * Integrations — webhook endpoints and setup guides.
 *
 * Restructured from a stack of accordions into a two-column layout: pick a
 * tool on the left, read its setup on the right. The old version made you
 * expand each section to find the URL you wanted and offered no way to see
 * what was available at a glance.
 *
 * Emoji section markers were replaced with real icons — they render
 * consistently and carry the same meaning at 13px.
 */
import { useState } from "react";
import {
  Check, Copy, Database, Flame, GitBranch, Radio, Signal,
} from "lucide-react";

import { getWebhookURLs } from "../api/integrations.js";
import { roleFromToken } from "../api/auth.js";
import SourcesSection from "./SourcesSection.jsx";
import { Page, PageHeader, Panel } from "./ui/Primitives.jsx";

const TOKEN_PLACEHOLDER = "<your source token>";

/* Fill a freshly revealed token into the setup text, so the snippet is
   ready to paste. Only ever the token just created or rotated here. */
function withToken(text, token) {
  return token ? text.split(TOKEN_PLACEHOLDER).join(token) : text;
}

function CopyButton({ value }) {
  const [copied, setCopied] = useState(false);

  function copy() {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <button type="button" className="btn btn-outline btn-xs copy-btn" onClick={copy}>
      {copied ? <Check size={11} /> : <Copy size={11} />}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function EndpointRow({ label, url, method = "POST", desc }) {
  return (
    <div className="endpoint">
      <div className="endpoint-text">
        <p className="endpoint-label">{label}</p>
        {desc && <p className="endpoint-desc">{desc}</p>}
        <div className="endpoint-url">
          <span className={`method-badge method-${method.toLowerCase()}`}>{method}</span>
          <code>{url}</code>
        </div>
      </div>
      <CopyButton value={url} />
    </div>
  );
}

function Steps({ items }) {
  return (
    <ol className="setup-steps">
      {items.map((step, i) => (
        <li key={i} className="setup-step">
          <span className="setup-step-index">{i + 1}</span>
          <span className="setup-step-text">{step}</span>
        </li>
      ))}
    </ol>
  );
}

/* The catalogue. Each entry owns its endpoints, steps and optional code sample,
   so adding a tool is a data change rather than more JSX.

   Only five open-source integrations are offered right now; every other one
   was removed and is listed in plan.md for later. Every endpoint needs a
   source token, sent as X-Source-Token or as Authorization: Bearer <token>. */
function buildCatalogue(urls) {
  return [
    {
      key: "otel",
      name: "OpenTelemetry",
      icon: Radio,
      blurb: "Logs, metrics and traces over OTLP",
      endpoints: [
        { label: "OTLP logs", url: urls.otelLogs, desc: "OTLP/HTTP — protobuf or JSON, gzip or not." },
        { label: "OTLP metrics", url: urls.otelMetrics, desc: "OTLP/HTTP — protobuf or JSON, gzip or not." },
        { label: "OTLP traces", url: urls.otelTraces, desc: "OTLP/HTTP. Failing spans become trace errors." },
      ],
      steps: [
        "Add the otlphttp exporter below to your OpenTelemetry Collector.",
        "The exporter's defaults work (protobuf + gzip). OTLP over gRPC (port 4317) is not supported — use otlphttp.",
        "Put your source token in the X-Source-Token header.",
        "Add the exporter to each pipeline (logs, metrics, traces) you want to send, then restart the Collector.",
      ],
      code: {
        title: "otel-collector.yaml",
        body: `exporters:
  otlphttp/neuroops:
    logs_endpoint: ${urls.otelLogs}
    metrics_endpoint: ${urls.otelMetrics}
    traces_endpoint: ${urls.otelTraces}
    headers:
      X-Source-Token: <your source token>

service:
  pipelines:
    metrics:
      exporters: [otlphttp/neuroops]   # keep your other exporters too`,
      },
    },
    {
      key: "prometheus",
      name: "Prometheus",
      icon: Flame,
      blurb: "Alertmanager receiver",
      endpoints: [
        { label: "Alertmanager webhook", url: urls.prometheus, desc: "Use as a webhook receiver in alertmanager.yml." },
      ],
      steps: [
        "Add the receiver below to alertmanager.yml and route your alerts to it.",
        "Give every alert rule a service label — NeuroOps groups alerts into incidents by service. If the label names an exporter (e.g. kube-state-metrics), NeuroOps uses namespace/workload instead.",
        "Reload Alertmanager.",
      ],
      code: {
        title: "alertmanager.yml",
        body: `receivers:
  - name: neuroops
    webhook_configs:
      - url: '${urls.prometheus}'
        send_resolved: true
        http_config:
          authorization:
            type: Bearer
            credentials: '<your source token>'`,
      },
    },
    {
      key: "grafana",
      name: "Grafana",
      icon: Signal,
      blurb: "Grafana Alerting contact point",
      endpoints: [
        {
          label: "Grafana webhook",
          url: urls.grafana,
          desc: "For a Webhook contact point (Grafana 11, 12 and 13). Resolved alerts close incidents automatically.",
        },
      ],
      steps: [
        "Connect Grafana below to get a source token.",
        "In Grafana: Alerting → Contact points → + Add contact point. Name it NeuroOps, choose the Webhook integration and paste the URL above.",
        "Open Optional Webhook settings: set Authorization Header - Scheme to Bearer and Authorization Header - Credentials to <your source token>.",
        "Click Test. A “[Test] Grafana contact point test” incident appears in NeuroOps — open it to check the alert arrived. It is marked recovered and closes itself after a few minutes.",
        "Save the contact point, then in Alerting → Notification policies make NeuroOps the default contact point, or add a child policy that matches the alerts to send (turn on Continue matching to keep your existing notifications).",
        "Set root_url under [server] in grafana.ini (e.g. https://grafana.example.com/). Without it the “Open rule in Grafana” and “Dashboard” links point to localhost:3000.",
        "Give alert rules a service label (otherwise the folder name is used) and a severity label (otherwise medium). “No data” and query-error alerts become monitoring-gap incidents with severity unknown.",
        "Prefer files? Put the provisioning file below in provisioning/alerting/ instead of steps 2–3 and restart Grafana.",
      ],
      code: {
        title: "provisioning/alerting/neuroops.yaml",
        body: `apiVersion: 1
contactPoints:
  - orgId: 1
    name: NeuroOps
    receivers:
      - uid: neuroops
        type: webhook
        settings:
          url: ${urls.grafana}
          httpMethod: POST
          authorization_scheme: Bearer
          authorization_credentials: <your source token>`,
      },
    },
    {
      key: "jaeger",
      name: "Jaeger",
      icon: GitBranch,
      blurb: "Traces via the OpenTelemetry Collector",
      endpoints: [
        {
          label: "OTLP traces",
          url: urls.otelTraces,
          desc: "Jaeger has no alerting of its own — send the same traces to NeuroOps through the Collector.",
        },
      ],
      steps: [
        "In the Collector pipeline that exports traces to Jaeger, add a second exporter for NeuroOps.",
        "The exporter's defaults work (protobuf + gzip); use otlphttp, not the gRPC otlp exporter.",
        "Put your source token in the X-Source-Token header.",
        "Failing spans (status ERROR) become trace-error signals for incident correlation.",
      ],
      code: {
        title: "otel-collector.yaml",
        body: `exporters:
  otlp/jaeger:
    endpoint: jaeger-collector:4317     # your existing Jaeger exporter
  otlphttp/neuroops:
    traces_endpoint: ${urls.otelTraces}
    headers:
      X-Source-Token: <your source token>

service:
  pipelines:
    traces:
      exporters: [otlp/jaeger, otlphttp/neuroops]`,
      },
    },
    {
      key: "zabbix",
      name: "Zabbix",
      icon: Database,
      blurb: "Problems, recoveries and acks",
      endpoints: [
        {
          label: "Zabbix webhook",
          url: urls.zabbix,
          desc: "Used by the NeuroOps media type. Recoveries close incidents automatically.",
        },
      ],
      download: { label: "Download NeuroOps media type (Zabbix 6.0 / 7.0)", href: "/integrations/neuroops-zabbix.yaml" },
      steps: [
        "Download the NeuroOps media type below.",
        "In Zabbix: Alerts → Media types → Import, and choose the file.",
        "Open the NeuroOps media type and set url to the endpoint above and token to <your source token>.",
        "Optional: set the global macro {$ZABBIX.URL} to your Zabbix address for “Open in Zabbix” links.",
        "Create a dedicated user (e.g. neuroops) in a user group with read access to the host groups you want sent, and add Media → NeuroOps to it. Use a dedicated user: Zabbix never sends update notifications to the person who made the update.",
        "Alerts → Actions → Trigger actions → Create action. Add a condition (e.g. severity ≥ Warning) and add NeuroOps under Operations, Recovery operations and Update operations.",
        "Back here, use Test on your source — or click Test on the media type in Zabbix.",
        "Tip: add a service tag to triggers or hosts (service = billing) so incidents group by service; otherwise the host group is used.",
      ],
    },
  ];
}

export default function IntegrationsHub({ token }) {
  const urls = getWebhookURLs();
  const catalogue = buildCatalogue(urls);
  const [activeKey, setActiveKey] = useState(catalogue[0].key);
  const [revealedToken, setRevealedToken] = useState(null);
  const active = catalogue.find((c) => c.key === activeKey) || catalogue[0];
  const ActiveIcon = active.icon;
  const role = roleFromToken(token);
  const canManage = role === "admin" || role === "operator";

  return (
    <Page>
      <PageHeader
        title="Connect your tools"
        meta="OpenTelemetry, Prometheus, Grafana, Jaeger and Zabbix — paste the endpoint into your tool and incidents flow automatically"
      />

      <div className="integrations-layout">
        <nav className="integration-nav" aria-label="Integrations">
          {catalogue.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.key}
                type="button"
                className={`integration-nav-item${item.key === activeKey ? " is-active" : ""}`}
                onClick={() => {
                  setActiveKey(item.key);
                  setRevealedToken(null);
                }}
              >
                <span className="integration-nav-icon" aria-hidden="true">
                  <Icon size={14} />
                </span>
                <span className="integration-nav-text">
                  <span className="integration-nav-name">{item.name}</span>
                  <span className="integration-nav-blurb">{item.blurb}</span>
                </span>
              </button>
            );
          })}
        </nav>

        <Panel
          title={
            <span className="integration-title">
              <ActiveIcon size={14} /> {active.name}
            </span>
          }
          sub={active.blurb}
        >
          {active.notice && <p className="integration-notice">{active.notice}</p>}
          {active.endpoints.map((ep) => (
            <EndpointRow key={ep.label} {...ep} />
          ))}
          {active.download && (
            <a className="btn btn-outline btn-sm integration-download" href={active.download.href} download>
              {active.download.label}
            </a>
          )}

          <SourcesSection
            key={active.key}
            token={token}
            type={active.key}
            integrationName={active.name}
            canManage={canManage}
            onTokenRevealed={setRevealedToken}
          />

          <div className="detail-section is-divided">
            <h4 className="detail-section-label">Setup</h4>
            <Steps items={active.steps.map((step) => withToken(step, revealedToken))} />
          </div>

          {active.code && (
            <div className="detail-section is-divided">
              <h4 className="detail-section-label">{active.code.title}</h4>
              <pre className="code-block">{withToken(active.code.body, revealedToken)}</pre>
            </div>
          )}
        </Panel>
      </div>
    </Page>
  );
}
