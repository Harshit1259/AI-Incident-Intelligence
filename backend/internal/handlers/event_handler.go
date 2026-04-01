package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type EventHandler struct {
	eventStore          *store.EventStore
	correlationService  *services.CorrelationService
}

func NewEventHandler(eventStore *store.EventStore, correlationService *services.CorrelationService) *EventHandler {
	return &EventHandler{
		eventStore:         eventStore,
		correlationService: correlationService,
	}
}

func (eventHandler *EventHandler) CreateEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event models.Event

	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		http.Error(responseWriter, "invalid request body", http.StatusBadRequest)
		return
	}

	eventHandler.eventStore.AddEvent(event)
	correlatedIncident := eventHandler.correlationService.CorrelateEvent(event)

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusCreated)

	response := map[string]interface{}{
		"event":     event,
		"incident":  correlatedIncident,
	}

	_ = json.NewEncoder(responseWriter).Encode(response)
}

func (eventHandler *EventHandler) ListEvents(responseWriter http.ResponseWriter, request *http.Request) {
	events := eventHandler.eventStore.GetEvents()

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(events)
}
