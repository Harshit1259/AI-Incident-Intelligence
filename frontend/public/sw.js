// sw.js — AIOps Platform Service Worker
// Responsibilities:
//   1. Cache shell assets for offline use (install/activate)
//   2. Handle incoming Web Push notifications (push event)
//   3. Handle notification click to focus/open the app (notificationclick)

const CACHE_NAME = "aiops-shell-v1";

// Assets to pre-cache on install.
const SHELL_ASSETS = ["/", "/index.html", "/manifest.json"];

// ── Install: pre-cache the app shell ─────────────────────────────────────────
self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_ASSETS))
  );
});

// ── Activate: prune old caches ────────────────────────────────────────────────
self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))
      )
    )
  );
  return self.clients.claim();
});

// ── Fetch: network-first for API, cache-first for shell ───────────────────────
self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);

  // API calls always go to network.
  if (url.pathname.startsWith("/api/")) {
    return; // browser default
  }

  // For navigation requests, serve cached index.html (SPA routing).
  if (event.request.mode === "navigate") {
    event.respondWith(
      caches.match("/index.html").then((cached) => cached || fetch(event.request))
    );
    return;
  }

  // Cache-first for static assets.
  event.respondWith(
    caches.match(event.request).then((cached) => cached || fetch(event.request))
  );
});

// ── Push: show notification from server-sent payload ─────────────────────────
self.addEventListener("push", (event) => {
  if (!event.data) return;

  let payload;
  try {
    payload = event.data.json();
  } catch {
    payload = { title: "AIOps Alert", body: event.data.text() };
  }

  const title = payload.title || "AIOps Incident Alert";
  const options = {
    body: payload.body || "A new incident requires your attention.",
    icon: "/favicon.svg",
    badge: "/favicon.svg",
    data: payload.data || {},
    tag: payload.data?.incident_id || "aiops-alert",
    renotify: true,
    requireInteraction: payload.data?.severity === "critical",
    actions: [
      { action: "view",   title: "View Incident" },
      { action: "ack",    title: "Acknowledge"   },
    ],
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

// ── Notification click: open or focus the app ────────────────────────────────
self.addEventListener("notificationclick", (event) => {
  event.notification.close();

  const incidentID = event.notification.data?.incident_id;
  const targetURL = incidentID
    ? `/?incident=${incidentID}`
    : "/";

  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      // Focus existing window if open.
      for (const client of clients) {
        if (client.url.startsWith(self.location.origin) && "focus" in client) {
          client.navigate(targetURL);
          return client.focus();
        }
      }
      // Otherwise open a new window.
      return self.clients.openWindow(targetURL);
    })
  );
});
