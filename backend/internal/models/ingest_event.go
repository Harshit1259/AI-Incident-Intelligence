package models

import (
	"fmt"
	"time"
)

// IngestEvent is the canonical intermediate representation produced by every
// mapper before being converted to models.Event for the correlation pipeline.
// All ingest paths (OTel, Prometheus, custom, generic webhook) produce this struct.
type IngestEvent struct {
	TenantID    string            `json:"tenant_id"`
	Source      string            `json:"source"`
	ExternalID  string            `json:"external_id"`
	Service     string            `json:"service"`
	Resource    string            `json:"resource"`
	Environment string            `json:"environment"`
	Severity    string            `json:"severity"`
	SignalType  string            `json:"signal_type"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`

	// OTel-specific fields — empty for non-OTel sources.
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ScopeName    string `json:"scope_name,omitempty"`
	// IngestSchema records the ingestion path so the UI and correlation engine
	// can distinguish signal types.
	// Values: otel-logs | otel-metrics | otel-traces | prometheus | webhook | custom:<source_type>
	IngestSchema string `json:"ingest_schema,omitempty"`
	// AttrsJSON carries the full JSON-encoded OTel resource+scope+record attributes
	// so the Event store can persist them without re-marshalling.
	AttrsJSON    string `json:"attrs_json,omitempty"`
}

// ToEvent converts an IngestEvent to a models.Event for storage and correlation.
// The caller is responsible for generating a unique ID if ExternalID is empty.
func (ie *IngestEvent) ToEvent(id string) Event {
	if id == "" {
		id = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	tenantID := ie.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	// Fingerprint is left empty so CorrelationService.ProcessEvent computes
	// it. Copying ExternalID (or the event ID) into it made every OTel and
	// custom event its own fingerprint, so nothing from those paths deduplicated.
	return Event{
		ID:           id,
		TenantID:     tenantID,
		Source:       ie.Source,
		ExternalID:   ie.ExternalID,
		Service:      ie.Service,
		Resource:     ie.Resource,
		Environment:  ie.Environment,
		Severity:     ie.Severity,
		Type:         ie.SignalType,
		Title:        ie.Title,
		Message:      ie.Message,
		Labels:       ie.Labels,
		Timestamp:    ie.Timestamp,
		TraceID:      ie.TraceID,
		SpanID:       ie.SpanID,
		IngestSchema: ie.IngestSchema,
		AttrsJSON:    ie.AttrsJSON,
	}
}
