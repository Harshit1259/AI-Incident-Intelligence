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

type EventHandler struct {
	eventStore         *store.EventStore
	correlationService *services.CorrelationService
}

func NewEventHandler(eventStore *store.EventStore, correlationService *services.CorrelationService) *EventHandler {
	return &EventHandler{
		eventStore:         eventStore,
		correlationService: correlationService,
	}
}

func normalizeSeverity(severity string) string {
	normalized := strings.ToLower(strings.TrimSpace(severity))

	switch normalized {
	case "critical", "high", "medium", "low":
		return normalized
	default:
		return "medium"
	}
}

func validateEvent(event *models.Event) error {
	if strings.TrimSpace(event.Service) == "" {
		return fmt.Errorf("service is required")
	}
	if strings.TrimSpace(event.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

func enrichEvent(event *models.Event) {
	// ID generation
	if strings.TrimSpace(event.ID) == "" {
		event.ID = fmt.Sprintf("event-%d", time.Now().UnixNano())
	}

	// Timestamp normalization
	if strings.TrimSpace(event.Timestamp) == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Severity normalization
	event.Severity = normalizeSeverity(event.Severity)
}

func (eventHandler *EventHandler) CreateEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event models.Event

	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		http.Error(responseWriter, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateEvent(&event); err != nil {
		http.Error(responseWriter, err.Error(), http.StatusBadRequest)
		return
	}

	enrichEvent(&event)

	eventHandler.eventStore.AddEvent(event)
	correlatedIncident := eventHandler.correlationService.CorrelateEvent(event)

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusCreated)

	response := map[string]interface{}{
		"event":    event,
		"incident": correlatedIncident,
	}

	_ = json.NewEncoder(responseWriter).Encode(response)
}

func (eventHandler *EventHandler) ListEvents(responseWriter http.ResponseWriter, request *http.Request) {
	events := eventHandler.eventStore.GetEvents()

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(events)
}
