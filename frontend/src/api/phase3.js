// phase3.js — Phase 3 API client (SLOs, On-Call, Anomalies, Health, ROI, Digest)

import { apiRequest } from "./client.js";

// ── SLO Tracking ──
export const getSLOs = () => apiRequest("/slos");
export const createSLO = (data) => apiRequest("/slos", { method: "POST", body: JSON.stringify(data) });
export const getSLOByID = (id) => apiRequest(`/slos/${id}`);
export const deleteSLO = (id) => apiRequest(`/slos/${id}`, { method: "DELETE" });
export const recordSLOMeasurement = (id, data) =>
  apiRequest(`/slos/${id}/measure`, { method: "POST", body: JSON.stringify(data) });

// ── On-Call ──
export const getOnCallSchedules = () => apiRequest("/oncall");
export const createOnCallSchedule = (data) => apiRequest("/oncall", { method: "POST", body: JSON.stringify(data) });
export const getOnCallByID = (id) => apiRequest(`/oncall/${id}`);
export const deleteOnCallSchedule = (id) => apiRequest(`/oncall/${id}`, { method: "DELETE" });
export const addOnCallOverride = (id, data) =>
  apiRequest(`/oncall/${id}/override`, { method: "POST", body: JSON.stringify(data) });

// ── Anomaly Detection ──
export const getAnomalies = () => apiRequest("/anomalies");
export const checkMetric = (data) => apiRequest("/anomalies/check", { method: "POST", body: JSON.stringify(data) });
export const ackAnomaly = (id) => apiRequest(`/anomalies/${id}/ack`, { method: "POST" });
export const simulateAnomaly = (service) =>
  apiRequest("/anomalies/simulate", { method: "POST", body: JSON.stringify({ service }) });

// ── Engineering Health ──
export const getEngineeringHealth = (days = 30) => apiRequest(`/engineering/health?days=${days}`);

// ── ROI Dashboard ──
export const getROI = (days = 30) => apiRequest(`/roi?days=${days}`);

// ── Weekly Digest ──
export const getWeeklyDigest = () => apiRequest("/digest/weekly");
export const sendDigest = () => apiRequest("/digest/send", { method: "POST" });

// ── Topology: Incident-specific evidence graph (backwards compat) ──
export const getTopologyGraph = (incidentId) => apiRequest(`/incidents/topology/${incidentId}`);

// ── Topology: Live tenant-wide graph ──
export const getLiveTopologyGraph = () => apiRequest("/topology/graph");

// ── Topology: Blast radius for a service ──
export const getBlastRadius = (nodeId) => apiRequest(`/topology/blast-radius/${encodeURIComponent(nodeId)}`);

// ── Topology: Causal RCA for an incident ──
export const getCausalRCA = (incidentId) => apiRequest(`/topology/rca/${incidentId}`);

// ── Topology: Node & edge management ──
export const upsertTopologyNode = (node) =>
  apiRequest("/topology/nodes", { method: "POST", body: JSON.stringify(node) });
export const upsertTopologyEdge = (edge) =>
  apiRequest("/topology/edges", { method: "POST", body: JSON.stringify(edge) });

// ── Topology: Auto-discovery from incident history ──
export const discoverTopology = () => apiRequest("/topology/discover", { method: "POST" });

// ── Incident Memory & Learning ──
export const getIncidentMemory = (incidentId) => apiRequest(`/incidents/memory/${incidentId}`);
export const recordIncidentResolution = (incidentId, note) =>
  apiRequest(`/incidents/memory/${incidentId}/record`, {
    method: "POST",
    body: JSON.stringify({ note }),
  });

// ── Business Risk / Exposure Dashboard ──
export const getRiskExposure = () => apiRequest("/risk/exposure");
export const getAtRiskServices = () => apiRequest("/risk/at-risk-services");

// ── AI / Deployment Status ──
export const getAIStatus = () => apiRequest("/ai/status");
