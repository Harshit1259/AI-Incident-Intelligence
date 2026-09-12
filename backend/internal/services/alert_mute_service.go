// alert_mute_service.go — admin-created alert mutes.
//
// A mute stops matching alerts from creating or updating incidents for 7 days.
// Matched alerts are still stored as events and are written to the mute log,
// so nothing disappears without a trace.
//
// Mutes cover Prometheus/Grafana, OTel-metric and Zabbix alerts. Other
// sources have no reliable alert name or value, so they never match a mute
// and flow through as before.
package services

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

var (
	// ErrInvalidMute wraps every validation failure; handlers map it to 400.
	ErrInvalidMute = errors.New("invalid mute")
	// ErrMuteSourceNotFound means the alert or incident a mute refers to does
	// not exist for the caller's tenant; handlers map it to 404.
	ErrMuteSourceNotFound = errors.New("alert or incident not found")
	// ErrMuteNotActive re-exports the store error so handlers depend on one package.
	ErrMuteNotActive = store.ErrMuteNotActive
)

const (
	schemaPrometheus  = "prometheus"
	schemaOTelMetrics = "otel-metrics"
	schemaZabbix      = "zabbix"
	schemaGrafana     = "grafana"

	grafanaValueLabel = "grafana.value"
	testLabel         = "neuroops_test"

	// muteCacheTTL bounds how long a pod keeps a tenant's active mutes in
	// memory. OTel metrics can arrive many times a second; without the cache
	// every data point would add a query. Create and Unmute clear the cache.
	muteCacheTTL = 5 * time.Second

	muteReasonMaxLen   = 500
	mutePreviewWindow  = 7 * 24 * time.Hour
	mutePreviewScanCap = 20000
	muteSuggestScanCap = 5000
	muteMaxKnownDevice = 50
	muteMaxIncidentIDs = 500
	mutePreviewTopDev  = 10
)

// deviceLabelKeys are checked in order when an event has no resource field.
// Prometheus alerts carry the device in labels; OTel metrics in resource
// attributes that the normaliser merges into labels.
var deviceLabelKeys = []string{
	"instance", "host", "hostname", "host.name",
	"node", "k8s.node.name", "pod", "k8s.pod.name", "service.instance.id",
}

var environmentLabelKeys = []string{"environment", "env", "deployment.environment"}

type muteRepository interface {
	Create(m models.AlertMute) error
	ActiveMutes(tenantID string, now time.Time) ([]models.AlertMute, error)
	ListRecent(tenantID string, since time.Time) ([]models.AlertMute, error)
	Unmute(tenantID, id, userID string, now time.Time) error
	RecordMatch(e models.AlertMuteLogEntry) error
	ListLog(tenantID, muteID string, limit, offset int) ([]models.AlertMuteLogEntry, error)
	CountLogSince(tenantID string, since time.Time) (int, error)
}

type muteEventSource interface {
	GetTenantEventsByIDs(tenantID string, eventIDs []string) ([]models.Event, error)
	RecentTenantEvents(tenantID, service string, since time.Time, limit int) ([]models.Event, error)
}

type muteIncidentSource interface {
	GetIncidentByID(incidentID string) (models.Incident, bool)
}

type muteCacheEntry struct {
	mutes  []models.AlertMute
	loaded time.Time
}

type AlertMuteService struct {
	store     muteRepository
	events    muteEventSource
	incidents muteIncidentSource
	now       func() time.Time

	mu    sync.Mutex
	cache map[string]muteCacheEntry
}

func NewAlertMuteService(s *store.AlertMuteStore, es *store.EventStore, is *store.IncidentStore) *AlertMuteService {
	return newAlertMuteService(s, es, is, time.Now)
}

func newAlertMuteService(s muteRepository, es muteEventSource, is muteIncidentSource, now func() time.Time) *AlertMuteService {
	return &AlertMuteService{
		store:     s,
		events:    es,
		incidents: is,
		now:       now,
		cache:     make(map[string]muteCacheEntry),
	}
}

