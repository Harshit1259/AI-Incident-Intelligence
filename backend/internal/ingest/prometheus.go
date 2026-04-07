package ingest

import (
	"time"

	"ai-incident-platform/backend/internal/models"
)

type AlertManagerPayload struct {
	Alerts []struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    string            `json:"startsAt"`
		Fingerprint string            `json:"fingerprint"`
	} `json:"alerts"`
}

func NormalizePrometheus(payload AlertManagerPayload) []models.IngestEvent {
	events := make([]models.IngestEvent, 0, len(payload.Alerts))

	for _, alert := range payload.Alerts {
		startedAt, err := time.Parse(time.RFC3339, alert.StartsAt)
		if err != nil {
			startedAt = time.Now().UTC()
		}

		service := alert.Labels["service"]
		if service == "" {
			service = alert.Labels["job"]
		}

		environment := alert.Labels["environment"]
		if environment == "" {
			environment = "prod"
		}

		message := alert.Annotations["description"]
		if message == "" {
			message = alert.Annotations["summary"]
		}

		events = append(events, models.IngestEvent{
			TenantID:    "default",
			Source:      "prometheus",
			ExternalID:  alert.Fingerprint,
			Service:     service,
			Resource:    alert.Labels["instance"],
			Environment: environment,
			Severity:    normalizeSeverity(alert.Labels["severity"]),
			SignalType:  "alert",
			Title:       alert.Annotations["summary"],
			Message:     message,
			Labels:      alert.Labels,
			Timestamp:   startedAt,
		})
	}

	return events
}
