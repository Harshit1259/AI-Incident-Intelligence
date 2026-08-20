// auth.js — Authentication for the AI Incident Platform
//
// Flow on first load:
//   1. Check localStorage for a valid (non-expired) JWT -> use it
//   2. If no token, return null (caller should show a login form)
//
// login() and register() are available for the UI to call with user-provided credentials.

const API_BASE = "/api/v1";

const TOKEN_KEY = "aiops_jwt_token";
const TOKEN_EXP = "aiops_jwt_exp";

// ─────────────────────────────────────────────────────
// Token storage
// ─────────────────────────────────────────────────────

export function getStoredToken() {
  try {
    const token = localStorage.getItem(TOKEN_KEY);
    if (!token) return null;

    const exp = Number(localStorage.getItem(TOKEN_EXP) || 0);
    // Expire 60 s early to avoid sending a token that expires mid-request
    if (exp > 0 && Date.now() / 1000 > exp - 60) {
      clearStoredToken();
      return null;
    }
    return token;
  } catch {
    return null;
  }
}

export function storeToken(token) {
  try {
    localStorage.setItem(TOKEN_KEY, token);
    // Decode exp from JWT payload (base64url-encoded JSON)
    const parts = token.split(".");
    if (parts.length === 3) {
      // base64url -> base64 -> decode
      const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
      const padded = b64 + "=".repeat((4 - b64.length % 4) % 4);
      const payload = JSON.parse(atob(padded));
      if (payload.exp) {
        localStorage.setItem(TOKEN_EXP, String(payload.exp));
      }
    }
  } catch {
    // ignore storage errors
  }
}

export function clearStoredToken() {
  try {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(TOKEN_EXP);
  } catch {
    // ignore
  }
}

// ─────────────────────────────────────────────────────
// ensureAuthenticated
// ─────────────────────────────────────────────────────

/**
 * ensureAuthenticated() — returns a valid JWT string if one exists in storage,
 * or null if no token is available. The caller should show a login form when null.
 */
export async function ensureAuthenticated() {
  // Fast path: valid token already in storage
  const cached = getStoredToken();
  if (cached) return cached;

  // No stored token — caller should present a login UI
  return null;
}

// ─────────────────────────────────────────────────────
// Login / Register API calls (for use by login form UI)
// ─────────────────────────────────────────────────────

/**
 * login() — authenticate with email and password.
 * Returns the JWT token on success, or null on failure.
 */
export async function login(email, password) {
  try {
    const resp = await fetch(`${API_BASE}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });

    if (!resp.ok) return null;

    const data = await resp.json();
    if (data.token) {
      storeToken(data.token);
      console.warn("[auth] Logged in successfully");
      return data.token;
    }
    return null;
  } catch (err) {
    console.warn("[auth] Login network error:", err.message);
    return null;
  }
}

/**
 * register() — create a new account.
 * Returns the JWT token on success, or null on failure.
 */
export async function register(email, password, role = "operator", tenantId = "default") {
  try {
    const resp = await fetch(`${API_BASE}/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email,
        password,
        role,
        tenant_id: tenantId,
      }),
    });

    if (!resp.ok) {
      const body = await resp.json().catch(() => ({}));
      console.error("[auth] Register failed:", resp.status, body.error || "");
      return null;
    }

    const data = await resp.json();
    if (data.token) {
      storeToken(data.token);
      console.warn("[auth] Registered and logged in successfully");
      return data.token;
    }
    return null;
  } catch (err) {
    console.warn("[auth] Register network error:", err.message);
    return null;
  }
}
