// screens/LoginScreen.js
import { useState } from 'react';
import {
  View, Text, TextInput, TouchableOpacity,
  StyleSheet, KeyboardAvoidingView, Platform, ActivityIndicator,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { login } from '../api/client';
import { registerForPushNotifications } from '../utils/push';

export default function LoginScreen({ navigation }) {
  const [email, setEmail]       = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading]   = useState(false);
  const [error, setError]       = useState('');

  async function handleLogin() {
    if (!email.trim() || !password.trim()) {
      setError('Email and password are required.');
      return;
    }
    setLoading(true);
    setError('');
    try {
      await login(email.trim(), password);
      // Register for push notifications after successful login.
      // Best-effort — failure should not block the user.
      registerForPushNotifications().catch(() => {});
      navigation.replace('IncidentList');
    } catch (err) {
      setError(err.message || 'Login failed. Check your credentials.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <KeyboardAvoidingView
      style={styles.root}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      <StatusBar style="light" />

      <View style={styles.logoRow}>
        <View style={styles.logoBox}>
          <Text style={styles.logoIcon}>⚡</Text>
        </View>
        <View>
          <Text style={styles.logoName}>AIOps Platform</Text>
          <Text style={styles.logoSub}>Incident Management</Text>
        </View>
      </View>

      <View style={styles.card}>
        <Text style={styles.title}>Sign in</Text>
        <Text style={styles.subtitle}>On-call access for your team</Text>

        {!!error && <Text style={styles.errorText}>{error}</Text>}

        <Text style={styles.label}>Email</Text>
        <TextInput
          style={styles.input}
          value={email}
          onChangeText={setEmail}
          placeholder="you@company.com"
          placeholderTextColor="#4b5563"
          autoCapitalize="none"
          keyboardType="email-address"
          autoComplete="email"
          returnKeyType="next"
        />

        <Text style={styles.label}>Password</Text>
        <TextInput
          style={styles.input}
          value={password}
          onChangeText={setPassword}
          placeholder="••••••••"
          placeholderTextColor="#4b5563"
          secureTextEntry
          returnKeyType="done"
          onSubmitEditing={handleLogin}
        />

        <TouchableOpacity
          style={[styles.button, loading && styles.buttonDisabled]}
          onPress={handleLogin}
          disabled={loading}
          activeOpacity={0.8}
        >
          {loading
            ? <ActivityIndicator color="#fff" />
            : <Text style={styles.buttonText}>Sign in</Text>
          }
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: '#081225',
    justifyContent: 'center',
    paddingHorizontal: 24,
  },
  logoRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    marginBottom: 36,
    paddingHorizontal: 4,
  },
  logoBox: {
    width: 48,
    height: 48,
    borderRadius: 14,
    backgroundColor: '#4f46e5',
    alignItems: 'center',
    justifyContent: 'center',
  },
  logoIcon:  { fontSize: 24 },
  logoName:  { color: '#edf4ff', fontSize: 18, fontWeight: '700' },
  logoSub:   { color: '#6b7280', fontSize: 12, marginTop: 1 },
  card: {
    backgroundColor: 'rgba(255,255,255,0.04)',
    borderRadius: 16,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.08)',
    padding: 24,
  },
  title:    { color: '#edf4ff', fontSize: 22, fontWeight: '700', marginBottom: 4 },
  subtitle: { color: '#6b7280', fontSize: 14, marginBottom: 20 },
  errorText: {
    color: '#ef4444',
    fontSize: 13,
    marginBottom: 12,
    backgroundColor: 'rgba(239,68,68,0.1)',
    borderRadius: 6,
    padding: 8,
  },
  label: { color: '#9ca3af', fontSize: 12, fontWeight: '600', marginBottom: 6, marginTop: 12 },
  input: {
    backgroundColor: 'rgba(255,255,255,0.06)',
    borderRadius: 10,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
    color: '#edf4ff',
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 15,
  },
  button: {
    marginTop: 24,
    backgroundColor: '#4f46e5',
    borderRadius: 10,
    paddingVertical: 14,
    alignItems: 'center',
  },
  buttonDisabled: { opacity: 0.6 },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
});
