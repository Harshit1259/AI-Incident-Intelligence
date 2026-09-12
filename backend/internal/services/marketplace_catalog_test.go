package services

import (
	"encoding/json"
	"testing"

	"ai-incident-platform/backend/internal/ingest"
)

// Every catalog sample must parse with the parser its integration really uses,
// so "Send test alert" and the docs never show a payload the product rejects.
func TestCatalogSamplesParseWithRealParsers(t *testing.T) {
	registry := ingest.NewMapperRegistry()
	otlpSourceType := map[string]string{"otel": "otel-metrics", "jaeger": "otel-traces"}

	for _, e := range allCatalogEntries() {
		t.Run(e.ID, func(t *testing.T) {
			switch e.WebhookPath {
			case "":
				t.Fatalf("%s has no endpoint", e.ID)
			case "/api/v1/ingest/zabbix":
				var doc struct {
					EventID   string `json:"event_id"`
					TriggerID string `json:"trigger_id"`
					Host      string `json:"host"`
				}
				if err := json.Unmarshal([]byte(e.SamplePayload), &doc); err != nil ||
					doc.EventID == "" || doc.TriggerID == "" || doc.Host == "" {
					t.Fatalf("sample is not a NeuroOps Zabbix payload: %v", err)
				}
			case "/api/v1/ingest/grafana":
				var doc struct {
					Alerts []struct {
						Labels  map[string]string `json:"labels"`
						RuleUID string            `json:"ruleUID"`
					} `json:"alerts"`
				}
				if err := json.Unmarshal([]byte(e.SamplePayload), &doc); err != nil || len(doc.Alerts) != 1 ||
					doc.Alerts[0].Labels["alertname"] != "TestAlert" || doc.Alerts[0].RuleUID != "" {
					t.Fatalf("sample is not Grafana's contact point test payload: %v", err)
				}
			case "/api/v1/ingest/prometheus":
				var doc struct {
					Alerts []struct {
						Labels map[string]string `json:"labels"`
					} `json:"alerts"`
				}
				if err := json.Unmarshal([]byte(e.SamplePayload), &doc); err != nil || len(doc.Alerts) == 0 {
					t.Fatalf("sample is not an Alertmanager payload with alerts: %v", err)
				}
			default:
				mapper, ok := registry.Get(otlpSourceType[e.ID])
				if !ok {
					t.Fatalf("no OTLP mapper for %s", e.ID)
				}
				events, err := mapper.Map([]byte(e.SamplePayload), "acme")
				if err != nil || len(events) == 0 {
					t.Fatalf("sample produced %d events, err %v", len(events), err)
				}
			}
		})
	}
}
