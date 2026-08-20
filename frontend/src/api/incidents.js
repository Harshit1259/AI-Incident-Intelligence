import { apiRequest } from "./client";

export function getIncidents(filters) {
  const searchParams = new URLSearchParams();

  Object.entries(filters).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      searchParams.set(key, String(value));
    }
  });

  const queryString = searchParams.toString();
  const path = queryString ? `/incidents?${queryString}` : "/incidents";

  return apiRequest(path);
}

export function getIncidentDetail(incidentId) {
  return apiRequest(`/incidents/${incidentId}`);
}

export function updateIncidentStatus(incidentId, action) {
  return apiRequest(`/incidents/${incidentId}/${action}`, {
    method: "POST",
  });
}

export function getIncidentExplanation(incidentId) {
  return apiRequest(`/incidents/explain/${incidentId}`);
}

// Explicitly run AI root-cause analysis on an incident.
//
// This is a POST because it spends an LLM call and produces a causal claim
// attributed to the caller. It must only ever be triggered by a user action —
// never on mount, never on poll. Opening an incident is free; analysing it
// is not.
//
// Can take up to 30s, so callers should show a pending state rather than a
// spinner that looks stuck.
export function analyzeIncident(incidentId) {
  return apiRequest(`/incidents/${incidentId}/analyze`, {
    method: "POST",
  });
}

export function askIncidentCopilot(incidentId, question) {
  return apiRequest(`/incidents/copilot/${incidentId}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ question }),
  });
}

export function getIncidentActivity(incidentId) {
  return apiRequest(`/incidents/activity/${incidentId}`);
}

export function getIncidentCounts() {
  return apiRequest("/incidents/counts");
}
