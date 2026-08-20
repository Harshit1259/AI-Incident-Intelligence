import { getStoredToken, ensureAuthenticated, clearStoredToken } from "./auth";

const RAW_API_BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api/v1";
const API_BASE_URL = RAW_API_BASE_URL.replace(/\/+$/, "");

export async function apiRequest(path, options = {}) {
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;

  // Get or auto-provision a JWT token
  let token = getStoredToken();
  if (!token) {
    token = await ensureAuthenticated();
  }

  const authHeaders = token ? { Authorization: `Bearer ${token}` } : {};

  let response;
  try {
    response = await fetch(`${API_BASE_URL}${normalizedPath}`, {
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
        ...(options.headers || {}),
      },
      ...options,
    });
  } catch {
    throw new Error("Failed to fetch");
  }

  // If we get a 401, clear the stale token and try once more with a fresh one
  if (response.status === 401) {
    clearStoredToken();
    const freshToken = await ensureAuthenticated();
    if (freshToken) {
      try {
        response = await fetch(`${API_BASE_URL}${normalizedPath}`, {
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${freshToken}`,
            ...(options.headers || {}),
          },
          ...options,
        });
      } catch {
        throw new Error("Failed to fetch");
      }
    }
  }

  let payload = null;
  const contentType = response.headers.get("content-type") || "";

  if (contentType.includes("application/json")) {
    payload = await response.json();
  } else {
    const text = await response.text();
    payload = text ? { error: text } : null;
  }

  if (!response.ok) {
    const message =
      payload?.error ||
      payload?.message ||
      `Request failed with status ${response.status}`;
    throw new Error(message);
  }

  return payload;
}
