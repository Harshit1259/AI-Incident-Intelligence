package store

import (
	"sync"

	"ai-incident-platform/backend/internal/models"
)

type EventStore struct {
	mu     sync.RWMutex
	events []models.Event
}

func NewEventStore() *EventStore {
	return &EventStore{
		events: []models.Event{},
	}
}

func (eventStore *EventStore) AddEvent(event models.Event) {
	eventStore.mu.Lock()
	defer eventStore.mu.Unlock()

	eventStore.events = append(eventStore.events, event)
}

func (eventStore *EventStore) GetEvents() []models.Event {
	eventStore.mu.RLock()
	defer eventStore.mu.RUnlock()

	eventsCopy := make([]models.Event, len(eventStore.events))
	copy(eventsCopy, eventStore.events)

	return eventsCopy
}

func (eventStore *EventStore) GetEventsByIDs(eventIDs []string) []models.Event {
	eventStore.mu.RLock()
	defer eventStore.mu.RUnlock()

	eventIDSet := make(map[string]bool)
	for _, eventID := range eventIDs {
		eventIDSet[eventID] = true
	}

	filteredEvents := make([]models.Event, 0)
	for _, event := range eventStore.events {
		if eventIDSet[event.ID] {
			filteredEvents = append(filteredEvents, event)
		}
	}

	return filteredEvents
}
