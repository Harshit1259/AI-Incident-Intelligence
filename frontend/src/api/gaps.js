// gaps.js — API client for gap-filling features

import { apiRequest } from "./client.js";

// ── Business Impact ──
export const getBusinessImpact = (incidentID) => apiRequest(`/incidents/${incidentID}/business-impact`);
export const generateBusinessImpact = (incidentID) => apiRequest(`/incidents/${incidentID}/business-impact/generate`, { method: "POST" });

// ── Alert Feedback ──
export const submitAlertFeedback = (data) => apiRequest("/alerts/feedback", { method: "POST", body: JSON.stringify(data) });
export const getAlertFeedbackStats = () => apiRequest("/alerts/feedback/stats");
export const getSuppressedAlerts = () => apiRequest("/alerts/suppressed");

// ── Auto-Resolve Rules ──
export const getAutoResolveRules = () => apiRequest("/auto-resolve/rules");
export const createAutoResolveRule = (data) => apiRequest("/auto-resolve/rules", { method: "POST", body: JSON.stringify(data) });
export const deleteAutoResolveRule = (id) => apiRequest(`/auto-resolve/rules/${id}`, { method: "DELETE" });
export const toggleAutoResolveRule = (id, enabled) => apiRequest(`/auto-resolve/rules/${id}/toggle`, { method: "PUT", body: JSON.stringify({ enabled }) });

// ── Runbooks ──
export const getRunbooks = (service, q) => {
  const params = new URLSearchParams();
  if (service) params.set("service", service);
  if (q) params.set("q", q);
  return apiRequest(`/runbooks?${params}`);
};
export const createRunbook = (data) => apiRequest("/runbooks", { method: "POST", body: JSON.stringify(data) });
export const getRunbookByID = (id) => apiRequest(`/runbooks/${id}`);
export const updateRunbook = (id, data) => apiRequest(`/runbooks/${id}`, { method: "PUT", body: JSON.stringify(data) });
export const deleteRunbook = (id) => apiRequest(`/runbooks/${id}`, { method: "DELETE" });
export const getIncidentRunbooks = (incidentID) => apiRequest(`/incidents/${incidentID}/runbooks`);

// ── Dependencies ──
export const getDependencies = () => apiRequest("/dependencies");
export const addDependency = (data) => apiRequest("/dependencies", { method: "POST", body: JSON.stringify(data) });
export const deleteDependency = (id) => apiRequest(`/dependencies/${id}`, { method: "DELETE" });
export const getIncidentAttribution = (incidentID) => apiRequest(`/incidents/${incidentID}/attribution`);

// ── WhatsApp ──
export const getWhatsAppConfig = () => apiRequest("/whatsapp/config");
export const saveWhatsAppConfig = (data) => apiRequest("/whatsapp/config", { method: "PUT", body: JSON.stringify(data) });
export const testWhatsApp = () => apiRequest("/whatsapp/test", { method: "POST" });

// ── Compliance ──
export const getComplianceReport = (days = 30) => apiRequest(`/compliance/report?days=${days}`);
export const exportComplianceCSV = (days = 30) =>
  fetch(`/api/v1/compliance/export?days=${days}`, {
    headers: { Authorization: `Bearer ${localStorage.getItem("aiops_token")}` },
  }).then(r => r.blob());
