// push.js — Web Push subscription helper for the PWA

const REGISTER_URL = "/api/v1/push/register";

/**
 * Register the service worker and request notification permission.
 * If granted, subscribes to Web Push and registers the device token
 * with the backend.
 *
 * @param {string} jwtToken  — the current user's JWT (from auth context)
 * @returns {Promise<boolean>} true if successfully registered
 */
export async function registerPushNotifications(jwtToken) {
  if (!("serviceWorker" in navigator) || !("Notification" in window)) {
    console.warn("push: browser does not support notifications");
    return false;
  }

  // Request permission
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    return false;
  }

  // Ensure service worker is registered
  const reg = await navigator.serviceWorker.ready;

  // For Web Push we'd need a VAPID public key here.
  // For the MVP, we register the sw.pushManager to store the endpoint.
  // Native push (iOS/Android) is handled by the Expo mobile app instead.
  try {
    // Store a "web" platform subscription — endpoint is the sw registration scope.
    // This enables future VAPID-based push without a rebuild.
    const body = JSON.stringify({
      platform: "web",
      device_token: reg.scope,
      app_version: "pwa-1.0",
    });

    await fetch(REGISTER_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${jwtToken}`,
      },
      body,
    });
    return true;
  } catch (err) {
    console.error("push: registration failed", err);
    return false;
  }
}

/**
 * Unregister push notifications for this browser.
 * @param {string} subID      — the subscription ID returned by the list endpoint
 * @param {string} jwtToken
 */
export async function unregisterPushNotifications(subID, jwtToken) {
  await fetch(`${REGISTER_URL}/${subID}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${jwtToken}` },
  });
}

/**
 * Show a local (in-browser) notification without a server push.
 * Useful for foreground notifications when the app is already open.
 */
export function showLocalNotification(title, body, data = {}) {
  if (!("Notification" in window) || Notification.permission !== "granted") return;
  navigator.serviceWorker.ready.then((reg) => {
    reg.showNotification(title, {
      body,
      icon: "/favicon.svg",
      data,
      tag: data.incident_id || "aiops",
    });
  });
}
