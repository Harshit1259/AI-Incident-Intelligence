package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type EventHandler struct {
	eventStore *store.EventStore
}

func NewEventHandler(eventStore *store.EventStore) *EventHandler {
	return &EventHandler{
		eventStore: eventStore,
	}
}

func (eventHandler *EventHandler) CreateEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event models.Event

	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		http.Error(responseWriter, "invalid request body", http.StatusBadRequest)
		return
	}

	eventHandler.eventStore.AddEvent(event)

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(responseWriter).Encode(event)
}

func (eventHandler *EventHandler) ListEvents(responseWriter http.ResponseWriter, request *http.Request) {
	events := eventHandler.eventStore.GetEvents()

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(events)
}
