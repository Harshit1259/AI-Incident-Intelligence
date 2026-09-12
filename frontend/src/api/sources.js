// sources.js — ingest sources (the token each connected tool sends).
//
// Takes the caller's session token explicitly, like notifications.js and
// mutes.js, so it never disagrees with the token AppShell holds.

async function request(path, token, options = {}) {
  const res = await fetch(`/api/v1${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers || {}),
    },
  });
  let body = null;
  try {
    body = await res.json();
  } catch {
    /* empty or non-JSON body */
  }
  if (!res.ok) {
    throw new Error(body?.error || body?.message || `Request failed (${res.status})`);
  }
  return body;
}

export const listSources = (token) => request("/sources", token).then((d) => d?.items || []);

/** Creates a source; the response is the only time its token is returned. */
export const createSource = (token, name, type) =>
  request("/sources", token, { method: "POST", body: JSON.stringify({ name, type }) });

/** Issues a new token; the old one stops working immediately. */
export const rotateSourceToken = (token, id) =>
  request(`/sources/${encodeURIComponent(id)}/rotate`, token, { method: "POST" });

export const deleteSource = (token, id) =>
  request(`/sources/${encodeURIComponent(id)}`, token, { method: "DELETE" });

/** Sends the integration's sample payload through the real parser. */
export const sendSourceTest = (token, id) =>
  request("/sources/test", token, { method: "POST", body: JSON.stringify({ source_id: id }) });
