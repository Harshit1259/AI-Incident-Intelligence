// custom_mapper.go — schema-driven mapper for custom (user-defined) source types.
//
// CustomMapper reads a SchemaMapping from the DB (via SchemaRegistry) and applies
// its FieldMap, SeverityMap, TitleTemplate, and defaults to any incoming JSON payload.
package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// CustomMapper converts an arbitrary JSON payload to an IngestEvent using a
// user-defined SchemaMapping. One CustomMapper instance per source type.
type CustomMapper struct {
	mapping models.SchemaMapping
}

// NewCustomMapper creates a CustomMapper from a SchemaMapping.
func NewCustomMapper(m models.SchemaMapping) *CustomMapper {
	return &CustomMapper{mapping: m}
}

func (c *CustomMapper) SourceType() string { return "custom:" + c.mapping.SourceType }

func (c *CustomMapper) Map(payload []byte, tenantID string) ([]models.IngestEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("custom(%s): parse error: %w", c.mapping.SourceType, err)
	}

	ie := models.IngestEvent{
		TenantID:    tenantID,
		Source:      c.mapping.SourceType,
		Timestamp:   time.Now().UTC(),
		Labels:      make(map[string]string),
		IngestSchema: "custom:" + c.mapping.SourceType,
	}

	// Apply FieldMap: raw field → canonical IngestEvent field.
	for srcField, dstField := range c.mapping.FieldMap {
		val := stringVal(raw[srcField])
		if val == "" {
			continue
		}
		switch dstField {
		case "service":
			ie.Service = val
		case "resource":
			ie.Resource = val
		case "environment":
			ie.Environment = val
		case "severity":
			ie.Severity = val
		case "signal_type":
			ie.SignalType = val
		case "title":
			ie.Title = val
		case "message":
			ie.Message = val
		case "external_id":
			ie.ExternalID = val
		case "timestamp":
			if t, err := parseFlexTime(val); err == nil {
				ie.Timestamp = t
			}
		}
	}

	// Normalize severity via SeverityMap (case-insensitive).
	if ie.Severity != "" && len(c.mapping.SeverityMap) > 0 {
		if mapped, ok := c.mapping.SeverityMap[strings.ToLower(ie.Severity)]; ok {
			ie.Severity = mapped
		}
	}

	// Apply defaults when fields are still empty.
	if ie.Severity == "" {
		ie.Severity = c.mapping.DefaultSeverity
	}
	if ie.SignalType == "" {
		ie.SignalType = c.mapping.DefaultSignalType
	}

	// Apply TitleTemplate: "$field_name" tokens are substituted with resolved values.
	if c.mapping.TitleTemplate != "" {
		ie.Title = applyTitleTemplate(c.mapping.TitleTemplate, ie, raw)
	}

	// Collect remaining raw fields as labels for visibility.
	for k, v := range raw {
		if sv := stringVal(v); sv != "" {
			ie.Labels[k] = sv
		}
	}

	return []models.IngestEvent{ie}, nil
}

// stringVal coerces an interface{} value to string. Returns "" for nil or
// types that cannot be meaningfully stringified.
func stringVal(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// applyTitleTemplate replaces $field_name tokens with resolved field values.
// Checks canonical IngestEvent fields first, then the raw payload map.
func applyTitleTemplate(tmpl string, ie models.IngestEvent, raw map[string]any) string {
	replacements := map[string]string{
		"$service":      ie.Service,
		"$resource":     ie.Resource,
		"$environment":  ie.Environment,
		"$severity":     ie.Severity,
		"$signal_type":  ie.SignalType,
		"$title":        ie.Title,
		"$message":      ie.Message,
		"$external_id":  ie.ExternalID,
	}
	result := tmpl
	for token, val := range replacements {
		result = strings.ReplaceAll(result, token, val)
	}
	// Replace any remaining $tokens from the raw payload.
	for k, v := range raw {
		token := "$" + k
		if strings.Contains(result, token) {
			result = strings.ReplaceAll(result, token, stringVal(v))
		}
	}
	return result
}

// parseFlexTime tries common timestamp formats.
func parseFlexTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time format: %q", s)
}
