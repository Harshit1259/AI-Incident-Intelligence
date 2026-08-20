// screens/IncidentListScreen.js
import { useState, useEffect, useCallback } from 'react';
import {
  View, Text, FlatList, TouchableOpacity, RefreshControl,
  StyleSheet, SafeAreaView, TextInput,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { listIncidents, logout } from '../api/client';

const SEV_COLOR = {
  critical: '#ef4444',
  high:     '#f97316',
  medium:   '#f59e0b',
  low:      '#6b7280',
};
const STATUS_COLOR = {
  open:         '#ef4444',
  acknowledged: '#f59e0b',
  resolved:     '#10b981',
};

function IncidentRow({ item, onPress }) {
  const sevColor    = SEV_COLOR[item.severity]  || '#6b7280';
  const statColor   = STATUS_COLOR[item.status] || '#6b7280';
  const started     = item.first_event_time
    ? new Date(item.first_event_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : '—';

  return (
    <TouchableOpacity style={styles.row} onPress={() => onPress(item)} activeOpacity={0.75}>
      <View style={[styles.sevBar, { backgroundColor: sevColor }]} />
      <View style={styles.rowBody}>
        <View style={styles.rowTop}>
          <Text style={styles.rowTitle} numberOfLines={2}>{item.title}</Text>
          <View style={[styles.chip, { borderColor: statColor }]}>
            <Text style={[styles.chipText, { color: statColor }]}>{item.status?.toUpperCase()}</Text>
          </View>
        </View>
        <View style={styles.rowMeta}>
          <Text style={styles.rowSvc}>{item.service}</Text>
          <Text style={styles.rowDot}>·</Text>
          <Text style={[styles.rowSev, { color: sevColor }]}>{item.severity?.toUpperCase()}</Text>
          <Text style={styles.rowDot}>·</Text>
          <Text style={styles.rowTime}>{started}</Text>
        </View>
        {!!item.root_cause_summary && (
          <Text style={styles.rowRCA} numberOfLines={1}>{item.root_cause_summary}</Text>
        )}
      </View>
    </TouchableOpacity>
  );
}

export default function IncidentListScreen({ navigation }) {
  const [incidents, setIncidents] = useState([]);
  const [loading, setLoading]     = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [filter, setFilter]       = useState('open');
  const [error, setError]         = useState('');

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true); else setLoading(true);
    setError('');
    try {
      const data = await listIncidents(filter, 1, 50);
      setIncidents(data?.items || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [filter]);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh every 30 s.
  useEffect(() => {
    const t = setInterval(() => load(true), 30_000);
    return () => clearInterval(t);
  }, [load]);

  async function handleLogout() {
    await logout();
    navigation.replace('Login');
  }

  const tabs = ['open', 'acknowledged', 'resolved', ''];

  return (
    <SafeAreaView style={styles.root}>
      <StatusBar style="light" />

      {/* Header */}
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Incidents</Text>
        <TouchableOpacity onPress={handleLogout}>
          <Text style={styles.headerAction}>Sign out</Text>
        </TouchableOpacity>
      </View>

      {/* Status filter tabs */}
      <View style={styles.tabs}>
        {[
          { key: 'open',         label: 'Open'   },
          { key: 'acknowledged', label: 'Active' },
          { key: 'resolved',     label: 'Closed' },
          { key: '',             label: 'All'    },
        ].map(tab => (
          <TouchableOpacity
            key={tab.key}
            style={[styles.tab, filter === tab.key && styles.tabActive]}
            onPress={() => setFilter(tab.key)}
          >
            <Text style={[styles.tabText, filter === tab.key && styles.tabTextActive]}>
              {tab.label}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {!!error && <Text style={styles.errorText}>{error}</Text>}

      <FlatList
        data={incidents}
        keyExtractor={item => item.id}
        renderItem={({ item }) => (
          <IncidentRow
            item={item}
            onPress={inc => navigation.navigate('IncidentDetail', { incidentId: inc.id, title: inc.service })}
          />
        )}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => load(true)}
            tintColor="#818cf8"
          />
        }
        ListEmptyComponent={
          !loading && (
            <View style={styles.empty}>
              <Text style={styles.emptyIcon}>✅</Text>
              <Text style={styles.emptyText}>
                {filter === 'open' ? 'No active incidents. All clear!' : 'No incidents found.'}
              </Text>
            </View>
          )
        }
        contentContainerStyle={incidents.length === 0 ? styles.emptyContainer : undefined}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root:         { flex: 1, backgroundColor: '#081225' },
  header:       { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: 16, paddingVertical: 12 },
  headerTitle:  { color: '#edf4ff', fontSize: 20, fontWeight: '700' },
  headerAction: { color: '#818cf8', fontSize: 14 },
  tabs:         { flexDirection: 'row', paddingHorizontal: 16, gap: 8, marginBottom: 8 },
  tab:          { paddingHorizontal: 14, paddingVertical: 6, borderRadius: 20, borderWidth: 1, borderColor: 'rgba(255,255,255,0.1)' },
  tabActive:    { backgroundColor: 'rgba(79,70,229,0.2)', borderColor: '#4f46e5' },
  tabText:      { color: '#6b7280', fontSize: 13, fontWeight: '500' },
  tabTextActive:{ color: '#818cf8' },
  row:          { flexDirection: 'row', backgroundColor: 'rgba(255,255,255,0.03)', borderBottomWidth: 1, borderBottomColor: 'rgba(255,255,255,0.06)' },
  sevBar:       { width: 4 },
  rowBody:      { flex: 1, padding: 14 },
  rowTop:       { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 },
  rowTitle:     { color: '#edf4ff', fontSize: 14, fontWeight: '600', flex: 1, lineHeight: 20 },
  chip:         { borderWidth: 1, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2 },
  chipText:     { fontSize: 10, fontWeight: '700', letterSpacing: 0.5 },
  rowMeta:      { flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: 4 },
  rowSvc:       { color: '#9ca3af', fontSize: 12 },
  rowDot:       { color: '#4b5563', fontSize: 12 },
  rowSev:       { fontSize: 11, fontWeight: '700' },
  rowTime:      { color: '#6b7280', fontSize: 11 },
  rowRCA:       { color: '#6b7280', fontSize: 12, marginTop: 4, fontStyle: 'italic' },
  errorText:    { color: '#ef4444', fontSize: 13, paddingHorizontal: 16, marginBottom: 8 },
  emptyContainer: { flex: 1, justifyContent: 'center' },
  empty:        { alignItems: 'center', padding: 48 },
  emptyIcon:    { fontSize: 40, marginBottom: 12 },
  emptyText:    { color: '#6b7280', fontSize: 15, textAlign: 'center' },
});
