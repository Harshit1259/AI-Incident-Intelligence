package handlers

// Grafana Alerting ingest — POST /api/v1/ingest/grafana
//
// Grafana's webhook contact point sends the Alertmanager format plus extras:
// the evaluated values, the rule UID and links to the rule, dashboard and
// panel. Three kinds of alert get special handling (see plan.md, "Grafana"):
//   - DatasourceNoData / DatasourceError: the rule could not evaluate. That is
//     a monitoring gap, not a confirmed problem — severity "unknown" and an
//     honest title instead of the rule's own summary.
//   - TestAlert (the contact point's Test button): becomes a "[Test]" incident
//     that is marked recovered at once, so it closes itself after the
//     auto-close quiet period. Grafana never sends "resolved" for a test.
//   - everything else: a normal alert, deduplicated by Grafana's fingerprint.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/models"
)

// Labels that Grafana-specific UI and mutes read.
const (
	GrafanaValueLabel        = "grafana.value"
	GrafanaRuleURLLabel      = "grafana.rule_url"
	GrafanaDashboardURLLabel = "grafana.dashboard_url"
	GrafanaPanelURLLabel     = "grafana.panel_url"
	GrafanaRuleUIDLabel      = "grafana.rule_uid"
	GrafanaOrgIDLabel        = "grafana.org_id"
	// GrafanaGapLabel is "no_data" or "query_error" on alerts Grafana raises
	// when a rule could not evaluate.
	GrafanaGapLabel = "grafana.monitoring_gap"
	// TestLabel marks events created by a test button (ours or the tool's).
	TestLabel = "neuroops_test"

	grafanaSchema = "grafana"
)

type grafanaAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	Fingerprint  string            `json:"fingerprint"`
	GeneratorURL string            `json:"generatorURL"`
	DashboardURL string            `json:"dashboardURL"`
	PanelURL     string            `json:"panelURL"`
	RuleUID      string            `json:"ruleUID"`
	OrgID        *int64            `json:"orgId"`
	Values       map[string]any    `json:"values"`
	ValueString  string            `json:"valueString"`
}

type grafanaKind int

const (
	grafanaAlertNormal grafanaKind = iota
	grafanaAlertTest
)

// GrafanaWebhook handles POST /api/v1/ingest/grafana.
func (h *IngestHandler) GrafanaWebhook(w http.ResponseWriter, r *http.Request) {
	h.serveAlertWebhook(w, r, "/ingest/grafana", []string{"grafana"},
		func(body []byte, tenantID string, src *models.SourceConnection) (int, error) {
			name := ""
			if src != nil {
				name = src.Name
			}
			return h.ingestGrafanaPayload(body, tenantID, sourceIDOf(src), name)
		})
}

