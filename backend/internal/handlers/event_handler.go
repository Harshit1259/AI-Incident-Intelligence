package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

const duplicateEventWindow = 30 * time.Second

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
	normalizedSeverity := strings.ToLower(strings.TrimSpace(severity))

	switch normalizedSeverity {
	case "critical", "high", "medium", "low":
		return normalizedSeverity
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
	if strings.TrimSpace(event.ID) == "" {
		event.ID = fmt.Sprintf("event-%d", time.Now().UnixNano())
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	event.Severity = normalizeSeverity(event.Severity)
	event.Service = strings.TrimSpace(event.Service)
	event.Message = strings.TrimSpace(event.Message)
	event.Type = strings.TrimSpace(event.Type)
	event.Source = strings.TrimSpace(event.Source)
}

func (eventHandler *EventHandler) CreateEvent(responseWriter http.ResponseWriter, request *http.Request) {
	var event models.Event

	if err := json.NewDecoder(request.Body).Decode(&event); err != nil {
		api.WriteError(responseWriter, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateEvent(&event); err != nil {
		api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
		return
	}

	enrichEvent(&event)

	duplicateEvent, duplicateFound, err := eventHandler.eventStore.FindRecentDuplicate(event, duplicateEventWindow)
	if err != nil {
		log.Printf("duplicate check failed: %v", err)
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to process event")
		return
	}

	if duplicateFound {
		eventHandler.correlationService.ProcessEvent(duplicateEvent)

		api.WriteJSON(responseWriter, http.StatusOK, map[string]interface{}{
			"event":        duplicateEvent,
			"duplicate":    true,
			"duplicate_of": duplicateEvent.ID,
		})
		return
	}

	if err := eventHandler.eventStore.AddEvent(event); err != nil {
		log.Printf("event persist failed: %v", err)
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to persist event")
		return
	}

	eventHandler.correlationService.ProcessEvent(event)

	api.WriteJSON(responseWriter, http.StatusCreated, map[string]interface{}{
		"event":     event,
		"duplicate": false,
	})
}

func (eventHandler *EventHandler) ListEvents(responseWriter http.ResponseWriter, request *http.Request) {
	events, err := eventHandler.eventStore.GetEvents()
	if err != nil {
		log.Printf("list events failed: %v", err)
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to fetch events")
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, events)
}