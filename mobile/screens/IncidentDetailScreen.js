// screens/IncidentDetailScreen.js
import { useState, useEffect } from 'react';
import {
  View, Text, ScrollView, TouchableOpacity,
  StyleSheet, SafeAreaView, ActivityIndicator, Alert,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import {
  getIncident, acknowledgeIncident, resolveIncident,
  escalateIncident, explainIncident,
} from '../api/client';

const SEV_COLOR = { critical: '#ef4444', high: '#f97316', medium: '#f59e0b', low: '#6b7280' };

function InfoRow({ label, value, valueColor }) {
  return (
    <View style={styles.infoRow}>
      <Text style={styles.infoLabel}>{label}</Text>
      <Text style={[styles.infoValue, valueColor && { color: valueColor }]}>{value || '—'}</Text>
    </View>
  );
}

function ActionButton({ label, color, onPress, disabled }) {
  return (
    <TouchableOpacity
      style={[styles.actionBtn, { borderColor: color, backgroundColor: color + '18' }, disabled && styles.actionBtnDisabled]}
      onPress={onPress}
      disabled={disabled}
      activeOpacity={0.75}
    >
      <Text style={[styles.actionBtnText, { color }]}>{label}</Text>
    </TouchableOpacity>
  );
}

export default function IncidentDetailScreen({ route, navigation }) {
  const { incidentId } = route.params;

  const [incident, setIncident]     = useState(null);
  const [loading, setLoading]       = useState(true);
  const [aiExplain, setAiExplain]   = useState(null);
  const [explainLoading, setExplainLoading] = useState(false);
  const [actionLoading, setActionLoading]   = useState('');
  const [error, setError]           = useState('');

  async function load() {
    setLoading(true);
    try {
      const data = await getIncident(incidentId);
      setIncident(data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, [incidentId]);

  async function handleAction(action, fn, confirm) {
    if (confirm) {
      Alert.alert(`Confirm ${action}`, `Are you sure you want to ${action.toLowerCase()} this incident?`, [
        { text: 'Cancel', style: 'cancel' },
        { text: action, style: action === 'Resolve' ? 'default' : 'destructive', onPress: () => doAction(action, fn) },
      ]);
      return;
    }
    doAction(action, fn);
  }

  async function doAction(label, fn) {
    setActionLoading(label);
    setError('');
    try {
      await fn(incidentId);
      await load(); // Refresh incident state
    } catch (err) {
      setError(err.message);
    } finally {
      setActionLoading('');
    }
  }

  async function handleExplain() {
    setExplainLoading(true);
    setError('');
    try {
      const data = await explainIncident(incidentId);
      setAiExplain(data.explanation || data.summary || JSON.stringify(data));
    } catch (err) {
      setError('AI explain failed: ' + err.message);
    } finally {
      setExplainLoading(false);
    }
  }

  if (loading) {
    return (
      <SafeAreaView style={[styles.root, styles.center]}>
        <ActivityIndicator color="#818cf8" size="large" />
      </SafeAreaView>
    );
  }

  if (!incident) {
    return (
      <SafeAreaView style={[styles.root, styles.center]}>
        <Text style={styles.errorText}>{error || 'Incident not found.'}</Text>
      </SafeAreaView>
    );
  }

  const sevColor  = SEV_COLOR[incident.severity] || '#6b7280';
  const isOpen    = incident.status === 'open';
  const isActive  = isOpen || incident.status === 'acknowledged';

  return (
    <SafeAreaView style={styles.root}>
      <StatusBar style="light" />

      {/* Custom nav header */}
      <View style={styles.navBar}>
        <TouchableOpacity onPress={() => navigation.goBack()} style={styles.backBtn}>
          <Text style={styles.backText}>← Back</Text>
        </TouchableOpacity>
        <TouchableOpacity
          onPress={() => navigation.navigate('ActionApproval', { incidentId })}
        >
          <Text style={styles.actionsLink}>Actions ›</Text>
        </TouchableOpacity>
      </View>

      <ScrollView contentContainerStyle={{ paddingBottom: 32 }}>
        {/* Severity banner */}
        <View style={[styles.banner, { borderLeftColor: sevColor }]}>
          <View style={styles.bannerTop}>
            <View style={[styles.sevChip, { backgroundColor: sevColor + '22', borderColor: sevColor }]}>
              <Text style={[styles.sevChipText, { color: sevColor }]}>
                {incident.severity?.toUpperCase()}
              </Text>
            </View>
            <Text style={styles.statusText}>{incident.status?.toUpperCase()}</Text>
          </View>
          <Text style={styles.incidentTitle}>{incident.title}</Text>
        </View>

        {!!error && <Text style={styles.errorText}>{error}</Text>}

        {/* Quick action row */}
        {isActive && (
          <View style={styles.actionRow}>
            {isOpen && (
              <ActionButton
                label={actionLoading === 'Acknowledge' ? '...' : 'Acknowledge'}
                color="#f59e0b"
                disabled={!!actionLoading}
                onPress={() => handleAction('Acknowledge', acknowledgeIncident)}
              />
            )}
            <ActionButton
              label={actionLoading === 'Escalate' ? '...' : 'Escalate'}
              color="#f97316"
              disabled={!!actionLoading}
              onPress={() => handleAction('Escalate', escalateIncident)}
            />
            <ActionButton
              label={actionLoading === 'Resolve' ? '...' : 'Resolve'}
              color="#10b981"
              disabled={!!actionLoading}
              onPress={() => handleAction('Resolve', resolveIncident, true)}
            />
          </View>
        )}

        {/* Details */}
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Incident Details</Text>
          <InfoRow label="Service"     value={incident.service} />
          <InfoRow label="Severity"    value={incident.severity?.toUpperCase()} valueColor={sevColor} />
          <InfoRow label="Status"      value={incident.status?.toUpperCase()} />
          <InfoRow label="Event count" value={String(incident.event_count || 0)} />
          <InfoRow label="Risk score"  value={String(incident.risk_score || 0)} />
          <InfoRow
            label="Started"
            value={incident.first_event_time ? new Date(incident.first_event_time).toLocaleString() : '—'}
          />
        </View>

        {/* Root cause */}
        {!!incident.root_cause_summary && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Root Cause</Text>
            <Text style={styles.rcaText}>{incident.root_cause_summary}</Text>
          </View>
        )}

        {/* AI Explanation */}
        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text style={styles.sectionTitle}>AI Explanation</Text>
            <TouchableOpacity
              onPress={handleExplain}
              disabled={explainLoading}
              style={styles.explainBtn}
            >
              <Text style={styles.explainBtnText}>
                {explainLoading ? 'Thinking…' : aiExplain ? 'Re-explain' : 'Explain with AI'}
              </Text>
            </TouchableOpacity>
          </View>
          {aiExplain ? (
            <Text style={styles.explainText}>{aiExplain}</Text>
          ) : (
            <Text style={styles.explainPlaceholder}>
              Tap "Explain with AI" to get a plain-English root cause analysis.
            </Text>
          )}
        </View>

        {/* What changed */}
        {!!incident.what_changed_type && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>What Changed?</Text>
            <InfoRow label="Type"    value={incident.what_changed_type} />
            <InfoRow label="Service" value={incident.what_changed_service} />
            <InfoRow label="Version" value={incident.what_changed_version} />
            {!!incident.what_changed_description && (
              <Text style={styles.rcaText}>{incident.what_changed_description}</Text>
            )}
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root:     { flex: 1, backgroundColor: '#081225' },
  center:   { justifyContent: 'center', alignItems: 'center' },
  navBar:   { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 10 },
  backBtn:  { padding: 4 },
  backText: { color: '#818cf8', fontSize: 15 },
  actionsLink: { color: '#818cf8', fontSize: 14 },
  banner:   { margin: 16, padding: 16, backgroundColor: 'rgba(255,255,255,0.04)', borderRadius: 12, borderWidth: 1, borderColor: 'rgba(255,255,255,0.08)', borderLeftWidth: 4 },
  bannerTop: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 },
  sevChip:  { paddingHorizontal: 10, paddingVertical: 3, borderRadius: 6, borderWidth: 1 },
  sevChipText: { fontSize: 11, fontWeight: '700', letterSpacing: 0.5 },
  statusText: { color: '#6b7280', fontSize: 12, fontWeight: '600' },
  incidentTitle: { color: '#edf4ff', fontSize: 16, fontWeight: '700', lineHeight: 22 },
  actionRow: { flexDirection: 'row', gap: 10, paddingHorizontal: 16, marginBottom: 8, flexWrap: 'wrap' },
  actionBtn: { flex: 1, minWidth: 90, borderWidth: 1, borderRadius: 8, paddingVertical: 10, alignItems: 'center' },
  actionBtnDisabled: { opacity: 0.4 },
  actionBtnText: { fontSize: 13, fontWeight: '600' },
  section:  { marginHorizontal: 16, marginTop: 16, padding: 14, backgroundColor: 'rgba(255,255,255,0.03)', borderRadius: 12, borderWidth: 1, borderColor: 'rgba(255,255,255,0.07)' },
  sectionHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 },
  sectionTitle: { color: '#9ca3af', fontSize: 11, fontWeight: '700', textTransform: 'uppercase', letterSpacing: 0.8, marginBottom: 10 },
  infoRow:  { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 5, borderBottomWidth: 1, borderBottomColor: 'rgba(255,255,255,0.05)' },
  infoLabel: { color: '#6b7280', fontSize: 13 },
  infoValue: { color: '#edf4ff', fontSize: 13, fontWeight: '500' },
  rcaText:  { color: '#d1d5db', fontSize: 13, lineHeight: 20 },
  explainBtn: { backgroundColor: 'rgba(79,70,229,0.2)', borderRadius: 6, paddingHorizontal: 10, paddingVertical: 4, borderWidth: 1, borderColor: '#4f46e5' },
  explainBtnText: { color: '#818cf8', fontSize: 12, fontWeight: '600' },
  explainText: { color: '#d1d5db', fontSize: 13, lineHeight: 20 },
  explainPlaceholder: { color: '#4b5563', fontSize: 13, fontStyle: 'italic' },
  errorText: { color: '#ef4444', fontSize: 13, margin: 16 },
});
