// integrations.js — Phase 2 Integrations API client

const BASE = "/api/v1";

export async function getStatus(tenant = "") {
  const url = tenant ? `${BASE}/status/${tenant}` : `${BASE}/status`;
  // Status page is public — no auth header needed
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`Status API ${resp.status}`);
  return resp.json();
}

export async function subscribeToStatus(channel, target, tenant = "default") {
  const url = tenant && tenant !== "default"
    ? `${BASE}/status/${tenant}/subscribe`
    : `${BASE}/status/subscribe`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ channel, target, tenant_id: tenant }),
  });
  if (!resp.ok) throw new Error(`Subscribe API ${resp.status}`);
  return resp.json();
}

// Returns the endpoints the user should configure in each supported tool.
// Only OpenTelemetry, Prometheus, Grafana, Jaeger and Zabbix are supported
// right now; every other integration was removed (see plan.md).
export function getWebhookURLs() {
  const base = window.location.origin;
  return {
    prometheus:  `${base}/api/v1/ingest/prometheus`,
    grafana:     `${base}/api/v1/ingest/grafana`,
    otelLogs:    `${base}/api/v1/otel/logs`,
    otelMetrics: `${base}/api/v1/otel/metrics`,
    otelTraces:  `${base}/api/v1/otel/traces`,
    zabbix:      `${base}/api/v1/ingest/zabbix`,
  };
}
