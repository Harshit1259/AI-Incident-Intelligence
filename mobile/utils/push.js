// utils/push.js — Expo push notification setup
// Called once after login. Registers the device token with the backend.

import * as Notifications from 'expo-notifications';
import * as Device from 'expo-device';
import { Platform } from 'react-native';
import { registerPushToken } from '../api/client';

// Configure how notifications are presented while the app is in the foreground.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge:  true,
  }),
});

/**
 * Request push notification permissions and register the Expo push token
 * with the AIOps backend.
 *
 * Must be called on a physical device — Expo Go or simulator will not receive
 * production APNs/FCM pushes, but the token registration still succeeds.
 */
export async function registerForPushNotifications() {
  if (!Device.isDevice) {
    console.log('push: push notifications require a physical device');
    return;
  }

  // iOS requires explicit permission; Android 13+ also requires it.
  const { status: existingStatus } = await Notifications.getPermissionsAsync();
  let finalStatus = existingStatus;

  if (existingStatus !== 'granted') {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== 'granted') {
    console.warn('push: notification permission not granted');
    return;
  }

  // Android needs a notification channel.
  if (Platform.OS === 'android') {
    await Notifications.setNotificationChannelAsync('incidents', {
      name: 'Incident Alerts',
      importance: Notifications.AndroidImportance.MAX,
      vibrationPattern: [0, 250, 250, 250],
      lightColor: '#ef4444',
      sound: true,
    });
  }

  // Get the Expo push token. Expo routes this to FCM (Android) or APNs (iOS).
  const token = await Notifications.getExpoPushTokenAsync();
  if (!token?.data) return;

  // Register with the AIOps backend.
  try {
    await registerPushToken(token.data);
    console.log('push: registered', token.data);
  } catch (err) {
    console.warn('push: backend registration failed', err.message);
  }
}

/**
 * Set up notification response listeners.
 * Returns a cleanup function — call it in useEffect return.
 *
 * @param {function} onNotificationReceived  — called when a notification arrives in foreground
 * @param {function} onNotificationResponse  — called when the user taps a notification
 */
export function setupNotificationListeners(onNotificationReceived, onNotificationResponse) {
  const receivedSub = Notifications.addNotificationReceivedListener(onNotificationReceived);
  const responseSub = Notifications.addNotificationResponseReceivedListener(onNotificationResponse);

  return () => {
    receivedSub.remove();
    responseSub.remove();
  };
}