// ── Pipeline hook ────────────────────────────────────────────────────────────

// Check reports whether an active mute matches the event. On a match it writes
// the mute log and returns true; the caller must then stop processing the
// event. Any lookup failure returns false: an alert is never hidden because
// the mute store was unreachable.
func (s *AlertMuteService) Check(event models.Event) bool {
	if !isMuteable(event) {
		return false
	}
	mutes, err := s.activeFor(event.TenantID)
	if err != nil {
		slog.Error("alert_mute: could not load active mutes; alert not muted", "tenant_id", event.TenantID, "error", err)
		return false
	}
	now := s.now()
	for _, m := range mutes {
		if !muteMatches(m, event, now) {
			continue
		}
		_, rawValue, _ := muteValue(event)
		entry := models.AlertMuteLogEntry{
			TenantID:   event.TenantID,
			MuteID:     m.ID,
			EventID:    event.ID,
			AlertName:  muteAlertName(event),
			Device:     muteDevice(event),
			Service:    event.Service,
			Value:      rawValue,
			ReceivedAt: now,
		}
		if err := s.store.RecordMatch(entry); err != nil {
			slog.Error("alert_mute: failed to record muted alert", "mute_id", m.ID, "event_id", event.ID, "error", err)
		}
		slog.Info("alert_mute: alert muted", "mute_id", m.ID, "event_id", event.ID, "tenant_id", event.TenantID)
		return true
	}
	return false
}

func (s *AlertMuteService) activeFor(tenantID string) ([]models.AlertMute, error) {
	now := s.now()
	s.mu.Lock()
	entry, ok := s.cache[tenantID]
	s.mu.Unlock()
	if ok && now.Sub(entry.loaded) < muteCacheTTL {
		return entry.mutes, nil
	}
	mutes, err := s.store.ActiveMutes(tenantID, now)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[tenantID] = muteCacheEntry{mutes: mutes, loaded: now}
	s.mu.Unlock()
	return mutes, nil
}

func (s *AlertMuteService) invalidate(tenantID string) {
	s.mu.Lock()
	delete(s.cache, tenantID)
	s.mu.Unlock()
}

// ── Admin operations ─────────────────────────────────────────────────────────

// Create validates the request and stores a mute that ends 7 days from now.
func (s *AlertMuteService) Create(req models.AlertMuteRequest, tenantID, userID string) (*models.AlertMute, error) {
	now := s.now()
	m, err := s.buildMute(req, tenantID, now, true)
	if err != nil {
		return nil, err
	}
	m.ID = fmt.Sprintf("mute-%d", now.UnixNano())
	m.CreatedBy = userID
	if err := s.store.Create(m); err != nil {
		return nil, fmt.Errorf("save mute: %w", err)
	}
	s.invalidate(tenantID)
	slog.Info("alert_mute: mute created", "mute_id", m.ID, "tenant_id", tenantID, "created_by", userID)
	return &m, nil
}

// Preview counts how many of the tenant's alerts from the last 7 days the
// requested mute would have caught. The reason is not required here.
func (s *AlertMuteService) Preview(req models.AlertMuteRequest, tenantID string) (*models.AlertMutePreview, error) {
	now := s.now()
	m, err := s.buildMute(req, tenantID, now, false)
	if err != nil {
		return nil, err
	}
	events, err := s.events.RecentTenantEvents(tenantID, m.Service, now.Add(-mutePreviewWindow), mutePreviewScanCap)
	if err != nil {
		return nil, fmt.Errorf("load recent alerts: %w", err)
	}
	p := &models.AlertMutePreview{
		Scanned:    len(events),
		WindowDays: int(mutePreviewWindow / (24 * time.Hour)),
		ByDevice:   map[string]int{},
		Truncated:  len(events) >= mutePreviewScanCap,
	}
	for _, e := range events {
		if muteMatches(m, e, now) {
			p.Matched++
			p.ByDevice[muteDevice(e)]++
		}
	}
	p.ByDevice = topCounts(p.ByDevice, mutePreviewTopDev)
	return p, nil
}

