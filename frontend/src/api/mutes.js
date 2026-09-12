// mutes.js — admin-created alert mutes
//
// Every call takes the caller's session token, for the same reason as
// notifications.js: AppShell owns the token in React state, and reading a
// second copy from localStorage can disagree with it.

async function request(path, token, options = {}) {
  const res = await fetch(`/api/v1${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers || {}),
    },
  });
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      message = body.error || body.message || message;
    } catch {
      /* non-JSON error body — keep the status message */
    }
    throw new Error(message);
  }
  return res.json();
}

/** Active mutes plus those that ended in the last 7 days, with dashboard counts. */
export const listMutes = (token) => request("/mutes", token);

/** Muted alerts, newest first. Pass muteId to see one mute's entries. */
export function getMuteLog(token, { muteId = "", limit = 50, offset = 0 } = {}) {
  const q = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (muteId) q.set("mute_id", muteId);
  return request(`/mutes/log?${q}`, token);
}

/** Pre-fill the mute form from one alert or from a whole incident. */
export function suggestMute(token, { eventId = "", incidentId = "" }) {
  const q = new URLSearchParams();
  if (eventId) q.set("event_id", eventId);
  if (incidentId) q.set("incident_id", incidentId);
  return request(`/mutes/suggest?${q}`, token);
}

/** How many alerts from the last 7 days this mute would have caught. */
export const previewMute = (token, body) =>
  request("/mutes/preview", token, { method: "POST", body: JSON.stringify(body) });

export const createMute = (token, body) =>
  request("/mutes", token, { method: "POST", body: JSON.stringify(body) });

export const unmute = (token, id) =>
  request(`/mutes/${encodeURIComponent(id)}/unmute`, token, { method: "POST" });
