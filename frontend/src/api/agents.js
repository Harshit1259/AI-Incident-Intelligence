// agents.js — Agent management API

import { apiRequest } from "./client.js";

export const getAgents = () => apiRequest("/agents");
export const getAgentByID = (id) => apiRequest(`/agents/${id}`);
export const startAgent = (id) => apiRequest(`/agents/${id}/start`, { method: "POST" });
export const stopAgent = (id) => apiRequest(`/agents/${id}/stop`, { method: "POST" });
export const deleteAgent = (id) => apiRequest(`/agents/${id}`, { method: "DELETE" });
export const getAgentMetrics = (id, type, limit = 20) => {
  const params = new URLSearchParams();
  if (type) params.set("type", type);
  if (limit) params.set("limit", String(limit));
  return apiRequest(`/agents/${id}/metrics?${params}`);
};
