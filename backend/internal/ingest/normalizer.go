package ingest

import (
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

func NormalizeGeneric(input map[string]interface{}) models.IngestEvent {
	now := time.Now().UTC()

	getString := func(key string) string {
		if value, exists := input[key]; exists {
			if stringValue, ok := value.(string); ok {
				return stringValue
			}
		}
		return ""
	}

	return models.IngestEvent{
		TenantID:    defaultIfEmpty(getString("tenant_id"), "default"),
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
