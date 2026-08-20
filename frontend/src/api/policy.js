// policy.js — Automation Policy Engine API client

import { apiRequest } from "./client.js";

// ── Policy CRUD ──────────────────────────────────────────────────────────────

export const getPolicies = () =>
  apiRequest("/policies");

export const getPolicyByID = (id) =>
  apiRequest(`/policies/${id}`);

export const createPolicy = (policy) =>
  apiRequest("/policies", {
    method: "POST",
    body: JSON.stringify(policy),
  });

export const updatePolicy = (id, policy) =>
  apiRequest(`/policies/${id}`, {
    method: "PUT",
    body: JSON.stringify(policy),
  });

export const deletePolicy = (id) =>
  apiRequest(`/policies/${id}`, { method: "DELETE" });

// ── Policy engine ─────────────────────────────────────────────────────────────

export const evaluatePolicy = (ctx) =>
  apiRequest("/policies/evaluate", {
    method: "POST",
    body: JSON.stringify(ctx),
  });

export const simulatePolicy = (ctx) =>
  apiRequest("/policies/simulate", {
    method: "POST",
    body: JSON.stringify(ctx),
  });

// ── Execution trail ──────────────────────────────────────────────────────────

export const getTenantTrail = (limit = 100) =>
  apiRequest(`/policies/trail?limit=${limit}`);

export const getPolicyTrail = (policyID, limit = 50) =>
  apiRequest(`/policies/${policyID}/trail?limit=${limit}`);

// ── Circuit breakers ─────────────────────────────────────────────────────────

export const getCircuitBreakers = () =>
  apiRequest("/policies/circuit-breakers");

export const resetCircuitBreaker = (policyID) =>
  apiRequest(`/policies/${policyID}/circuit-breaker/reset`, { method: "POST" });
