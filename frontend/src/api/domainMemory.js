import { apiRequest } from "./client.js";

const BASE = "/api/v1/memory";

// Full AI memory context for an incident
export function getMemoryContext(incidentId) {
  return apiRequest(`${BASE}/context/${incidentId}`);
}

// Record a structured resolution (links to past history + upserts patterns)
export function recordResolution(incidentId, payload) {
  return apiRequest(`${BASE}/resolve/${incidentId}`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

// ── Remediation Patterns ──────────────────────────────────────────────────────

export function listRemediationPatterns(service = "") {
  const q = service ? `?service=${encodeURIComponent(service)}` : "";
  return apiRequest(`${BASE}/remediations${q}`);
}

export function createRemediationPattern(payload) {
  return apiRequest(`${BASE}/remediations`, { method: "POST", body: JSON.stringify(payload) });
}

export function updateRemediationPattern(id, payload) {
  return apiRequest(`${BASE}/remediations/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteRemediationPattern(id) {
  return apiRequest(`${BASE}/remediations/${id}`, { method: "DELETE" });
}

export function markRemediationOutcome(id, success) {
  return apiRequest(`${BASE}/remediations/${id}/outcome`, {
    method: "POST",
    body: JSON.stringify({ success }),
  });
}

// ── Deploy Signatures ─────────────────────────────────────────────────────────

export function listDeploySignatures(service = "") {
  const q = service ? `?service=${encodeURIComponent(service)}` : "";
  return apiRequest(`${BASE}/deploy-signatures${q}`);
}

export function createDeploySignature(payload) {
  return apiRequest(`${BASE}/deploy-signatures`, { method: "POST", body: JSON.stringify(payload) });
}

export function updateDeploySignature(id, payload) {
  return apiRequest(`${BASE}/deploy-signatures/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteDeploySignature(id) {
  return apiRequest(`${BASE}/deploy-signatures/${id}`, { method: "DELETE" });
}

// ── Runbook Preferences ───────────────────────────────────────────────────────

export function listRunbookPreferences(team = "", service = "") {
  const params = new URLSearchParams();
  if (team) params.set("team", team);
  if (service) params.set("service", service);
  const q = params.toString() ? `?${params.toString()}` : "";
  return apiRequest(`${BASE}/runbook-preferences${q}`);
}

export function createRunbookPreference(payload) {
  return apiRequest(`${BASE}/runbook-preferences`, { method: "POST", body: JSON.stringify(payload) });
}

export function updateRunbookPreference(id, payload) {
  return apiRequest(`${BASE}/runbook-preferences/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteRunbookPreference(id) {
  return apiRequest(`${BASE}/runbook-preferences/${id}`, { method: "DELETE" });
}
