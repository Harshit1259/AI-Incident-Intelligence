package services

import (
	"fmt"
	"time"
	"strings"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type CorrelationService struct {
	incidentStore *store.IncidentStore
}

func NewCorrelationService(incidentStore *store.IncidentStore) *CorrelationService {
	return &CorrelationService{
		incidentStore: incidentStore,
	}
}

func (correlationService *CorrelationService) CorrelateEvent(event models.Event) models.Incident {
	existingIncidents := correlationService.incidentStore.GetIncidents()

	eventTime, eventTimeError := time.Parse(time.RFC3339, event.Timestamp)
	if eventTimeError != nil {
		eventTime = time.Now().UTC()
		event.Timestamp = eventTime.Format(time.RFC3339)
	}

	for _, incident := range existingIncidents {
		if incident.Service != event.Service {
			continue
		}

		if incident.Severity != event.Severity {
			continue
		}

		lastEventTime, parseError := time.Parse(time.RFC3339, incident.LastEventTime)
		if parseError != nil {
			continue
		}

		timeDifference := eventTime.Sub(lastEventTime)
		if timeDifference < 0 {
			timeDifference = -timeDifference
		}

		if timeDifference <= 10*time.Minute {
			incident.EventIDs = append(incident.EventIDs, event.ID)
			incident.LastEventTime = event.Timestamp
			correlationService.incidentStore.UpdateIncident(incident)
			return incident
		}
	}

	newIncident := models.Incident{
		ID:             fmt.Sprintf("incident-%d", time.Now().UnixNano()),
		Service:        event.Service,
		Severity:       event.Severity,
		Status:         "open",
		EventIDs:       []string{event.ID},
		FirstEventTime: event.Timestamp,
		LastEventTime:  event.Timestamp,
		Title:          fmt.Sprintf("[%s] %s - %s", strings.ToUpper(event.Severity), event.Service, event.Message),
	}

	correlationService.incidentStore.AddIncident(newIncident)

	return newIncident
}
