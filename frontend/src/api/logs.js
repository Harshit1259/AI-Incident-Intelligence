// logs.js — Log Explorer API

import { apiRequest } from "./client.js";

export async function queryLogs({ agentId, host, category, tag, search, from, to, limit = 100, offset = 0 } = {}) {
  const params = new URLSearchParams();
  if (agentId) params.set("agent_id", agentId);
  if (host) params.set("host", host);
  if (category) params.set("category", category);
  if (tag) params.set("tag", tag);
  if (search) params.set("search", search);
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  params.set("limit", String(limit));
  params.set("offset", String(offset));
  return apiRequest(`/logs?${params}`);
}

export async function getLogStats(sinceHours = 24) {
  return apiRequest(`/logs/stats?since=${sinceHours}`);
}

export function createLogStream(lastId = 0, onEntry, onError) {
  const token = localStorage.getItem("aiops_token");
  const url = `/api/v1/logs/stream?last_id=${lastId}`;
  const es = new EventSource(url + `&token=${token}`);
  // EventSource doesn't support auth headers, so we use query param fallback
  // For SSE, we'll use fetch-based polling instead
  es.close();

  // Use polling-based approach since SSE + auth is tricky
  let running = true;
  let currentLastId = lastId;

  async function poll() {
    while (running) {
      try {
        const resp = await fetch(`/api/v1/logs/stream?last_id=${currentLastId}`, {
          headers: { Authorization: `Bearer ${token}` },
          signal: AbortSignal.timeout(35000),
        });
        if (!resp.ok) throw new Error(resp.status);
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        while (running) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n\n");
          buffer = lines.pop() || "";
          for (const line of lines) {
            if (line.startsWith("data: ")) {
              try {
                const entry = JSON.parse(line.slice(6));
                if (entry.id && entry.id > currentLastId) currentLastId = entry.id;
                onEntry(entry);
              } catch { /* ignore malformed SSE frame */ }
            }
          }
        }
      } catch (e) {
        if (running && onError) onError(e);
        await new Promise(r => setTimeout(r, 3000));
      }
    }
  }

  poll();
  return () => { running = false; };
}
