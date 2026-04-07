package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type IngestHandler struct {
	eventStore        *store.EventStore
	correlationService *services.CorrelationService
}

func NewIngestHandler(es *store.EventStore, cs *services.CorrelationService) *IngestHandler {
	return &IngestHandler{
		eventStore:        es,
		correlationService: cs,
	}
}

func (h *IngestHandler) GenericWebhook(w http.ResponseWriter, r *http.Request) {
	var e models.Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 🔴 HARDENING
	if e.Service == "" {
		e.Service = "unknown-service"
	}
	if e.Severity == "" {
		e.Severity = "medium"
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

		event := models.Event{
			ID:        a.Fingerprint,
			Source:    "prometheus",
			Service:   a.Labels["service"],
			Severity:  a.Labels["severity"],
			Type:      "alert",
			Title:     a.Annotations["summary"],
			Message:   a.Annotations["description"],
			Labels:    a.Labels,
			Timestamp: ts,
		}

		if event.Service == "" {
			event.Service = "unknown-service"
		}
		if event.Severity == "" {
			event.Severity = "medium"
		}

		h.eventStore.SaveEvent(event)
		h.correlationService.ProcessEvent(event)
	}

	w.WriteHeader(http.StatusOK)
}
