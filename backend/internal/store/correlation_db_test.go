package store_test

// Runs against a real PostgreSQL when TEST_POSTGRES_DSN is set, e.g.
//
//	TEST_POSTGRES_DSN="host=localhost port=5432 user=aiops_user password=… dbname=aiops_test sslmode=disable" \
//	  go test ./internal/store/ -run DB
//
// Skipped otherwise. Use a throwaway database: migrations run against it.

import (
	"os"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

func testDB(t *testing.T) (*store.EventStore, *store.IncidentStore, *store.AutoResolveStore) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	db, err := store.NewDB(config.Config{PostgresDSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewEventStore(db), store.NewIncidentStore(db), store.NewAutoResolveStore(db)
}

// Two tenants with the same service name and the same alert fingerprint must
// never see each other's events or incidents while correlating.
func TestDBCorrelationLookupsAreTenantScoped(t *testing.T) {
	events, incidents, rules := testDB(t)
	suffix := time.Now().Format("150405.000000")
	acme, globex := "acme-"+suffix, "globex-"+suffix
	fp := "fp-" + suffix
	now := time.Now()

	if err := events.SaveEvent(models.Event{ID: "e1-" + suffix, TenantID: acme, Source: "prometheus",
		Service: "checkout", Severity: "critical", Title: "CPU", Timestamp: now, Fingerprint: fp}); err != nil {
		t.Fatal(err)
	}
	if err := incidents.AddIncident(models.Incident{ID: "inc-" + suffix, TenantID: acme, Service: "checkout",
		Severity: "critical", Status: "open", Title: "CPU", FirstEventTime: now, LastEventTime: now,
		Fingerprint: fp}); err != nil {
		t.Fatal(err)
	}
	window := now.Add(-5 * time.Minute)

	// acme sees its own rows
	if !events.FingerprintSeenInWindow(acme, fp, window, "other") {
		t.Error("acme: own fingerprint not seen")
	}
	if incidents.FindOpenIncidentByFingerprint(acme, fp) == nil {
		t.Error("acme: own incident not found by fingerprint")
	}
	if incidents.FindOpenIncidentForService(acme, "checkout", window) == nil {
		t.Error("acme: own incident not found by service")
	}
	if incidents.FindOpenIncidentForServices(acme, []string{"checkout"}, window) == nil {
		t.Error("acme: own incident not found by related services")
	}

	// globex, with the identical fingerprint and service, sees nothing
	if events.FingerprintSeenInWindow(globex, fp, window, "other") {
		t.Error("globex: acme's event counted as a duplicate — globex's alert would be dropped")
	}
	if inc := incidents.FindOpenIncidentByFingerprint(globex, fp); inc != nil {
		t.Errorf("globex: merged into acme's incident %s by fingerprint", inc.ID)
	}
	if inc := incidents.FindOpenIncidentForService(globex, "checkout", window); inc != nil {
		t.Errorf("globex: merged into acme's incident %s by service", inc.ID)
	}
	if inc := incidents.FindOpenIncidentForServices(globex, []string{"checkout"}, window); inc != nil {
		t.Errorf("globex: merged into acme's incident %s by related service", inc.ID)
	}

	// auto-resolve rules belong to one tenant
	if err := rules.Create(models.AutoResolveRule{ID: "rule-" + suffix, TenantID: acme, Name: "r",
		Service: "checkout", Action: "resolve", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rules.FindMatchingRules(acme, "checkout", "critical", "cpu"); len(got) != 1 {
		t.Errorf("acme: %d rules, want its 1", len(got))
	}
	if got, _ := rules.FindMatchingRules(globex, "checkout", "critical", "cpu"); len(got) != 0 {
		t.Errorf("globex: matched %d of acme's rules — would auto-resolve globex's incidents", len(got))
	}
}

// A recovery message must keep the value the alert fired with; the other
// labels take the recovery's version.
func TestDBResolvedKeepsFiringValue(t *testing.T) {
	events, _, _ := testDB(t)
	suffix := time.Now().Format("150405.000000")
	id, tenant := "gf-"+suffix, "acme-"+suffix
	save := func(status, value, desc string) {
		t.Helper()
		if err := events.SaveEvent(models.Event{ID: id, TenantID: tenant, Source: "grafana", Service: "checkout",
			Title: "CPU", Timestamp: time.Now(), AlertStatus: status,
			Labels: map[string]string{"grafana.value": value, "alertname": "HighCPU", "note": desc}}); err != nil {
			t.Fatal(err)
		}
	}
	get := func() models.Event {
		t.Helper()
		got, err := events.GetTenantEventsByIDs(tenant, []string{id})
		if err != nil || len(got) != 1 {
			t.Fatalf("read back: %v (%d rows)", err, len(got))
		}
		return got[0]
	}

	save("firing", "95", "fired")
	save("resolved", "50", "recovered")
	e := get()
	if e.Labels["grafana.value"] != "95" {
		t.Errorf("value after recovery = %q, want the firing value 95", e.Labels["grafana.value"])
	}
	if e.Labels["note"] != "recovered" {
		t.Errorf("other labels should come from the recovery, got note=%q", e.Labels["note"])
	}

	// firing again later carries its own new value
	save("firing", "97", "fired again")
	if v := get().Labels["grafana.value"]; v != "97" {
		t.Errorf("value after re-firing = %q, want 97", v)
	}
}
