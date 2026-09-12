package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeMuteRepo struct {
	mutes       []models.AlertMute
	logged      []models.AlertMuteLogEntry
	activeCalls int
	activeErr   error
	unmuteErr   error
}

func (f *fakeMuteRepo) Create(m models.AlertMute) error { f.mutes = append(f.mutes, m); return nil }
func (f *fakeMuteRepo) ActiveMutes(tenantID string, now time.Time) ([]models.AlertMute, error) {
	f.activeCalls++
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	var out []models.AlertMute
	for _, m := range f.mutes {
		if m.TenantID == tenantID && m.StatusAt(now) == models.MuteStatusActive {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeMuteRepo) ListRecent(string, time.Time) ([]models.AlertMute, error) { return f.mutes, nil }
func (f *fakeMuteRepo) Unmute(tenantID, id, userID string, now time.Time) error {
	if f.unmuteErr != nil {
		return f.unmuteErr
	}
	for i := range f.mutes {
		if f.mutes[i].ID == id && f.mutes[i].TenantID == tenantID {
			f.mutes[i].UnmutedAt = &now
			f.mutes[i].UnmutedBy = userID
			return nil
		}
	}
	return ErrMuteNotActive
}
func (f *fakeMuteRepo) RecordMatch(e models.AlertMuteLogEntry) error {
	f.logged = append(f.logged, e)
	return nil
}
func (f *fakeMuteRepo) ListLog(string, string, int, int) ([]models.AlertMuteLogEntry, error) {
	return f.logged, nil
}
func (f *fakeMuteRepo) CountLogSince(string, time.Time) (int, error) { return len(f.logged), nil }

type fakeMuteEvents struct{ events []models.Event }

func (f *fakeMuteEvents) GetTenantEventsByIDs(tenantID string, ids []string) ([]models.Event, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []models.Event
	for _, e := range f.events {
		if e.TenantID == tenantID && want[e.ID] {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fakeMuteEvents) RecentTenantEvents(tenantID, service string, since time.Time, limit int) ([]models.Event, error) {
	var out []models.Event
	for _, e := range f.events {
		if e.TenantID == tenantID && !e.Timestamp.Before(since) &&
			(service == "" || strings.EqualFold(service, e.Service)) {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeMuteIncidents struct{ incidents map[string]models.Incident }

func (f *fakeMuteIncidents) GetIncidentByID(id string) (models.Incident, bool) {
	inc, ok := f.incidents[id]
	return inc, ok
}

// ── helpers ──────────────────────────────────────────────────────────────────

var muteNow = time.Date(2026, 9, 12, 16, 0, 0, 0, time.UTC)

func f64(v float64) *float64 { return &v }

func promAlert(name, instance, value string) models.Event {
	labels := map[string]string{"alertname": name, "instance": instance}
	if value != "" {
		labels[models.PrometheusValueLabel] = value
	}
	return models.Event{
		ID: "evt-" + name + "-" + instance, TenantID: "acme", Source: "prometheus",
		IngestSchema: "prometheus", Service: "web", Labels: labels, Timestamp: muteNow,
	}
}

func otelMetric(name, host, value string) models.Event {
	return models.Event{
		ID: "otel-" + name + "-" + host, TenantID: "acme", Source: "otel",
		IngestSchema: "otel-metrics", Service: "web", Environment: "prod", Timestamp: muteNow,
		Labels: map[string]string{"otel.metric.name": name, "otel.metric.value": value, "host.name": host},
	}
}

func activeMute(names, devices []string) models.AlertMute {
	return models.AlertMute{
		ID: "mute-1", TenantID: "acme", AlertNames: names, Devices: devices,
		Reason: "known issue", CreatedAt: muteNow.Add(-time.Hour), EndsAt: muteNow.Add(6 * 24 * time.Hour),
	}
}

// ── matching ─────────────────────────────────────────────────────────────────

func TestMuteMatches(t *testing.T) {
	cpu := []string{"HighCPU"}
	web01 := []string{"web01"}
	past := muteNow.Add(-time.Minute)

	tests := []struct {
		name  string
		mute  func() models.AlertMute
		event models.Event
		want  bool
	}{
		{"same alert and device", func() models.AlertMute { return activeMute(cpu, web01) }, promAlert("HighCPU", "web01", ""), true},
		{"alert name is case-insensitive", func() models.AlertMute { return activeMute([]string{"highcpu"}, web01) }, promAlert("HighCPU", "web01", ""), true},
		{"device not selected", func() models.AlertMute { return activeMute(cpu, web01) }, promAlert("HighCPU", "web02", ""), false},
		{"one of several devices", func() models.AlertMute { return activeMute(cpu, []string{"web01", "web02"}) }, promAlert("HighCPU", "web02", ""), true},
		{"all devices", func() models.AlertMute { return activeMute(cpu, []string{"all"}) }, promAlert("HighCPU", "db-7", ""), true},
		{"different alert name", func() models.AlertMute { return activeMute(cpu, web01) }, promAlert("DiskFull", "web01", ""), false},
		{"all alert names on one device", func() models.AlertMute { return activeMute([]string{"all"}, web01) }, promAlert("DiskFull", "web01", ""), true},
		{"alert without device never matches a device list", func() models.AlertMute { return activeMute(cpu, web01) }, promAlert("HighCPU", "", ""), false},

		{"value inside range", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "85"), true},
		{"value on lower edge", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "80"), true},
		{"value on upper edge", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "90"), true},
		{"value above range is shown", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "90.5"), false},
		{"value below range is shown", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "79"), false},
		{"only a maximum", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMax = f64(90)
			return m
		}, promAlert("HighCPU", "web01", "12"), true},
		{"percent sign accepted", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", "85%"), true},
		{"missing value with a range is shown", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, promAlert("HighCPU", "web01", ""), false},
		{"unreadable value with a range is shown", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.ValueMax = f64(90)
			return m
		}, promAlert("HighCPU", "web01", "high"), false},
		{"no range ignores value", func() models.AlertMute { return activeMute(cpu, web01) }, promAlert("HighCPU", "web01", "99"), true},

		{"service must match when set", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.Service = "checkout"
			return m
		}, promAlert("HighCPU", "web01", ""), false},
		{"environment from prometheus labels", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.Environment = "prod"
			return m
		}, func() models.Event {
			e := promAlert("HighCPU", "web01", "")
			e.Labels["env"] = "prod"
			return e
		}(), true},

		{"expired mute", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.EndsAt = muteNow
			return m
		}, promAlert("HighCPU", "web01", ""), false},
		{"unmuted mute", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.UnmutedAt = &past
			return m
		}, promAlert("HighCPU", "web01", ""), false},
		{"other tenant", func() models.AlertMute {
			m := activeMute(cpu, web01)
			m.TenantID = "globex"
			return m
		}, promAlert("HighCPU", "web01", ""), false},

		{"otel metric by metric name and host", func() models.AlertMute {
			m := activeMute([]string{"system.cpu.utilization"}, web01)
			m.ValueMin, m.ValueMax = f64(0.8), f64(0.9)
			return m
		}, otelMetric("system.cpu.utilization", "web01", "0.85"), true},
		{"grafana alert by alert name, host and value", func() models.AlertMute {
			m := activeMute([]string{"Disk almost full"}, []string{"web00"})
			m.ValueMin, m.ValueMax = f64(80), f64(90)
			return m
		}, models.Event{TenantID: "acme", Source: "grafana", IngestSchema: "grafana", Service: "Checkout", Timestamp: muteNow,
			Labels: map[string]string{"alertname": "Disk almost full", "host": "web00", "grafana.value": "84.69"}}, true},
		{"test alerts are never muted", func() models.AlertMute { return activeMute([]string{"all"}, []string{"all"}) },
			models.Event{TenantID: "acme", Source: "grafana", IngestSchema: "grafana", Timestamp: muteNow,
				Labels: map[string]string{"alertname": "TestAlert", "instance": "Grafana", "neuroops_test": "true"}}, false},
		{"generic webhook is not covered yet", func() models.AlertMute { return activeMute([]string{"all"}, []string{"all"}) },
			models.Event{TenantID: "acme", IngestSchema: "webhook", Source: "nagios", Resource: "web01",
				Labels: map[string]string{"alertname": "HighCPU"}}, false},
		{"legacy prometheus row without ingest_schema", func() models.AlertMute { return activeMute(cpu, web01) },
			func() models.Event { e := promAlert("HighCPU", "web01", ""); e.IngestSchema = ""; return e }(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := muteMatches(tt.mute(), tt.event, muteNow); got != tt.want {
				t.Errorf("muteMatches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMuteDevicePrecedence(t *testing.T) {
	e := promAlert("HighCPU", "10.0.0.5:9100", "")
	e.Labels["pod"] = "web-7d4b9c8f6d-x2k9p"
	if got := muteDevice(e); got != "10.0.0.5:9100" {
		t.Errorf("instance should win over pod, got %q", got)
	}
	e.Resource = "web01"
	if got := muteDevice(e); got != "web01" {
		t.Errorf("resource should win over labels, got %q", got)
	}
}

// ── pipeline hook ────────────────────────────────────────────────────────────

func TestCheckMutesAndLogs(t *testing.T) {
	repo := &fakeMuteRepo{mutes: []models.AlertMute{activeMute([]string{"HighCPU"}, []string{"web01"})}}
	svc := newAlertMuteService(repo, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return muteNow })

	if !svc.Check(promAlert("HighCPU", "web01", "85")) {
		t.Fatal("expected alert to be muted")
	}
	if len(repo.logged) != 1 {
		t.Fatalf("expected 1 mute-log row, got %d", len(repo.logged))
	}
	got := repo.logged[0]
	if got.MuteID != "mute-1" || got.AlertName != "HighCPU" || got.Device != "web01" || got.Value != "85" || got.TenantID != "acme" {
		t.Errorf("unexpected log entry: %+v", got)
	}

	if svc.Check(promAlert("HighCPU", "web02", "85")) {
		t.Error("alert on an unselected device must not be muted")
	}
	if len(repo.logged) != 1 {
		t.Errorf("a pass-through alert must not be logged, got %d rows", len(repo.logged))
	}
}

func TestCheckFailsOpen(t *testing.T) {
	repo := &fakeMuteRepo{activeErr: errors.New("db down")}
	svc := newAlertMuteService(repo, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return muteNow })
	if svc.Check(promAlert("HighCPU", "web01", "")) {
		t.Error("an alert must never be muted when mutes cannot be loaded")
	}
}

func TestCheckSkipsLookupForUncoveredSources(t *testing.T) {
	repo := &fakeMuteRepo{}
	svc := newAlertMuteService(repo, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return muteNow })
	svc.Check(models.Event{TenantID: "acme", IngestSchema: "webhook"})
	if repo.activeCalls != 0 {
		t.Errorf("generic webhook alerts should not query mutes, got %d calls", repo.activeCalls)
	}
}

