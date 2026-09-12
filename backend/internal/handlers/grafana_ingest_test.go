package handlers

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// The fixtures in testdata/grafana are real webhook bodies captured from
// Grafana 13.2.1 (see plan.md, "Grafana").

var grafanaNow = time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

func grafanaFixture(t *testing.T, name string) []grafanaAlert {
	t.Helper()
	raw, err := os.ReadFile("testdata/grafana/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Alerts []grafanaAlert `json:"alerts"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Alerts
}

func TestGrafanaFiringAlert(t *testing.T) {
	a := grafanaFixture(t, "firing_cpu")[0]
	e, kind := grafanaToEvent(a, "acme", "prod-grafana", grafanaNow, 0)

	if kind != grafanaAlertNormal {
		t.Fatalf("kind = %v, want normal", kind)
	}
	checks := map[string][2]string{
		"id":          {e.ID, "gf-acme-1-6641ed85a308b5a5"},
		"fingerprint": {e.Fingerprint, "6641ed85a308b5a5"},
		"source":      {e.Source, "grafana"},
		"schema":      {e.IngestSchema, "grafana"},
		"service":     {e.Service, "checkout"},
		"severity":    {e.Severity, "critical"},
		"title":       {e.Title, "CPU above 90% on checkout"},
		"message":     {e.Message, "CPU is 95%"},
		"status":      {e.AlertStatus, "firing"},
		"value":       {e.Labels[GrafanaValueLabel], "95"},
		"runbook":     {e.Labels[models.PrometheusRunbookLabel], "https://runbooks.example.com/high-cpu"},
		"rule url":    {e.Labels[GrafanaRuleURLLabel], "http://localhost:3000/alerting/grafana/ffy207axtl8n4e/view?orgId=1"},
		"rule uid":    {e.Labels[GrafanaRuleUIDLabel], "ffy207axtl8n4e"},
		"org":         {e.Labels[GrafanaOrgIDLabel], "1"},
		"team label":  {e.Labels["team"], "payments"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if _, ok := e.Labels[GrafanaDashboardURLLabel]; ok {
		t.Error("empty dashboardURL must not become a label")
	}
	if e.Timestamp.Equal(grafanaNow) {
		t.Error("startsAt was not used as the timestamp")
	}
}

// Firing and resolved messages for one alert must land on the same event row,
// so auto-close sees the alert recover.
func TestGrafanaResolvedSharesEventID(t *testing.T) {
	firing, _ := grafanaToEvent(grafanaFixture(t, "firing_cpu")[0], "acme", "", grafanaNow, 0)
	resolved, _ := grafanaToEvent(grafanaFixture(t, "resolved_cpu")[0], "acme", "", grafanaNow, 0)
	if firing.ID != resolved.ID || firing.Fingerprint != resolved.Fingerprint {
		t.Errorf("firing %s/%s and resolved %s/%s differ", firing.ID, firing.Fingerprint, resolved.ID, resolved.Fingerprint)
	}
	if resolved.AlertStatus != "resolved" || resolved.Labels[GrafanaValueLabel] != "50" {
		t.Errorf("resolved event: status %q value %q", resolved.AlertStatus, resolved.Labels[GrafanaValueLabel])
	}
	other, _ := grafanaToEvent(grafanaFixture(t, "firing_cpu")[0], "globex", "", grafanaNow, 0)
	if other.ID == firing.ID {
		t.Error("two tenants must never share an event row")
	}
}

// One notification can carry several alert instances (one per series).
func TestGrafanaGroupedAlerts(t *testing.T) {
	alerts := grafanaFixture(t, "grouped_disk")
	if len(alerts) != 2 {
		t.Fatalf("fixture has %d alerts, want 2", len(alerts))
	}
	a, _ := grafanaToEvent(alerts[0], "acme", "", grafanaNow, 0)
	b, _ := grafanaToEvent(alerts[1], "acme", "", grafanaNow, 1)
	if a.ID == b.ID {
		t.Fatal("two alert instances share an event ID")
	}
	// No service label: the folder names the service; "warning" → medium.
	if a.Service != "Checkout" || a.Severity != "medium" || a.Title != "Disk usage high on web00" {
		t.Errorf("got service %q severity %q title %q", a.Service, a.Severity, a.Title)
	}
	if a.Labels[GrafanaValueLabel] != "84.69" {
		t.Errorf("value = %q, want 84.69 (the reduce result, rounded; not the threshold's 1)", a.Labels[GrafanaValueLabel])
	}
}

func TestGrafanaNoData(t *testing.T) {
	e, _ := grafanaToEvent(grafanaFixture(t, "nodata")[0], "acme", "", grafanaNow, 0)
	if e.Severity != models.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", e.Severity)
	}
	if e.Title != "No data: Queue depth" {
		t.Errorf("title = %q — the rule's own summary claims a problem nobody has seen", e.Title)
	}
	if e.Service != "orders" || e.Labels[GrafanaGapLabel] != "no_data" {
		t.Errorf("service %q gap %q", e.Service, e.Labels[GrafanaGapLabel])
	}
	if !strings.Contains(e.Message, "monitoring gap") {
		t.Errorf("message = %q", e.Message)
	}
	if _, ok := e.Labels[GrafanaValueLabel]; ok {
		t.Error("a no-data alert has no value")
	}
}

func TestGrafanaQueryError(t *testing.T) {
	a := grafanaFixture(t, "nodata")[0]
	a.Labels = map[string]string{"alertname": "DatasourceError", "rulename": "Queue depth", "grafana_folder": "Checkout"}
	a.Annotations = map[string]string{"summary": "Queue depth high", "Error": "connection refused"}
	e, _ := grafanaToEvent(a, "acme", "", grafanaNow, 0)
	if e.Severity != models.SeverityUnknown || e.Title != "Query error: Queue depth" ||
		e.Labels[GrafanaGapLabel] != "query_error" || !strings.Contains(e.Message, "connection refused") {
		t.Errorf("got severity %q title %q gap %q message %q", e.Severity, e.Title, e.Labels[GrafanaGapLabel], e.Message)
	}
	if e.Service != "Checkout" {
		t.Errorf("service = %q, want the folder", e.Service)
	}
}

func TestGrafanaTestButton(t *testing.T) {
	a := grafanaFixture(t, "test_alert")[0]
	e, kind := grafanaToEvent(a, "acme", "prod-grafana", grafanaNow, 0)
	if kind != grafanaAlertTest {
		t.Fatal("Grafana's test alert was not recognised")
	}
	if e.Title != "[Test] Grafana contact point test — prod-grafana" || e.Labels[TestLabel] != "true" ||
		e.Service != "grafana-test" || e.AlertStatus != "firing" {
		t.Errorf("got title %q test %q service %q status %q", e.Title, e.Labels[TestLabel], e.Service, e.AlertStatus)
	}
	// Grafana's test fingerprint is constant; each press must be its own event.
	again, _ := grafanaToEvent(a, "acme", "prod-grafana", grafanaNow.Add(time.Second), 0)
	if again.ID == e.ID || again.Fingerprint == e.Fingerprint || strings.Contains(e.Fingerprint, a.Fingerprint) {
		t.Errorf("repeated tests collide: %s / %s", e.Fingerprint, again.Fingerprint)
	}
	// A real rule that happens to be called TestAlert is not a test.
	a.RuleUID = "abc"
	if _, kind := grafanaToEvent(a, "acme", "", grafanaNow, 0); kind != grafanaAlertNormal {
		t.Error("an alert with a rule behind it is not the Test button")
	}
}

func TestGrafanaSeverityAndServiceFallbacks(t *testing.T) {
	base := func(labels map[string]string) grafanaAlert {
		return grafanaAlert{Status: "firing", Labels: labels, RuleUID: "r1", Fingerprint: "f1"}
	}
	tests := []struct {
		name         string
		labels       map[string]string
		service, sev string
	}{
		{"no severity label → medium", map[string]string{"alertname": "A", "service": "api"}, "api", "medium"},
		{"severity mapped", map[string]string{"alertname": "A", "service": "api", "severity": "P1"}, "api", "high"},
		{"folder when no service", map[string]string{"alertname": "A", "grafana_folder": "Payments"}, "Payments", "medium"},
		{"alert name last", map[string]string{"alertname": "A"}, "A", "medium"},
		{"exporter-named service is derived", map[string]string{"alertname": "A", "service": "kube-state-metrics",
			"namespace": "shop", "deployment": "cart"}, "shop/cart", "medium"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := grafanaToEvent(base(tt.labels), "acme", "", grafanaNow, 0)
			if e.Service != tt.service || e.Severity != tt.sev {
				t.Errorf("service %q severity %q, want %q %q", e.Service, e.Severity, tt.service, tt.sev)
			}
		})
	}
}

func TestGrafanaValue(t *testing.T) {
	tests := []struct {
		name string
		a    grafanaAlert
		want string
	}{
		{"reduce before threshold", grafanaAlert{ValueString: "[ var='B' labels={} type='reduce' value=95 ], [ var='C' labels={} type='threshold' value=1 ]"}, "95"},
		{"math expression", grafanaAlert{ValueString: "[ var='B' labels={} type='math' value=0.8712 ], [ var='C' labels={} type='threshold' value=1 ]"}, "0.87"},
		{"pre-11 format without types", grafanaAlert{ValueString: "[ var='B' labels={host=a} value=12.5 ]"}, "12.5"},
		{"classic condition with metric", grafanaAlert{ValueString: "[ var='B0' metric='cpu' labels={} value=77 ]"}, "77"},
		{"only values map, single entry", grafanaAlert{Values: map[string]any{"B": 42.0}}, "42"},
		{"ambiguous values map", grafanaAlert{Values: map[string]any{"A": 1.0, "B": 2.0}}, ""},
		{"falls back to value annotation", grafanaAlert{Annotations: map[string]string{"value": "88%"}}, "88%"},
		{"nothing", grafanaAlert{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := grafanaValue(tt.a); got != tt.want {
				t.Errorf("grafanaValue = %q, want %q", got, tt.want)
			}
		})
	}
}
