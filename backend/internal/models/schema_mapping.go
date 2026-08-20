package models

import "time"

// SchemaMapping defines how a custom source's payload fields map to the
// canonical IngestEvent. One mapping per (tenant, source_type).
type SchemaMapping struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	SourceType string    `json:"source_type"` // unique slug, e.g. "my-custom-tool"
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`

	// FieldMap maps source JSON field names → IngestEvent field names.
	// Supported target fields: service | environment | severity | signal_type |
	//   title | message | resource | external_id | timestamp
	// Example: {"alert_level": "severity", "app_name": "service"}
	FieldMap map[string]string `json:"field_map"`

	// SeverityMap normalises source-specific severity values to our four levels.
	// Keys are the raw source values (case-insensitive match);
	// values must be one of: critical | high | medium | low.
	// Example: {"fire": "critical", "watch": "medium", "advisory": "low"}
	SeverityMap map[string]string `json:"severity_map"`

	// DefaultSignalType is used when the payload contains no signal_type field
	// and FieldMap does not map to "signal_type".
	// Values: alert | log | metric | trace | change
	DefaultSignalType string `json:"default_signal_type"`

	// DefaultSeverity is applied when no severity can be resolved.
	DefaultSeverity string `json:"default_severity"`

	// TitleTemplate is a simple "$field_name" reference for building the event title.
	// If empty, the value of "title" after FieldMap is applied is used directly.
	// Example: "$alertname on $service"  (values substituted from resolved fields)
	TitleTemplate string `json:"title_template"`

	// SamplePayload is an optional example payload for documentation purposes.
	SamplePayload string `json:"sample_payload,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SchemaMappingStats is returned by the schema registry to show usage.
type SchemaMappingStats struct {
	SourceType    string    `json:"source_type"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	EventsIngested int64    `json:"events_ingested"`
	LastIngestedAt *time.Time `json:"last_ingested_at,omitempty"`
}
