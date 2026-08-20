// App.js — AIOps Mobile App root
// Navigation: Login → IncidentList → IncidentDetail → ActionApproval

import { useEffect, useRef } from 'react';
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import LoginScreen          from './screens/LoginScreen';
import IncidentListScreen   from './screens/IncidentListScreen';
import IncidentDetailScreen from './screens/IncidentDetailScreen';
import ActionApprovalScreen from './screens/ActionApprovalScreen';
import { setupNotificationListeners } from './utils/push';
import { getToken } from './api/client';

const Stack = createNativeStackNavigator();

const SCREEN_OPTIONS = {
  headerShown: false,
  contentStyle: { backgroundColor: '#081225' },
  animation: 'slide_from_right',
};

export default function App() {
  const navigationRef = useRef(null);

  useEffect(() => {
    // Handle notification tap → deep link to the relevant incident.
    const cleanup = setupNotificationListeners(
      () => {}, // foreground: do nothing extra — the notification appears as a banner
      (response) => {
        const data = response.notification.request.content.data;
        if (!data?.incident_id || !navigationRef.current) return;

        const screen = data.screen || 'IncidentDetail';
        navigationRef.current.navigate(screen, { incidentId: data.incident_id });
      },
    );
    return cleanup;
  }, []);

  return (
    <SafeAreaProvider>
      <NavigationContainer ref={navigationRef}>
        <Stack.Navigator screenOptions={SCREEN_OPTIONS} initialRouteName="Login">
          <Stack.Screen
            name="Login"
            component={LoginScreen}
            options={{ animation: 'fade' }}
          />
          <Stack.Screen
            name="IncidentList"
            component={IncidentListScreen}
            options={{ gestureEnabled: false }} // prevent swipe-back to login
          />
          <Stack.Screen
            name="IncidentDetail"
            component={IncidentDetailScreen}
          />
          <Stack.Screen
            name="ActionApproval"
            component={ActionApprovalScreen}
          />
        </Stack.Navigator>
      </NavigationContainer>
    </SafeAreaProvider>
  );
}
