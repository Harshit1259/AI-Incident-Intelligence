package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type IngestHandler struct {
	eventStore         *store.EventStore
	correlationService *services.CorrelationService
}

func NewIngestHandler(es *store.EventStore, cs *services.CorrelationService) *IngestHandler {
	return &IngestHandler{
		eventStore:         es,
		correlationService: cs,
	}
}

func (h *IngestHandler) GenericWebhook(w http.ResponseWriter, r *http.Request) {
	var e models.Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Hardening — ensure required fields are populated
	if strings.TrimSpace(e.ID) == "" {
		e.ID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if e.Service == "" {
		e.Service = "unknown-service"
	}
	if e.Severity == "" {
		e.Severity = "medium"
	}
	if e.Type == "" {
		e.Type = "alert"
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	h.eventStore.SaveEvent(e)
	h.correlationService.ProcessEvent(e)

	w.WriteHeader(http.StatusOK)
}

func (h *IngestHandler) PrometheusWebhook(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Alerts []struct {
			Status      string            `json:"status"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			StartsAt    string            `json:"startsAt"`
			Fingerprint string            `json:"fingerprint"`
		} `json:"alerts"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, a := range payload.Alerts {
		ts, _ := time.Parse(time.RFC3339, a.StartsAt)

		// Use Prometheus fingerprint as the event ID (stable per alert)
		eventID := strings.TrimSpace(a.Fingerprint)
		if eventID == "" {
			eventID = fmt.Sprintf("prom-%d", time.Now().UnixNano())
		}

		if ts.IsZero() {
			ts = time.Now()
		}

		event := models.Event{
			ID:          eventID,
			Source:      "prometheus",
			Service:     a.Labels["service"],
			Severity:    a.Labels["severity"],
			Type:        "alert",
			Title:       a.Annotations["summary"],
			Message:     a.Annotations["description"],
			Labels:      a.Labels,
			Timestamp:   ts,
			Fingerprint: a.Fingerprint, // Prometheus already provides fingerprints
		}

		if event.Service == "" {
			event.Service = a.Labels["alertname"]
		}
		if event.Service == "" {
			event.Service = "unknown-service"
		}
		if event.Severity == "" {
			event.Severity = "medium"
		}
		if event.Title == "" {
			event.Title = a.Labels["alertname"]
		}

		h.eventStore.SaveEvent(event)
		h.correlationService.ProcessEvent(event)
	}

	w.WriteHeader(http.StatusOK)
}