func TestCheckCachesAndUnmuteInvalidates(t *testing.T) {
	now := muteNow
	repo := &fakeMuteRepo{mutes: []models.AlertMute{activeMute([]string{"HighCPU"}, []string{"web01"})}}
	svc := newAlertMuteService(repo, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return now })

	svc.Check(promAlert("HighCPU", "web01", ""))
	svc.Check(promAlert("HighCPU", "web01", ""))
	if repo.activeCalls != 1 {
		t.Fatalf("expected one store lookup within the cache TTL, got %d", repo.activeCalls)
	}

	if err := svc.Unmute("acme", "mute-1", "admin-1"); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	if svc.Check(promAlert("HighCPU", "web01", "")) {
		t.Error("alert muted after unmute; cache was not cleared")
	}

	now = now.Add(muteCacheTTL + time.Second)
	svc.Check(promAlert("HighCPU", "web01", ""))
	if repo.activeCalls != 3 {
		t.Errorf("expected a fresh lookup after TTL, got %d calls", repo.activeCalls)
	}
}

// ── create / validation ──────────────────────────────────────────────────────

func TestCreateValidation(t *testing.T) {
	valid := func() models.AlertMuteRequest {
		return models.AlertMuteRequest{AlertNames: []string{"HighCPU"}, Devices: []string{"web01"}, Reason: "known batch job"}
	}
	tests := []struct {
		name    string
		mutate  func(*models.AlertMuteRequest)
		wantErr string
	}{
		{"valid", func(*models.AlertMuteRequest) {}, ""},
		{"reason required", func(r *models.AlertMuteRequest) { r.Reason = "   " }, "reason is required"},
		{"device required", func(r *models.AlertMuteRequest) { r.Devices = nil }, "choose at least one device"},
		{"blank devices", func(r *models.AlertMuteRequest) { r.Devices = []string{" ", ""} }, "choose at least one device"},
		{"alert name required", func(r *models.AlertMuteRequest) { r.AlertNames = []string{} }, "choose at least one alert name"},
		{"range inverted", func(r *models.AlertMuteRequest) { r.ValueMin, r.ValueMax = f64(90), f64(80) }, "minimum is greater"},
		{"everything muted", func(r *models.AlertMuteRequest) { r.AlertNames, r.Devices = []string{"all"}, []string{"ALL"} }, "would mute every alert"},
		{"all narrowed by service", func(r *models.AlertMuteRequest) {
			r.AlertNames, r.Devices, r.Service = []string{"all"}, []string{"all"}, "web"
		}, ""},
		{"reason too long", func(r *models.AlertMuteRequest) { r.Reason = strings.Repeat("x", muteReasonMaxLen+1) }, "characters or fewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newAlertMuteService(&fakeMuteRepo{}, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return muteNow })
			req := valid()
			tt.mutate(&req)
			_, err := svc.Create(req, "acme", "admin-1")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidMute) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want ErrInvalidMute containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateEndsAfterSevenDaysAndNormalises(t *testing.T) {
	repo := &fakeMuteRepo{}
	svc := newAlertMuteService(repo, &fakeMuteEvents{}, &fakeMuteIncidents{}, func() time.Time { return muteNow })
	m, err := svc.Create(models.AlertMuteRequest{
		AlertNames: []string{" HighCPU ", "highcpu", "DiskFull"},
		Devices:    []string{"web01", "All"},
		Service:    " web ",
		Reason:     " batch job ",
	}, "acme", "admin-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !m.EndsAt.Equal(muteNow.Add(7 * 24 * time.Hour)) {
		t.Errorf("EndsAt = %v, want created + 7 days", m.EndsAt)
	}
	if len(m.AlertNames) != 2 || m.AlertNames[0] != "HighCPU" || m.AlertNames[1] != "DiskFull" {
		t.Errorf("alert names not de-duplicated: %v", m.AlertNames)
	}
	if len(m.Devices) != 1 || m.Devices[0] != models.MuteAll {
		t.Errorf("a list containing all should collapse to [all], got %v", m.Devices)
	}
	if m.Service != "web" || m.Reason != "batch job" || m.CreatedBy != "admin-1" || m.TenantID != "acme" {
		t.Errorf("unexpected mute: %+v", m)
	}
}

func TestCreateRejectsOtherTenantsIncident(t *testing.T) {
	incs := &fakeMuteIncidents{incidents: map[string]models.Incident{"INC-9": {ID: "INC-9", TenantID: "globex"}}}
	svc := newAlertMuteService(&fakeMuteRepo{}, &fakeMuteEvents{}, incs, func() time.Time { return muteNow })
	_, err := svc.Create(models.AlertMuteRequest{
		AlertNames: []string{"HighCPU"}, Devices: []string{"all"}, Reason: "x", IncidentID: "INC-9",
	}, "acme", "admin-1")
	if !errors.Is(err, ErrMuteSourceNotFound) {
		t.Fatalf("error = %v, want ErrMuteSourceNotFound", err)
	}
}

// ── preview / suggest ────────────────────────────────────────────────────────

func TestPreviewCountsMatchesByDevice(t *testing.T) {
	events := &fakeMuteEvents{events: []models.Event{
		promAlert("HighCPU", "web01", "85"),
		promAlert("HighCPU", "web01", "88"),
		promAlert("HighCPU", "web01", "95"), // outside range
		promAlert("HighCPU", "web02", "85"), // other device
		promAlert("DiskFull", "web01", "85"),
	}}
	svc := newAlertMuteService(&fakeMuteRepo{}, events, &fakeMuteIncidents{}, func() time.Time { return muteNow })
	p, err := svc.Preview(models.AlertMuteRequest{
		AlertNames: []string{"HighCPU"}, Devices: []string{"web01"}, ValueMin: f64(80), ValueMax: f64(90),
	}, "acme")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if p.Matched != 2 || p.ByDevice["web01"] != 2 || p.Scanned != 5 {
		t.Errorf("unexpected preview: %+v", p)
	}
}

func TestSuggestFromIncident(t *testing.T) {
	a := promAlert("HighCPU", "web01", "85")
	b := promAlert("HighCPU", "web02", "91")
	c := promAlert("HighCPU", "web03", "70") // same alert, not in the incident
	c.ID = "evt-other"
	g := models.Event{ID: "evt-generic", TenantID: "acme", IngestSchema: "webhook", Resource: "nagios-1"}
	events := &fakeMuteEvents{events: []models.Event{a, b, c, g}}
	incs := &fakeMuteIncidents{incidents: map[string]models.Incident{
		"INC-1": {ID: "INC-1", TenantID: "acme", Service: "web", EventIDs: []string{a.ID, b.ID, g.ID}},
	}}
	svc := newAlertMuteService(&fakeMuteRepo{}, events, incs, func() time.Time { return muteNow })

	sg, err := svc.Suggest("acme", "", "INC-1")
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if !sg.Supported || sg.Service != "web" || sg.IncidentID != "INC-1" {
		t.Errorf("unexpected suggestion: %+v", sg)
	}
	if strings.Join(sg.AlertNames, ",") != "HighCPU" {
		t.Errorf("alert names = %v", sg.AlertNames)
	}
	if len(sg.Devices) != 2 {
		t.Errorf("incident devices = %v, want web01 and web02", sg.Devices)
	}
	if strings.Join(sg.KnownDevices, ",") != "web03" {
		t.Errorf("known devices = %v, want [web03]", sg.KnownDevices)
	}

	if _, err := svc.Suggest("globex", "", "INC-1"); !errors.Is(err, ErrMuteSourceNotFound) {
		t.Errorf("other tenant's incident: error = %v, want ErrMuteSourceNotFound", err)
	}
}

func TestSuggestGenericAlertIsUnsupported(t *testing.T) {
	g := models.Event{ID: "evt-generic", TenantID: "acme", IngestSchema: "webhook", Resource: "nagios-1"}
	svc := newAlertMuteService(&fakeMuteRepo{}, &fakeMuteEvents{events: []models.Event{g}}, &fakeMuteIncidents{}, func() time.Time { return muteNow })
	sg, err := svc.Suggest("acme", "evt-generic", "")
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if sg.Supported {
		t.Error("generic webhook alerts should be reported as unsupported")
	}
}
