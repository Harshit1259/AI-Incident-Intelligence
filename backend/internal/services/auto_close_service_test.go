package services

import (
	"strings"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// fakeAutoCloseRepo models one tenant's incidents and alert states in memory.
type fakeAutoCloseRepo struct {
	incidentAlerts map[string][]string // incident → event IDs
	alertStatus    map[string]string   // event ID → "firing" | "resolved" | ""
	open           map[string]bool
	pending        map[string]store.AutoClosePending
}

func newFakeAutoCloseRepo() *fakeAutoCloseRepo {
	return &fakeAutoCloseRepo{
		incidentAlerts: map[string][]string{}, alertStatus: map[string]string{},
		open: map[string]bool{}, pending: map[string]store.AutoClosePending{},
	}
}

func (f *fakeAutoCloseRepo) OpenIncidentsWithEvent(tenantID, eventID string) ([]string, error) {
	var out []string
	for inc, evs := range f.incidentAlerts {
		for _, e := range evs {
			if e == eventID && f.open[inc] {
				out = append(out, inc)
			}
		}
	}
	return out, nil
}
func (f *fakeAutoCloseRepo) AlertCounts(tenantID, incidentID string) (int, int, error) {
	total, unresolved := 0, 0
	for _, e := range f.incidentAlerts[incidentID] {
		total++
		if f.alertStatus[e] != "resolved" {
			unresolved++
		}
	}
	return total, unresolved, nil
}
func (f *fakeAutoCloseRepo) MarkRecovered(p store.AutoClosePending) error {
	f.pending[p.IncidentID] = p
	return nil
}
func (f *fakeAutoCloseRepo) ClearRecovered(id string) error { delete(f.pending, id); return nil }
func (f *fakeAutoCloseRepo) Pending(id string) (*store.AutoClosePending, error) {
	if p, ok := f.pending[id]; ok {
		return &p, nil
	}
	return nil, nil
}
func (f *fakeAutoCloseRepo) ClaimDue(now time.Time, limit int) ([]store.AutoClosePending, error) {
	var due []store.AutoClosePending
	for id, p := range f.pending {
		if !p.ClosesAt.After(now) {
			due = append(due, p)
			delete(f.pending, id)
		}
	}
	return due, nil
}
func (f *fakeAutoCloseRepo) IncidentIsOpen(tenantID, id string) (bool, error) { return f.open[id], nil }

type fakeResolver struct {
	calls []string // "incidentID|actor|note"
	repo  *fakeAutoCloseRepo
}

func (r *fakeResolver) UpdateIncidentStatusBy(id, action, actor, note string) (models.Incident, error) {
	r.calls = append(r.calls, id+"|"+actor+"|"+note)
	r.repo.open[id] = false
	return models.Incident{ID: id, Status: "resolved"}, nil
}

func setupAutoClose(t *testing.T) (*AutoCloseService, *fakeAutoCloseRepo, *fakeResolver, *time.Time) {
	t.Helper()
	repo := newFakeAutoCloseRepo()
	repo.incidentAlerts["INC-1"] = []string{"cpu-web01", "lat-web02", "pod-web03"}
	repo.open["INC-1"] = true
	for _, e := range repo.incidentAlerts["INC-1"] {
		repo.alertStatus[e] = "firing"
	}
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	res := &fakeResolver{repo: repo}
	svc := newAutoCloseService(repo, res, 5*time.Minute, func() time.Time { return now })
	return svc, repo, res, &now
}

func resolveAlert(svc *AutoCloseService, repo *fakeAutoCloseRepo, id string) {
	repo.alertStatus[id] = "resolved"
	svc.AlertResolved(models.Event{ID: id, TenantID: "acme", AlertStatus: "resolved"})
}

func TestAutoClose_WaitsForEveryAlert(t *testing.T) {
	svc, repo, res, now := setupAutoClose(t)

	resolveAlert(svc, repo, "pod-web03")
	resolveAlert(svc, repo, "cpu-web01")
	if _, ok := repo.pending["INC-1"]; ok {
		t.Fatal("quiet period started while an alert is still firing")
	}

	resolveAlert(svc, repo, "lat-web02")
	p, ok := repo.pending["INC-1"]
	if !ok {
		t.Fatal("all alerts resolved but no quiet period started")
	}
	if want := now.Add(5 * time.Minute); !p.ClosesAt.Equal(want) || p.AlertCount != 3 {
		t.Fatalf("pending = %+v, want closes at %v with 3 alerts", p, want)
	}
	if got := svc.PendingCloseAt("INC-1"); got == nil || !got.Equal(p.ClosesAt) {
		t.Errorf("PendingCloseAt = %v", got)
	}

	*now = now.Add(4 * time.Minute)
	if n := svc.CloseDue(); n != 0 || len(res.calls) != 0 {
		t.Fatal("closed before the quiet period ended")
	}

	*now = now.Add(time.Minute)
	if n := svc.CloseDue(); n != 1 {
		t.Fatalf("CloseDue closed %d, want 1", n)
	}
	call := res.calls[0]
	if !strings.HasPrefix(call, "INC-1|neuroops-auto-close|") || !strings.Contains(call, "all 3 alert(s) recovered") {
		t.Errorf("resolved with %q", call)
	}
}

func TestAutoClose_RefiringCancels(t *testing.T) {
	svc, repo, res, now := setupAutoClose(t)
	for _, e := range repo.incidentAlerts["INC-1"] {
		resolveAlert(svc, repo, e)
	}

	// cpu-web01 fires again during the quiet period: correlation merges it
	// into the incident, which calls IncidentActive.
	repo.alertStatus["cpu-web01"] = "firing"
	svc.IncidentActive("INC-1")

	*now = now.Add(10 * time.Minute)
	if svc.CloseDue() != 0 || len(res.calls) != 0 {
		t.Fatal("incident closed although an alert fired again")
	}
}

func TestAutoClose_RechecksBeforeClosing(t *testing.T) {
	svc, repo, res, now := setupAutoClose(t)
	for _, e := range repo.incidentAlerts["INC-1"] {
		resolveAlert(svc, repo, e)
	}
	// An alert flips back to firing without the cancel hook (e.g. a race).
	repo.alertStatus["lat-web02"] = "firing"

	*now = now.Add(5 * time.Minute)
	if svc.CloseDue() != 0 || len(res.calls) != 0 {
		t.Fatal("closed without re-checking alert states")
	}
}

func TestAutoClose_NeverClosesIncidentsWithUnknownAlerts(t *testing.T) {
	svc, repo, res, now := setupAutoClose(t)
	// An OTel trace error joins the incident; OTel never sends "resolved".
	repo.incidentAlerts["INC-1"] = append(repo.incidentAlerts["INC-1"], "otel-span-1")
	repo.alertStatus["otel-span-1"] = ""

	for _, e := range []string{"cpu-web01", "lat-web02", "pod-web03"} {
		resolveAlert(svc, repo, e)
	}
	*now = now.Add(time.Hour)
	if _, ok := repo.pending["INC-1"]; ok || svc.CloseDue() != 0 || len(res.calls) != 0 {
		t.Fatal("mixed-source incident must not auto-close")
	}
}

func TestAutoClose_LeavesManuallyClosedIncidents(t *testing.T) {
	svc, repo, res, now := setupAutoClose(t)
	for _, e := range repo.incidentAlerts["INC-1"] {
		resolveAlert(svc, repo, e)
	}
	repo.open["INC-1"] = false // someone resolved it by hand meanwhile

	*now = now.Add(5 * time.Minute)
	if svc.CloseDue() != 0 || len(res.calls) != 0 {
		t.Fatal("re-resolved an incident that was already closed")
	}
}

func TestAutoClose_IgnoresResolveForUnknownAlert(t *testing.T) {
	svc, repo, _, _ := setupAutoClose(t)
	svc.AlertResolved(models.Event{ID: "never-seen", TenantID: "acme", AlertStatus: "resolved"})
	if len(repo.pending) != 0 {
		t.Fatal("a resolve for an alert in no open incident must be ignored")
	}
}
