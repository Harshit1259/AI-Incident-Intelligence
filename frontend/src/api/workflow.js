import { apiRequest } from "./client.js";

const INC = (id) => `/api/v1/incidents/${id}`;
const TMPL = "/api/v1/workflow/templates";

// ── Commander ─────────────────────────────────────────────────────────────────

export function getCurrentCommander(incidentId) {
  return apiRequest(`${INC(incidentId)}/commander`);
}

export function getCommanderHistory(incidentId) {
  return apiRequest(`${INC(incidentId)}/commander/history`);
}

export function assignCommander(incidentId, payload) {
  return apiRequest(`${INC(incidentId)}/commander`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

// ── Templates ─────────────────────────────────────────────────────────────────

export function listTemplates() {
  return apiRequest(TMPL);
}

export function createTemplate(payload) {
  return apiRequest(TMPL, { method: "POST", body: JSON.stringify(payload) });
}

export function updateTemplate(id, payload) {
  return apiRequest(`${TMPL}/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteTemplate(id) {
  return apiRequest(`${TMPL}/${id}`, { method: "DELETE" });
}

export function renderTemplate(templateId, incidentId) {
  return apiRequest(`${TMPL}/${templateId}/render`, {
    method: "POST",
    body: JSON.stringify({ incident_id: incidentId }),
  });
}

// ── Tickets ───────────────────────────────────────────────────────────────────

export function getTickets(incidentId) {
  return apiRequest(`${INC(incidentId)}/tickets`);
}

export function createTicket(incidentId, payload) {
  return apiRequest(`${INC(incidentId)}/tickets`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function syncTicket(incidentId, provider) {
  return apiRequest(`${INC(incidentId)}/tickets/${provider}/sync`, { method: "POST" });
}

// ── Executive Summary ─────────────────────────────────────────────────────────

export function generateExecSummary(incidentId) {
  return apiRequest(`${INC(incidentId)}/exec-summary`, { method: "POST" });
}

// ── Status Communications ─────────────────────────────────────────────────────

export function getStatusComms(incidentId) {
  return apiRequest(`${INC(incidentId)}/status-comms`);
}

export function publishStatusComm(incidentId, payload) {
  return apiRequest(`${INC(incidentId)}/status-comms`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}
