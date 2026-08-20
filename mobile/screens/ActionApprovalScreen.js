// screens/ActionApprovalScreen.js
// Approve or reject automation actions pending human-in-the-loop review.
import { useState, useEffect } from 'react';
import {
  View, Text, FlatList, TouchableOpacity,
  StyleSheet, SafeAreaView, ActivityIndicator, Alert,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { listPendingActions, approveAction, rejectAction } from '../api/client';

function ActionCard({ item, onApprove, onReject, loading }) {
  const isLoading = loading === item.id;
  return (
    <View style={styles.card}>
      <View style={styles.cardHeader}>
        <Text style={styles.actionName}>{item.action_type || item.name || 'Action'}</Text>
        <View style={styles.riskBadge}>
          <Text style={styles.riskText}>Risk: {item.risk_level || 'medium'}</Text>
        </View>
      </View>
      {!!item.description && (
        <Text style={styles.description}>{item.description}</Text>
      )}
      {!!item.command && (
        <View style={styles.commandBox}>
          <Text style={styles.commandText}>{item.command}</Text>
        </View>
      )}
      <View style={styles.cardActions}>
        <TouchableOpacity
          style={[styles.rejectBtn, isLoading && styles.btnDisabled]}
          onPress={() => onReject(item.id)}
          disabled={isLoading}
          activeOpacity={0.75}
        >
          <Text style={styles.rejectText}>{isLoading ? '…' : '✗ Reject'}</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.approveBtn, isLoading && styles.btnDisabled]}
          onPress={() => onApprove(item.id)}
          disabled={isLoading}
          activeOpacity={0.75}
        >
          <Text style={styles.approveText}>{isLoading ? '…' : '✓ Approve'}</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

export default function ActionApprovalScreen({ route, navigation }) {
  const { incidentId } = route.params;

  const [actions, setActions]   = useState([]);
  const [loading, setLoading]   = useState(true);
  const [actionLoading, setActionLoading] = useState('');
  const [error, setError]       = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const data = await listPendingActions(incidentId);
      setActions(data?.actions || data?.items || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, [incidentId]);

  async function handleApprove(actionId) {
    Alert.alert('Approve Action', 'Execute this automation action?', [
      { text: 'Cancel', style: 'cancel' },
      { text: 'Approve & Run', onPress: () => doAction(actionId, approveAction, 'approved') },
    ]);
  }

  async function handleReject(actionId) {
    setActionLoading(actionId);
    try {
      await rejectAction(actionId);
      setActions(prev => prev.filter(a => a.id !== actionId));
    } catch (err) {
      setError(err.message);
    } finally {
      setActionLoading('');
    }
  }

  async function doAction(actionId, fn, result) {
    setActionLoading(actionId);
    setError('');
    try {
      await fn(actionId);
      setActions(prev => prev.filter(a => a.id !== actionId));
    } catch (err) {
      setError(err.message);
    } finally {
      setActionLoading('');
    }
  }

  return (
    <SafeAreaView style={styles.root}>
      <StatusBar style="light" />

      <View style={styles.navBar}>
        <TouchableOpacity onPress={() => navigation.goBack()}>
          <Text style={styles.backText}>← Incident</Text>
        </TouchableOpacity>
        <Text style={styles.navTitle}>Pending Actions</Text>
        <TouchableOpacity onPress={load}>
          <Text style={styles.refreshText}>Refresh</Text>
        </TouchableOpacity>
      </View>

      {!!error && <Text style={styles.errorText}>{error}</Text>}

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color="#818cf8" size="large" />
        </View>
      ) : actions.length === 0 ? (
        <View style={styles.center}>
          <Text style={styles.emptyIcon}>✅</Text>
          <Text style={styles.emptyText}>No pending actions.</Text>
          <Text style={styles.emptySubtext}>All automation actions for this incident have been handled.</Text>
        </View>
      ) : (
        <>
          <View style={styles.countBanner}>
            <Text style={styles.countText}>
              {actions.length} action{actions.length > 1 ? 's' : ''} waiting for approval
            </Text>
          </View>
          <FlatList
            data={actions}
            keyExtractor={item => item.id}
            renderItem={({ item }) => (
              <ActionCard
                item={item}
                onApprove={handleApprove}
                onReject={handleReject}
                loading={actionLoading}
              />
            )}
            contentContainerStyle={{ padding: 16, gap: 12 }}
          />
        </>
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root:    { flex: 1, backgroundColor: '#081225' },
  center:  { flex: 1, justifyContent: 'center', alignItems: 'center', padding: 32 },
  navBar:  { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 10 },
  backText:    { color: '#818cf8', fontSize: 15 },
  navTitle:    { color: '#edf4ff', fontSize: 16, fontWeight: '600' },
  refreshText: { color: '#818cf8', fontSize: 13 },
  countBanner: { backgroundColor: 'rgba(245,158,11,0.1)', borderBottomWidth: 1, borderBottomColor: 'rgba(245,158,11,0.2)', paddingHorizontal: 16, paddingVertical: 8 },
  countText:   { color: '#f59e0b', fontSize: 13, fontWeight: '600' },
  card:    { backgroundColor: 'rgba(255,255,255,0.04)', borderRadius: 12, borderWidth: 1, borderColor: 'rgba(255,255,255,0.08)', padding: 14 },
  cardHeader: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 },
  actionName:  { color: '#edf4ff', fontSize: 15, fontWeight: '600', flex: 1 },
  riskBadge:   { backgroundColor: 'rgba(249,115,22,0.15)', borderRadius: 4, paddingHorizontal: 8, paddingVertical: 2, borderWidth: 1, borderColor: 'rgba(249,115,22,0.3)' },
  riskText:    { color: '#f97316', fontSize: 11, fontWeight: '600' },
  description: { color: '#9ca3af', fontSize: 13, lineHeight: 18, marginBottom: 10 },
  commandBox:  { backgroundColor: 'rgba(0,0,0,0.3)', borderRadius: 6, padding: 10, marginBottom: 10 },
  commandText: { color: '#86efac', fontSize: 12, fontFamily: 'monospace' },
  cardActions: { flexDirection: 'row', gap: 10, marginTop: 4 },
  rejectBtn:   { flex: 1, borderWidth: 1, borderColor: '#ef4444', borderRadius: 8, paddingVertical: 10, alignItems: 'center', backgroundColor: 'rgba(239,68,68,0.08)' },
  rejectText:  { color: '#ef4444', fontSize: 13, fontWeight: '600' },
  approveBtn:  { flex: 1, borderWidth: 1, borderColor: '#10b981', borderRadius: 8, paddingVertical: 10, alignItems: 'center', backgroundColor: 'rgba(16,185,129,0.1)' },
  approveText: { color: '#10b981', fontSize: 13, fontWeight: '600' },
  btnDisabled: { opacity: 0.4 },
  errorText:   { color: '#ef4444', fontSize: 13, marginHorizontal: 16, marginTop: 4 },
  emptyIcon:   { fontSize: 40, marginBottom: 12 },
  emptyText:   { color: '#9ca3af', fontSize: 16, fontWeight: '600', textAlign: 'center' },
  emptySubtext: { color: '#6b7280', fontSize: 13, textAlign: 'center', marginTop: 6, lineHeight: 18 },
});