// Suggest pre-fills the mute form from one alert (eventID) or from every alert
// in an incident (incidentID).
func (s *AlertMuteService) Suggest(tenantID, eventID, incidentID string) (*models.AlertMuteSuggestion, error) {
	var (
		src     []models.Event
		service string
	)
	switch {
	case eventID != "":
		evs, err := s.events.GetTenantEventsByIDs(tenantID, []string{eventID})
		if err != nil {
			return nil, fmt.Errorf("load alert: %w", err)
		}
		if len(evs) == 0 {
			return nil, ErrMuteSourceNotFound
		}
		src = evs
		service = evs[0].Service
	case incidentID != "":
		inc, ok := s.incidents.GetIncidentByID(incidentID)
		if !ok || inc.TenantID != tenantID {
			return nil, ErrMuteSourceNotFound
		}
		ids := inc.EventIDs
		if len(ids) > muteMaxIncidentIDs {
			ids = ids[len(ids)-muteMaxIncidentIDs:]
		}
		evs, err := s.events.GetTenantEventsByIDs(tenantID, ids)
		if err != nil {
			return nil, fmt.Errorf("load incident alerts: %w", err)
		}
		src = evs
		service = inc.Service
	default:
		return nil, fmt.Errorf("%w: event_id or incident_id is required", ErrInvalidMute)
	}

	sg := &models.AlertMuteSuggestion{
		AlertNames:   []string{},
		Devices:      []string{},
		KnownDevices: []string{},
		Service:      service,
		IncidentID:   incidentID,
	}
	names, devices, envs := newOrderedSet(), newOrderedSet(), newOrderedSet()
	for _, e := range src {
		if !isMuteable(e) {
			continue
		}
		sg.Supported = true
		names.add(muteAlertName(e))
		devices.add(muteDevice(e))
		envs.add(muteEnvironment(e))
	}
	sg.AlertNames = names.items
	sg.Devices = devices.items
	if len(envs.items) == 1 {
		sg.Environment = envs.items[0]
	}

	if len(sg.AlertNames) > 0 {
		recent, err := s.events.RecentTenantEvents(tenantID, "", s.now().Add(-mutePreviewWindow), muteSuggestScanCap)
		if err != nil {
			slog.Error("alert_mute: could not load known devices", "tenant_id", tenantID, "error", err)
		} else {
			wanted := map[string]bool{}
			for _, n := range sg.AlertNames {
				wanted[strings.ToLower(n)] = true
			}
			known := newOrderedSet()
			for _, d := range sg.Devices {
				known.add(d) // so they are skipped below
			}
			skip := len(known.items)
			for _, e := range recent {
				if isMuteable(e) && wanted[strings.ToLower(muteAlertName(e))] {
					known.add(muteDevice(e))
				}
			}
			extra := known.items[skip:]
			sort.Strings(extra)
			if len(extra) > muteMaxKnownDevice {
				extra = extra[:muteMaxKnownDevice]
			}
			sg.KnownDevices = extra
		}
	}
	return sg, nil
}

// List returns the tenant's active mutes and those that ended in the last
// 7 days, plus the counts the dashboard card shows.
func (s *AlertMuteService) List(tenantID string) (*models.AlertMuteList, error) {
	now := s.now()
	mutes, err := s.store.ListRecent(tenantID, now.Add(-models.MuteDuration))
	if err != nil {
		return nil, err
	}
	out := &models.AlertMuteList{Items: mutes}
	for i := range out.Items {
		out.Items[i].Status = out.Items[i].StatusAt(now)
		if out.Items[i].Status == models.MuteStatusActive {
			out.ActiveCount++
		}
	}
	// Active mutes first, each group newest first (the store's order).
	sort.SliceStable(out.Items, func(i, j int) bool {
		return out.Items[i].Status == models.MuteStatusActive && out.Items[j].Status != models.MuteStatusActive
	})
	if out.Muted7d, err = s.store.CountLogSince(tenantID, now.Add(-7*24*time.Hour)); err != nil {
		return nil, err
	}
	return out, nil
}

