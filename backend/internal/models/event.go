package models

import "time"

// SeverityUnknown is for alerts that cannot say how bad things are — e.g.
// Grafana's "no data" and "query error" alerts, where the check itself failed.
// It ranks below every known severity when an incident's severity is chosen,
// so a real alert on the same incident always wins.
const SeverityUnknown = "unknown"

// AlertValueLabels are the labels holding an alert's measured value, per
// source. When an alert recovers, the stored value is kept: the incident is
// about the value that fired (95% CPU), not the one it recovered at (50%).
var AlertValueLabels = []string{
	"annotation.value",  // Prometheus (value annotation)
	"grafana.value",     // Grafana
	"zabbix.item_value", // Zabbix
	"otel.metric.value", // OpenTelemetry metrics
}

// Event is the persisted, correlation-ready form of a signal from any source.
type Event struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"` // required — set at the ingest boundary
	Source      string            `json:"source"`
	ExternalID  string            `json:"external_id"`
	Service     string            `json:"service"`
	Resource    string            `json:"resource"`
	Environment string            `json:"environment"`
	Severity    string            `json:"severity"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`
	// AlertStatus is what the source reported: "firing", "resolved", or ""
	// for sources that never send a resolve (OTel, custom). Drives auto-close.
	AlertStatus string `json:"alert_status,omitempty"`
	// Fingerprint is a deterministic hash used for deduplication.
	// Identical alerts within the dedup window share the same fingerprint.
	Fingerprint string `json:"fingerprint"`

	// OTel-native fields — populated for OTLP ingest; empty for legacy sources.
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	// IngestSchema records which ingest path produced this event.
	// Values: otel-logs | otel-metrics | otel-traces | prometheus | webhook | custom:<type>
	IngestSchema string `json:"ingest_schema,omitempty"`
	// AttrsJSON is a JSON-encoded map of all OTel resource+scope+record attributes.
	// Empty for non-OTel events.
	AttrsJSON string `json:"attrs_json,omitempty"`
}
