// api/client.js — AIOps mobile API client
// All calls go through this module so auth headers and base URL are centralised.

import * as SecureStore from 'expo-secure-store';

// Change this to your deployed backend URL.
// In development, use your machine's LAN IP so the device/emulator can reach it.
// e.g. http://192.168.1.42:8080
const BASE_URL = process.env.EXPO_PUBLIC_API_URL || 'http://localhost:8080';

const TOKEN_KEY = 'aiops_jwt';

// ── Token management ─────────────────────────────────────────────────────────

export async function saveToken(token) {
  await SecureStore.setItemAsync(TOKEN_KEY, token);
}

export async function getToken() {
  return SecureStore.getItemAsync(TOKEN_KEY);
}

export async function clearToken() {
  await SecureStore.deleteItemAsync(TOKEN_KEY);
}

// ── Core fetch wrapper ───────────────────────────────────────────────────────

async function request(method, path, body = null) {
  const token = await getToken();
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const options = { method, headers };
  if (body) options.body = JSON.stringify(body);

  const resp = await fetch(`${BASE_URL}${path}`, options);
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`API ${resp.status}: ${text}`);
  }
  return resp.json();
}

// ── Auth ──────────────────────────────────────────────────────────────────────

export async function login(email, password) {
  const data = await request('POST', '/api/v1/auth/login', { email, password });
  if (data.token) await saveToken(data.token);
  return data;
}

export async function logout() {
  await clearToken();
}

// ── Incidents ─────────────────────────────────────────────────────────────────

export async function listIncidents(status = '', page = 1, pageSize = 20) {
  const params = new URLSearchParams({ page, page_size: pageSize });
  if (status) params.set('status', status);
  return request('GET', `/api/v1/incidents?${params}`);
}

export async function getIncident(id) {
  return request('GET', `/api/v1/incidents/${id}`);
}

export async function acknowledgeIncident(id) {
  return request('POST', `/api/v1/incidents/${id}/acknowledge`);
}

export async function resolveIncident(id) {
  return request('POST', `/api/v1/incidents/${id}/resolve`);
}

export async function escalateIncident(id) {
  return request('POST', `/api/v1/incidents/${id}/escalate`);
}

// ── AI Explain ────────────────────────────────────────────────────────────────

export async function explainIncident(id) {
  return request('POST', `/api/v1/incidents/${id}/explain`);
}

// ── Automation actions (approve / reject) ────────────────────────────────────

export async function listPendingActions(incidentId) {
  return request('GET', `/api/v1/incidents/${incidentId}/actions`);
}

export async function approveAction(actionId) {
  return request('POST', `/api/v1/actions/${actionId}/approve`);
}

export async function rejectAction(actionId) {
  return request('POST', `/api/v1/actions/${actionId}/reject`);
}

// ── Push registration ─────────────────────────────────────────────────────────

export async function registerPushToken(expoToken, appVersion = '1.0.0') {
  return request('POST', '/api/v1/push/register', {
    platform: 'expo',
    device_token: expoToken,
    app_version: appVersion,
  });
}