// Log returns muted alerts, newest first, optionally for one mute.
func (s *AlertMuteService) Log(tenantID, muteID string, limit, offset int) ([]models.AlertMuteLogEntry, error) {
	return s.store.ListLog(tenantID, muteID, limit, offset)
}

// Unmute ends an active mute immediately.
func (s *AlertMuteService) Unmute(tenantID, muteID, userID string) error {
	if err := s.store.Unmute(tenantID, muteID, userID, s.now()); err != nil {
		return err
	}
	s.invalidate(tenantID)
	slog.Info("alert_mute: mute ended early", "mute_id", muteID, "tenant_id", tenantID, "unmuted_by", userID)
	return nil
}

// buildMute validates a request and returns the mute it describes, active
// from now for MuteDuration. ID and CreatedBy are left for the caller.
func (s *AlertMuteService) buildMute(req models.AlertMuteRequest, tenantID string, now time.Time, requireReason bool) (models.AlertMute, error) {
	invalid := func(msg string) (models.AlertMute, error) {
		return models.AlertMute{}, fmt.Errorf("%w: %s", ErrInvalidMute, msg)
	}

	names := normalizeMuteList(req.AlertNames)
	if len(names) == 0 {
		return invalid("choose at least one alert name, or all")
	}
	devices := normalizeMuteList(req.Devices)
	if len(devices) == 0 {
		return invalid("choose at least one device, or all")
	}
	service := strings.TrimSpace(req.Service)
	environment := strings.TrimSpace(req.Environment)
	if names[0] == models.MuteAll && devices[0] == models.MuteAll && service == "" && environment == "" {
		return invalid("this would mute every alert; narrow it by alert name, device, service or environment")
	}

	for _, v := range []*float64{req.ValueMin, req.ValueMax} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return invalid("value range must be a number")
		}
	}
	if req.ValueMin != nil && req.ValueMax != nil && *req.ValueMin > *req.ValueMax {
		return invalid("value range minimum is greater than the maximum")
	}

	reason := strings.TrimSpace(req.Reason)
	if requireReason && reason == "" {
		return invalid("reason is required")
	}
	if len(reason) > muteReasonMaxLen {
		return invalid(fmt.Sprintf("reason must be %d characters or fewer", muteReasonMaxLen))
	}

	incidentID := strings.TrimSpace(req.IncidentID)
	if incidentID != "" && s.incidents != nil {
		if inc, ok := s.incidents.GetIncidentByID(incidentID); !ok || inc.TenantID != tenantID {
			return models.AlertMute{}, ErrMuteSourceNotFound
		}
	}

	return models.AlertMute{
		TenantID:    tenantID,
		AlertNames:  names,
		Devices:     devices,
		Service:     service,
		Environment: environment,
		ValueMin:    req.ValueMin,
		ValueMax:    req.ValueMax,
		Reason:      reason,
		IncidentID:  incidentID,
		CreatedAt:   now,
		EndsAt:      now.Add(models.MuteDuration),
		Status:      models.MuteStatusActive,
	}, nil
}

// ── Matching ─────────────────────────────────────────────────────────────────

// muteMatches reports whether mute m, evaluated at now, catches event e.
func muteMatches(m models.AlertMute, e models.Event, now time.Time) bool {
	if m.StatusAt(now) != models.MuteStatusActive || m.TenantID != e.TenantID || !isMuteable(e) {
		return false
	}
	if !matchMuteList(m.AlertNames, muteAlertName(e)) || !matchMuteList(m.Devices, muteDevice(e)) {
		return false
	}
	if m.Service != "" && !strings.EqualFold(m.Service, e.Service) {
		return false
	}
	if m.Environment != "" && !strings.EqualFold(m.Environment, muteEnvironment(e)) {
		return false
	}
	if m.ValueMin != nil || m.ValueMax != nil {
		v, _, ok := muteValue(e)
		if !ok {
			return false // no value: show the alert rather than guess
		}
		if m.ValueMin != nil && v < *m.ValueMin {
			return false
		}
		if m.ValueMax != nil && v > *m.ValueMax {
			return false
		}
	}
	return true
}

