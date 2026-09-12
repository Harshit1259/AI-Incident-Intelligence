package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// MarketplaceSampleIngester sends an integration's catalog sample payload
// through the same parser and pipeline that real traffic uses, so the
// marketplace "Send test alert" button fails when a real alert would fail.
// (It used to hand a ready-made event straight to correlation, which was
// green even for integrations whose real payloads were rejected.)
type MarketplaceSampleIngester struct {
	Ingest *IngestHandler
	OTel   *OTelHandler
	Now    func() time.Time
}

// IngestSample implements the marketplace's sample-ingester interface.
// Timestamps in the sample are moved to now so the test incident is current,
// and Alertmanager samples are marked "[Test]".
func (m *MarketplaceSampleIngester) IngestSample(tenantID, sourceID, integrationID string, payload []byte) (int, error) {
	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	switch integrationID {
	case "grafana":
		// The sample is Grafana's own test payload; it becomes a "[Test]"
		// incident that closes itself, exactly like Grafana's Test button.
		return m.Ingest.ingestGrafanaPayload(payload, tenantID, sourceID, "")
	case "prometheus":
		body, err := freshAlertmanagerSample(payload, now())
		if err != nil {
			return 0, err
		}
		return m.Ingest.ingestAlertmanagerPayload(body, tenantID, sourceID)
	case "otel", "jaeger":
		body, err := freshOTLPSample(payload, now())
		if err != nil {
			return 0, err
		}
		sourceType := "otel-metrics"
		if integrationID == "jaeger" {
			sourceType = "otel-traces"
		}
		return m.OTel.mapAndProcess(tenantID, sourceType, body)
	case "zabbix":
		body, err := freshZabbixSample(payload, now())
		if err != nil {
			return 0, err
		}
		if _, err := m.Ingest.ingestZabbixPayload(body, tenantID, sourceID); err != nil {
			return 0, err
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("test alerts are not available yet for %q", integrationID)
	}
}

// freshZabbixSample gives the sample a unique event ID (so repeated tests are
// separate problems) and marks it as a test.
func freshZabbixSample(payload []byte, now time.Time) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("sample payload is not valid JSON: %w", err)
	}
	doc["event_id"] = "test-" + strconv.FormatInt(now.UnixNano(), 10)
	if name, _ := doc["event_name"].(string); name != "" {
		doc["event_name"] = "[Test] " + name
	}
	return json.Marshal(doc)
}

// freshAlertmanagerSample sets every alert's startsAt to now, marks it as a
// test, and drops the fixed fingerprint so repeated tests are separate events.
func freshAlertmanagerSample(payload []byte, now time.Time) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("sample payload is not valid JSON: %w", err)
	}
	alerts, _ := doc["alerts"].([]any)
	for _, a := range alerts {
		alert, ok := a.(map[string]any)
		if !ok {
			continue
		}
		alert["startsAt"] = now.UTC().Format(time.RFC3339)
		alert["endsAt"] = "0001-01-01T00:00:00Z"
		delete(alert, "fingerprint")
		labels, _ := alert["labels"].(map[string]any)
		if labels == nil {
			labels = map[string]any{}
			alert["labels"] = labels
		}
		labels["neuroops_test"] = "true"
		annotations, _ := alert["annotations"].(map[string]any)
		if annotations == nil {
			annotations = map[string]any{}
			alert["annotations"] = annotations
		}
		if summary, _ := annotations["summary"].(string); summary != "" {
			annotations["summary"] = "[Test] " + summary
		}
	}
	return json.Marshal(doc)
}

// otlpTimeKeys are the OTLP JSON fields holding Unix-nanosecond timestamps.
var otlpTimeKeys = map[string]bool{
	"timeUnixNano": true, "observedTimeUnixNano": true,
	"startTimeUnixNano": true, "endTimeUnixNano": true,
}

// freshOTLPSample rewrites every OTLP timestamp in the sample to now.
func freshOTLPSample(payload []byte, now time.Time) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("sample payload is not valid JSON: %w", err)
	}
	ns := strconv.FormatInt(now.UnixNano(), 10)
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				if otlpTimeKeys[k] {
					t[k] = ns
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(doc)
	return json.Marshal(doc)
}
