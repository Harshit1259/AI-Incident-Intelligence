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
