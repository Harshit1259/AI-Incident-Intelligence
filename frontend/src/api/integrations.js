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

// Returns the webhook URLs the user should configure in each tool
export function getWebhookURLs() {
  const base = window.location.origin;
  return {
    github:     `${base}/api/v1/ingest/github`,
    gitlab:     `${base}/api/v1/ingest/gitlab`,
    pagerduty:  `${base}/api/v1/ingest/pagerduty`,
    datadog:    `${base}/api/v1/ingest/datadog`,
    prometheus: `${base}/api/v1/ingest/prometheus`,
    generic:    `${base}/api/v1/ingest/webhook`,
    slackCmd:   `${base}/api/v1/slack/command`,
    slackInteract: `${base}/api/v1/slack/interaction`,
    statusPage: `${base}/api/v1/status`,
  };
}
