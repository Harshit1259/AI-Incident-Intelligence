package ingest

import (
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// NormalizeGeneric converts an arbitrary webhook payload into an IngestEvent.
// tenantID is resolved at the HTTP boundary (from the ingest token or JWT)
// and takes precedence over any tenant_id field in the payload body.
func NormalizeGeneric(input map[string]interface{}, tenantID string) models.IngestEvent {
	now := time.Now().UTC()

	getString := func(key string) string {
		if value, exists := input[key]; exists {
			if stringValue, ok := value.(string); ok {
				return stringValue
			}
		}
		return ""
	}

	// tenantID from the request boundary always wins; payload value is ignored.
	effectiveTenant := tenantID
	if effectiveTenant == "" {
		effectiveTenant = "default"
	}

	return models.IngestEvent{
		TenantID:    effectiveTenant,
		Source:      defaultIfEmpty(getString("source"), "generic"),
		ExternalID:  getString("external_id"),
		Service:     defaultIfEmpty(getString("service"), "unknown-service"),
		Resource:    getString("resource"),
		Environment: defaultIfEmpty(getString("environment"), "prod"),
		Severity:    normalizeSeverity(getString("severity")),
		SignalType:  defaultIfEmpty(getString("signal_type"), "alert"),
		Title:       getString("title"),
		Message:     getString("message"),
		Labels:      extractLabels(input),
		Timestamp:   parseTimeOrNow(getString("timestamp"), now),
	}
}

func defaultIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "high":
		return "critical"
	case "warning", "medium":
		return "high"
	case "low":
		return "medium"
	case "info":
		return "low"
	default:
		return "critical"
	}
}

func extractLabels(input map[string]interface{}) map[string]string {
	labels := make(map[string]string)

	rawLabels, exists := input["labels"]
	if !exists {
		return labels
	}

	labelMap, ok := rawLabels.(map[string]interface{})
	if !ok {
		return labels
	}

	for key, value := range labelMap {
		if stringValue, ok := value.(string); ok {
			labels[key] = stringValue
		}
	}

	return labels
}

func parseTimeOrNow(timestampValue string, fallback time.Time) time.Time {
	if strings.TrimSpace(timestampValue) == "" {
		return fallback
	}

	parsedTime, err := time.Parse(time.RFC3339, timestampValue)
	if err != nil {
		return fallback
	}

	return parsedTime
}
