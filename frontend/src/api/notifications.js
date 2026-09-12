// notifications.js — operator notification feed
//
// One endpoint backs both the bell badge and the slide-over panel, so the
// count can never disagree with the list.

import { apiRequest } from "./client.js";

/**
 * Fetch the notification feed.
 *
 * Pass the caller's token explicitly when it owns one. AppShell holds the
 * session token in React state and drives every other request from it, so the
 * feed must use that same value — falling back to client.js (which reads
 * localStorage independently) would give us a second source of truth for the
 * same token, and the two can disagree: localStorage expires its copy 60s
 * early, so state can still hold a working token after storage has dropped it.
 * That mismatch sends a request with no Authorization header at all.
 *
 * With no token argument we fall back to the shared client, which handles
 * lookup and 401 retry on its own.
 */
export async function getNotifications(token) {
  if (!token) return apiRequest("/notifications");

  const res = await fetch("/api/v1/notifications", {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
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

// ── Dismissal ────────────────────────────────────────────────────────────────
//
// Dismissals live in localStorage, not the database. The feed is derived from
// live state on every request, so "dismissed" is a per-viewer preference
// ("I've seen this, stop showing me"), not a fact about the tenant.
//
// A dismissal records the LastSeen timestamp at the moment it was dismissed.
// An item stays hidden only while its LastSeen has not moved past that mark —
// so if the same problem recurs, it comes back. Without this, dismissing a
// broken integration once would hide it forever, which is the silent-failure
// bug this whole feature exists to prevent.

const STORAGE_KEY = "neuroops.notifications.dismissed";

// Dismissals older than this are pruned so the key cannot grow without bound.
const DISMISSAL_TTL_MS = 30 * 24 * 60 * 60 * 1000; // 30 days

function readDismissals() {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    // Private mode, disabled storage, or corrupt JSON — behave as if nothing
    // was ever dismissed rather than breaking the panel.
    return {};
  }
}

function writeDismissals(map) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    /* storage unavailable — dismissal is best-effort by design */
  }
}

/** Hide a notification until its underlying problem recurs. */
export function dismissNotification(id, lastSeen) {
  if (!id) return;
  const map = readDismissals();
  map[id] = { at: Date.now(), lastSeen: lastSeen || null };
  writeDismissals(prune(map));
}

/** Clear every dismissal, revealing all current notifications. */
export function clearDismissals() {
  writeDismissals({});
}

/**
 * Remove items the viewer has dismissed, unless they have recurred since.
 * Returns a new array; the input is not modified.
 */
export function applyDismissals(items) {
  if (!Array.isArray(items) || items.length === 0) return [];
  const map = readDismissals();
  if (Object.keys(map).length === 0) return items;

  return items.filter((n) => {
    const entry = map[n.id];
    if (!entry) return true;

    // No recorded timestamp (older dismissal format) — honour it as-is.
    if (!entry.lastSeen) return false;

    const dismissedAt = Date.parse(entry.lastSeen);
    const currentAt = Date.parse(n.last_seen);
    if (Number.isNaN(dismissedAt) || Number.isNaN(currentAt)) return true;

    // Recurred since dismissal → show it again.
    return currentAt > dismissedAt;
  });
}

function prune(map) {
  const cutoff = Date.now() - DISMISSAL_TTL_MS;
  const out = {};
  for (const [id, entry] of Object.entries(map)) {
    if (entry && typeof entry.at === "number" && entry.at >= cutoff) out[id] = entry;
  }
  return out;
}