// ingestGrafanaPayload stores each alert in a Grafana webhook body and runs it
// through correlation. sourceName is only used in test-incident titles.
func (h *IngestHandler) ingestGrafanaPayload(body []byte, tenantID, sourceID, sourceName string) (int, error) {
	var payload struct {
		Alerts []grafanaAlert `json:"alerts"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(payload.Alerts) == 0 {
		return 0, errors.New("alerts array is empty or missing")
	}
	if len(payload.Alerts) > maxPrometheusAlerts {
		return 0, fmt.Errorf("too many alerts (max %d per request)", maxPrometheusAlerts)
	}

	now := time.Now()
	count := 0
	for i, a := range payload.Alerts {
		event, kind := grafanaToEvent(a, tenantID, sourceName, now, i)
		if err := h.eventStore.SaveEvent(event); err != nil {
			slog.Error("ingest: failed to save grafana event", "event_id", event.ID, "error", err)
			continue
		}
		switch {
		case kind == grafanaAlertTest:
			// Open the test incident, then mark the alert recovered at once so
			// auto-close closes it after the quiet period.
			h.correlationService.ProcessEvent(event)
			event.AlertStatus = "resolved"
			if err := h.eventStore.SaveEvent(event); err != nil {
				slog.Error("ingest: failed to resolve grafana test event", "event_id", event.ID, "error", err)
			} else {
				h.correlationService.ProcessResolved(event)
			}
		case event.AlertStatus == "resolved":
			h.correlationService.ProcessResolved(event)
		default:
			h.correlationService.ProcessEvent(event)
		}
		count++
	}

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, count)
	}
	return count, nil
}

// isGrafanaTest reports whether the alert is the one Grafana's contact point
// "Test" button sends: alertname TestAlert and no rule behind it.
func isGrafanaTest(a grafanaAlert) bool {
	return a.Labels["alertname"] == "TestAlert" && strings.TrimSpace(a.RuleUID) == ""
}

// grafanaToEvent maps one Grafana alert. now and index make test events unique.
func grafanaToEvent(a grafanaAlert, tenantID, sourceName string, now time.Time, index int) (models.Event, grafanaKind) {
	ts, _ := time.Parse(time.RFC3339, a.StartsAt)
	if ts.IsZero() {
		ts = now
	}
	status := "firing"
	if strings.EqualFold(strings.TrimSpace(a.Status), "resolved") {
		status = "resolved"
	}

	if isGrafanaTest(a) {
		return grafanaTestEvent(a, tenantID, sourceName, now, index), grafanaAlertTest
	}

	org := "0"
	if a.OrgID != nil {
		org = strconv.FormatInt(*a.OrgID, 10)
	}
	// Grafana's fingerprint is a hash of the alert's labels, stable across
	// firing, re-notify and resolved. Scope it to tenant and Grafana org.
	eventID := fmt.Sprintf("gf-%s-%d", tenantID, now.UnixNano()+int64(index))
	fingerprint := strings.TrimSpace(a.Fingerprint)
	if fingerprint != "" {
		eventID = "gf-" + tenantID + "-" + org + "-" + fingerprint
	}

	alertname := a.Labels["alertname"]
	ruleName := strings.TrimSpace(a.Labels["rulename"])
	if ruleName == "" {
		ruleName = alertname
	}

	service, derivedFrom := ingest.DeriveAlertService(a.Labels)
	if service == "" {
		service = strings.TrimSpace(a.Labels["grafana_folder"])
	}
	if service == "" {
		service = ruleName
	}
	if service == "" {
		service = "unknown-service"
	}

	severity := "medium"
	if raw := strings.TrimSpace(a.Labels["severity"]); raw != "" {
		severity = normalizeSeverityInput(raw)
	}

	title := strings.TrimSpace(a.Annotations["summary"])
	if title == "" {
		title = alertname
	}
	message := a.Annotations["description"]

	labels := make(map[string]string, len(a.Labels)+10)
	for k, v := range a.Labels {
		labels[k] = v
	}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			labels[k] = v
		}
	}

	// A rule that cannot evaluate. Its annotations are the rule's own ("Queue
	// depth high"), which would claim a problem nobody has seen.
	switch alertname {
	case "DatasourceNoData":
		severity = models.SeverityUnknown
		title = "No data: " + ruleName
		message = fmt.Sprintf("Grafana rule %q got no data from its data source, so it cannot tell whether %s is healthy. "+
			"This is a monitoring gap, not a confirmed problem.", ruleName, service)
		labels[GrafanaGapLabel] = "no_data"
	case "DatasourceError":
		severity = models.SeverityUnknown
		title = "Query error: " + ruleName
		message = fmt.Sprintf("Grafana rule %q could not query its data source, so it cannot tell whether %s is healthy. "+
			"This is a monitoring gap, not a confirmed problem.", ruleName, service)
		if e := strings.TrimSpace(a.Annotations["Error"]); e != "" {
			message += " Error: " + e
		}
		labels[GrafanaGapLabel] = "query_error"
	default:
		set(GrafanaValueLabel, grafanaValue(a))
	}

	set(models.PrometheusRunbookLabel, a.Annotations["runbook_url"])
	set(GrafanaRuleURLLabel, a.GeneratorURL)
	set(GrafanaDashboardURLLabel, a.DashboardURL)
	set(GrafanaPanelURLLabel, a.PanelURL)
	set(GrafanaRuleUIDLabel, a.RuleUID)
	if a.OrgID != nil {
		labels[GrafanaOrgIDLabel] = org
	}
	if derivedFrom != "" {
		labels[ingest.LabelServiceOriginal] = a.Labels["service"]
		labels[ingest.LabelServiceDerivedFrom] = derivedFrom
	}

	return models.Event{
		ID:           eventID,
		TenantID:     tenantID,
		Source:       "grafana",
		Service:      service,
		Severity:     severity,
		Type:         "alert",
		Title:        title,
		Message:      message,
		Labels:       labels,
		Timestamp:    ts,
		Fingerprint:  fingerprint,
		IngestSchema: grafanaSchema,
		AlertStatus:  status,
	}, grafanaAlertNormal
}

// grafanaTestEvent builds the "[Test]" event for Grafana's Test button. Every
// press gets its own event ID and fingerprint (Grafana's test fingerprint is
// constant), so a test is never deduplicated against an earlier one.
func grafanaTestEvent(a grafanaAlert, tenantID, sourceName string, now time.Time, index int) models.Event {
	unique := strconv.FormatInt(now.UnixNano()+int64(index), 10)
	title := "[Test] Grafana contact point test"
	if sourceName != "" {
		title += " — " + sourceName
	}
	labels := make(map[string]string, len(a.Labels)+1)
	for k, v := range a.Labels {
		labels[k] = v
	}
	labels[TestLabel] = "true"
	return models.Event{
		ID:       "gf-" + tenantID + "-test-" + unique,
		TenantID: tenantID,
		Source:   "grafana",
		Service:  "grafana-test",
		Severity: "info",
		Type:     "alert",
		Title:    title,
		Message: "Grafana's Test button reached NeuroOps. The alert was received, parsed and turned into this incident; " +
			"it is marked recovered and closes automatically.",
		Labels:       labels,
		Timestamp:    now,
		Fingerprint:  "grafana-test:" + tenantID + ":" + unique,
		IngestSchema: grafanaSchema,
		AlertStatus:  "firing",
	}
}

// grafanaValueEntry matches one "[ var='B' labels={…} type='reduce' value=95 ]"
// element of valueString. type is missing before Grafana 11.
var grafanaValueEntry = regexp.MustCompile(`\[ var='([^']*)'(?: metric='[^']*')? labels=\{[^}]*\}(?: type='([^']*)')? value=([^ \]]+) \]`)

// grafanaValue returns the metric value the rule fired on: the first
// expression in valueString that is not the threshold (the threshold's value
// is just 1 or 0). When valueString has no types, the first entry is used;
// when it cannot be parsed, a single entry in values is.
func grafanaValue(a grafanaAlert) string {
	matches := grafanaValueEntry.FindAllStringSubmatch(a.ValueString, -1)
	for _, m := range matches {
		if m[2] == "threshold" {
			continue
		}
		if v, err := strconv.ParseFloat(m[3], 64); err == nil {
			return formatGrafanaValue(v)
		}
	}
	if len(matches) == 0 && len(a.Values) == 1 {
		for _, raw := range a.Values {
			if v, ok := raw.(float64); ok {
				return formatGrafanaValue(v)
			}
		}
	}
	return strings.TrimSpace(a.Annotations["value"])
}

// formatGrafanaValue rounds to 2 decimals: 84.6945870473146 → "84.69".
func formatGrafanaValue(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}
