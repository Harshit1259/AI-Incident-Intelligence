import { apiRequest } from "./client.js";

const BASE = "/api/schema-registry";

export function listSchemaMappings() {
  return apiRequest(BASE);
}

export function getSchemaMappingStats() {
  return apiRequest(`${BASE}/stats`);
}

export function createSchemaMapping(payload) {
  return apiRequest(BASE, { method: "POST", body: JSON.stringify(payload) });
}

export function updateSchemaMapping(id, payload) {
  return apiRequest(`${BASE}/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteSchemaMapping(id) {
  return apiRequest(`${BASE}/${id}`, { method: "DELETE" });
}
