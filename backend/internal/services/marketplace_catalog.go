package services

import "ai-incident-platform/backend/internal/models"

// integrationCatalog is the canonical integration catalog.
// It is product-defined, never tenant-specific, and changes only through code.
//
// The product currently supports five open-source observability integrations.
// Every other integration was removed for now; plan.md at the repository root
// lists each one and how to bring it back.
var integrationCatalog = []models.IntegrationEntry{
	{
		ID: "otel", Name: "OpenTelemetry", Category: models.CategoryMonitoring,
		Description: "Send logs, metrics and traces over OTLP/HTTP (protobuf or JSON) from an OpenTelemetry Collector or SDK. Metrics become alert signals and failing spans become trace errors for incident correlation.",
		AuthMethod:  models.AuthWebhook, Inbound: true, Outbound: false,
		WebhookPath:  "/api/v1/otel",
		TestStrategy: models.TestInboundWebhook, Popular: true,
		Tags:          []string{"otlp", "logs", "metrics", "traces", "cncf", "open-source"},
		ConfigFields:  []models.IntegrationConfigField{},
		SamplePayload: `{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout-api"}},{"key":"host.name","value":{"stringValue":"web01"}}]},"scopeMetrics":[{"scope":{"name":"otelcol/hostmetricsreceiver"},"metrics":[{"name":"system.cpu.utilization","unit":"1","gauge":{"dataPoints":[{"asDouble":0.93,"timeUnixNano":"1705312800000000000","attributes":[{"key":"state","value":{"stringValue":"user"}}]}]}}]}]}]}`,
		DocsHint:      "Add an otlphttp exporter with logs_endpoint, metrics_endpoint and traces_endpoint set to this URL plus /logs, /metrics and /traces, and an X-Source-Token header. Protobuf and gzip defaults work; gRPC is not supported.",
	},
	{
		ID: "prometheus", Name: "Prometheus", Category: models.CategoryMonitoring,
		Description: "Receive Alertmanager webhooks from your Prometheus stack. Supports full alert payload including labels, annotations, and grouping.",
		AuthMethod:  models.AuthWebhook, Inbound: true, Outbound: false,
		WebhookPath:  "/api/v1/ingest/prometheus",
		TestStrategy: models.TestInboundWebhook, Popular: true,
		Tags:          []string{"metrics", "alertmanager", "cncf", "open-source"},
		ConfigFields:  []models.IntegrationConfigField{},
		SamplePayload: `{"version":"4","groupKey":"{}:{alertname=\"HighErrorRate\"}","status":"firing","groupLabels":{"alertname":"HighErrorRate"},"commonLabels":{"alertname":"HighErrorRate","severity":"critical","service":"checkout-api"},"commonAnnotations":{"summary":"Error rate > 5%","description":"checkout-api error rate is 8.3%"},"alerts":[{"status":"firing","labels":{"alertname":"HighErrorRate","severity":"critical","service":"checkout-api"},"annotations":{"summary":"Error rate > 5%"},"startsAt":"2024-01-15T10:00:00Z","endsAt":"0001-01-01T00:00:00Z","generatorURL":"http://prometheus/graph?g0.expr=error_rate%3E0.05"}]}`,
		DocsHint:      "Set webhook_url in Alertmanager route config. Use the Prometheus-native endpoint for full label support.",
	},
	{
		ID: "grafana", Name: "Grafana", Category: models.CategoryMonitoring,
		Description: "Receive Grafana Alerting notifications from a webhook contact point: firing and resolved alerts with their values and links back to the rule, dashboard and panel. \"No data\" and query errors become monitoring-gap incidents, and Grafana's Test button opens a test incident that closes itself.",
		AuthMethod:  models.AuthWebhook, Inbound: true, Outbound: false,
		WebhookPath:  "/api/v1/ingest/grafana",
		TestStrategy: models.TestInboundWebhook, Popular: true,
		Tags:         []string{"dashboards", "alerting", "observability", "open-source"},
		ConfigFields: []models.IntegrationConfigField{},
		// What Grafana 13's contact point "Test" button sends.
		SamplePayload: `{"receiver":"webhook","status":"firing","alerts":[{"status":"firing","labels":{"alertname":"TestAlert","instance":"Grafana"},"annotations":{"summary":"Notification test"},"startsAt":"2026-09-12T18:21:11.01428915Z","endsAt":"0001-01-01T00:00:00Z","generatorURL":"","fingerprint":"57c6d9296de2ad39","silenceURL":"http://localhost:3000/alerting/silence/new?alertmanager=grafana&matcher=alertname%3DTestAlert&matcher=instance%3DGrafana","dashboardURL":"","panelURL":"","values":null,"valueString":"[ metric='foo' labels={instance=bar} value=10 ]"}],"groupLabels":{"alertname":"TestAlert","instance":"Grafana"},"commonLabels":{"alertname":"TestAlert","instance":"Grafana"},"commonAnnotations":{"summary":"Notification test"},"externalURL":"http://localhost:3000/","appVersion":"13.2.1","version":"1","groupKey":"webhook-57c6d9296de2ad39-1789237271","truncatedAlerts":0,"orgId":1,"title":"[FIRING:1] TestAlert Grafana ","state":"alerting","message":"**Firing**\n\nValue: [no value]\nLabels:\n - alertname = TestAlert\n - instance = Grafana\nAnnotations:\n - summary = Notification test\nSilence: http://localhost:3000/alerting/silence/new?alertmanager=grafana&matcher=alertname%3DTestAlert&matcher=instance%3DGrafana\n"}`,
		DocsHint:      "In Grafana, Alerting → Contact points → add a Webhook contact point with this URL, set Authorization Header Scheme to Bearer with your source token as Credentials, and route your notification policy to it.",
	},
	{
		ID: "jaeger", Name: "Jaeger", Category: models.CategoryAPM,
		Description: "Jaeger has no alerting of its own. Send the traces your services report to Jaeger to NeuroOps as well, through the OpenTelemetry Collector; failing spans become signals for incident correlation.",
		AuthMethod:  models.AuthWebhook, Inbound: true, Outbound: false,
		WebhookPath:  "/api/v1/otel/traces",
		TestStrategy: models.TestInboundWebhook, Popular: false,
		Tags:          []string{"tracing", "cncf", "open-source"},
		ConfigFields:  []models.IntegrationConfigField{},
		SamplePayload: `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout-api"}}]},"scopeSpans":[{"scope":{"name":"io.opentelemetry.http"},"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"POST /checkout","kind":2,"startTimeUnixNano":"1705312800000000000","endTimeUnixNano":"1705312801200000000","status":{"code":2,"message":"database connection timeout"},"attributes":[{"key":"http.response.status_code","value":{"intValue":"500"}}]}]}]}]}`,
		DocsHint:      "In the Collector pipeline that feeds Jaeger, add an otlphttp exporter with traces_endpoint set to this URL and an X-Source-Token header.",
	},
	{
		ID: "zabbix", Name: "Zabbix", Category: models.CategoryMonitoring,
		Description: "Receive Zabbix problems, recoveries and acknowledgements through the NeuroOps webhook media type. Recoveries close incidents automatically; acknowledgements appear on the incident timeline.",
		AuthMethod:  models.AuthWebhook, Inbound: true, Outbound: false,
		WebhookPath:  "/api/v1/ingest/zabbix",
		TestStrategy: models.TestInboundWebhook, Popular: false,
		Tags:          []string{"network", "infrastructure", "on-premise", "open-source"},
		ConfigFields:  []models.IntegrationConfigField{},
		SamplePayload: `{"event_id":"1234","event_value":"1","event_status":"PROBLEM","update_status":"0","trigger_id":"5678","trigger_name":"High CPU utilization","event_name":"High CPU utilization (over 90% for 5m)","severity":"4","host":"db01","host_name":"DB Server 01","host_ip":"10.0.0.5","host_group":"Linux servers","tags":[{"tag":"service","value":"billing"},{"tag":"env","value":"prod"}],"item_value":"93.4","opdata":"Current utilization: 93.4 %","event_time":"2026.09.12 10:00:00","zabbix_url":"https://zabbix.example.com/tr_events.php?triggerid=5678&eventid=1234"}`,
		DocsHint:      "Import the NeuroOps media type into Zabbix (Alerts → Media types → Import), set its url and token parameters, add it to a dedicated NeuroOps user (Zabbix skips update notifications to the user who made the update), and create a trigger action with problem, recovery and update operations.",
	},
}

// catalogByID provides O(1) lookup. Built once at package init.
var catalogByID map[string]*models.IntegrationEntry

func init() {
	catalogByID = make(map[string]*models.IntegrationEntry, len(integrationCatalog))
	for i := range integrationCatalog {
		catalogByID[integrationCatalog[i].ID] = &integrationCatalog[i]
	}
}

// getCatalogEntry returns the catalog entry for the given ID, or nil.
func getCatalogEntry(id string) (*models.IntegrationEntry, bool) {
	e, ok := catalogByID[id]
	return e, ok
}

// IntegrationSamplePayload returns the catalog sample payload for an
// integration (used by "Send test event" on a source).
func IntegrationSamplePayload(id string) (string, bool) {
	e, ok := catalogByID[id]
	if !ok || e.SamplePayload == "" || e.WebhookPath == "" {
		return "", false
	}
	return e.SamplePayload, true
}

// allCatalogEntries returns a copy of the full catalog slice.
func allCatalogEntries() []models.IntegrationEntry {
	return integrationCatalog
}
