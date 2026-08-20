// mapper.go — pluggable mapper layer.
//
// Every ingest path implements Mapper. Built-in mappers are registered at
// startup; custom mappers are created from schema_mappings at request time
// (no startup registration needed — they're always loaded from the DB).
package ingest

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// Mapper converts a raw payload (JSON bytes) into zero or more IngestEvents.
// Each Mapper owns exactly one SourceType string that identifies it.
type Mapper interface {
	SourceType() string
	Map(payload []byte, tenantID string) ([]models.IngestEvent, error)
}

// MapperRegistry is a thread-safe registry of Mapper implementations.
// Built-in mappers are registered once at startup; custom mappers are
// loaded from the schema registry at request time and are not stored here.
type MapperRegistry struct {
	mu      sync.RWMutex
	mappers map[string]Mapper
}

// NewMapperRegistry creates a registry pre-loaded with all built-in mappers.
func NewMapperRegistry() *MapperRegistry {
	r := &MapperRegistry{mappers: make(map[string]Mapper)}
	r.Register(&OTLPLogsBuiltinMapper{})
	r.Register(&OTLPMetricsBuiltinMapper{})
	r.Register(&OTLPTracesBuiltinMapper{})
	r.Register(&PrometheusBuiltinMapper{})
	r.Register(&GenericBuiltinMapper{})
	return r
}

// Register adds or replaces a mapper in the registry.
func (r *MapperRegistry) Register(m Mapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappers[m.SourceType()] = m
}

// Get returns the mapper for a source type, or nil if not found.
func (r *MapperRegistry) Get(sourceType string) (Mapper, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.mappers[sourceType]
	return m, ok
}

// Deregister removes a mapper by source type (no-op if not found).
func (r *MapperRegistry) Deregister(sourceType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mappers, sourceType)
}

// BuiltinTypes returns the list of registered source type names.
func (r *MapperRegistry) BuiltinTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.mappers))
	for t := range r.mappers {
		types = append(types, t)
	}
	return types
}

// ── Built-in: OTel Logs ──────────────────────────────────────────────────────

type OTLPLogsBuiltinMapper struct{}

func (*OTLPLogsBuiltinMapper) SourceType() string { return "otel-logs" }

func (*OTLPLogsBuiltinMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var p OTLPLogsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("otel-logs: parse error: %w", err)
	}
	return NormalizeOTLPLogs(p, tenantID), nil
}

// ── Built-in: OTel Metrics ───────────────────────────────────────────────────

type OTLPMetricsBuiltinMapper struct{}

func (*OTLPMetricsBuiltinMapper) SourceType() string { return "otel-metrics" }

func (*OTLPMetricsBuiltinMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var p OTLPMetricsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("otel-metrics: parse error: %w", err)
	}
	return NormalizeOTLPMetrics(p, tenantID), nil
}

// ── Built-in: OTel Traces ────────────────────────────────────────────────────

type OTLPTracesBuiltinMapper struct{}

func (*OTLPTracesBuiltinMapper) SourceType() string { return "otel-traces" }

func (*OTLPTracesBuiltinMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var p OTLPTracesPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("otel-traces: parse error: %w", err)
	}
	return NormalizeOTLPTraces(p, tenantID), nil
}

// ── Built-in: Prometheus/AlertManager ────────────────────────────────────────

type PrometheusBuiltinMapper struct{}

func (*PrometheusBuiltinMapper) SourceType() string { return "prometheus" }

func (*PrometheusBuiltinMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var p AlertManagerPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("prometheus: parse error: %w", err)
	}
	events := NormalizePrometheus(p, tenantID)
	for i := range events {
		events[i].IngestSchema = "prometheus"
	}
	return events, nil
}

// ── Built-in: Generic webhook ────────────────────────────────────────────────

type GenericBuiltinMapper struct{}

func (*GenericBuiltinMapper) SourceType() string { return "generic" }

func (*GenericBuiltinMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("generic: parse error: %w", err)
	}
	e := NormalizeGeneric(raw, tenantID)
	e.IngestSchema = "webhook"
	return []models.IngestEvent{e}, nil
}

// ── IngestEvent → models.Event conversion helper ─────────────────────────────

// IngestEventToEvent converts an IngestEvent to a storable models.Event,
// generating a unique ID using the external ID or a nanosecond timestamp.
func IngestEventToEvent(ie models.IngestEvent) models.Event {
	id := ie.ExternalID
	if id == "" {
		id = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return ie.ToEvent(id)
}
