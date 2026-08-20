// postmortem.js — Phase 2 Post-Mortem API client

import { apiRequest } from "./client.js";

const BASE = "/incidents";

export async function getPostMortem(incidentID) {
  return apiRequest(`${BASE}/${incidentID}/postmortem`);
}

export async function generatePostMortem(incidentID) {
  return apiRequest(`${BASE}/${incidentID}/postmortem/generate`, {
    method: "POST",
  });
}

export async function updatePostMortem(incidentID, data) {
  return apiRequest(`${BASE}/${incidentID}/postmortem`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}