func matchMuteList(list []string, value string) bool {
	for _, item := range list {
		if item == models.MuteAll {
			return true
		}
	}
	if value == "" {
		return false
	}
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// muteSchema returns the ingest path an event came through. Prometheus rows
// stored before the webhook stamped ingest_schema are recognised by source.
func muteSchema(e models.Event) string {
	if e.IngestSchema == "" && strings.EqualFold(e.Source, schemaPrometheus) {
		return schemaPrometheus
	}
	return e.IngestSchema
}

// isMuteable reports whether mutes apply to the event. Test alerts never are:
// a test must show whether the whole path works.
func isMuteable(e models.Event) bool {
	if e.Labels[testLabel] == "true" {
		return false
	}
	s := muteSchema(e)
	return s == schemaPrometheus || s == schemaOTelMetrics || s == schemaZabbix || s == schemaGrafana
}

func muteAlertName(e models.Event) string {
	switch muteSchema(e) {
	case schemaPrometheus, schemaZabbix, schemaGrafana:
		return strings.TrimSpace(e.Labels["alertname"])
	case schemaOTelMetrics:
		return strings.TrimSpace(e.Labels["otel.metric.name"])
	}
	return ""
}

func muteDevice(e models.Event) string {
	if r := strings.TrimSpace(e.Resource); r != "" {
		return r
	}
	for _, k := range deviceLabelKeys {
		if v := strings.TrimSpace(e.Labels[k]); v != "" {
			return v
		}
	}
	return ""
}

func muteEnvironment(e models.Event) string {
	if env := strings.TrimSpace(e.Environment); env != "" {
		return env
	}
	for _, k := range environmentLabelKeys {
		if v := strings.TrimSpace(e.Labels[k]); v != "" {
			return v
		}
	}
	return ""
}

// muteValue returns the alert's numeric value, its raw text, and whether a
// number could be read. A trailing "%" is accepted ("85%" → 85).
func muteValue(e models.Event) (float64, string, bool) {
	var raw string
	switch muteSchema(e) {
	case schemaPrometheus:
		raw = e.Labels[models.PrometheusValueLabel]
	case schemaOTelMetrics:
		raw = e.Labels["otel.metric.value"]
	case schemaZabbix:
		raw = e.Labels["zabbix.item_value"]
	case schemaGrafana:
		raw = e.Labels[grafanaValueLabel]
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(raw, "%")), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, raw, false
	}
	return v, raw, true
}

// normalizeMuteList trims, drops blanks and case-insensitive duplicates, and
// collapses any list containing "all" to exactly ["all"].
func normalizeMuteList(in []string) []string {
	set := newOrderedSet()
	for _, v := range in {
		v = strings.TrimSpace(v)
		if strings.EqualFold(v, models.MuteAll) {
			return []string{models.MuteAll}
		}
		set.add(v)
	}
	return set.items
}

// orderedSet keeps first-seen order and ignores blanks and case-insensitive repeats.
type orderedSet struct {
	seen  map[string]bool
	items []string
}

func newOrderedSet() *orderedSet {
	return &orderedSet{seen: map[string]bool{}, items: []string{}}
}

func (o *orderedSet) add(v string) {
	key := strings.ToLower(v)
	if v == "" || o.seen[key] {
		return
	}
	o.seen[key] = true
	o.items = append(o.items, v)
}

// topCounts keeps the n largest entries of m.
func topCounts(m map[string]int, n int) map[string]int {
	if len(m) <= n {
		return m
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	out := make(map[string]int, n)
	for _, k := range keys[:n] {
		out[k] = m[k]
	}
	return out
}
