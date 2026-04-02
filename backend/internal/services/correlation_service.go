package services

import (
	"fmt"
	"time"

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

func (correlationService *CorrelationService) CorrelateEvent(event models.Event) (models.Incident, error) {
	existingIncidents, err := correlationService.incidentStore.GetIncidents()
	if err != nil {
		return models.Incident{}, err
	}

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

			if err := correlationService.incidentStore.UpdateIncident(incident); err != nil {
				return models.Incident{}, err
			}

			return incident, nil
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
		Title:          fmt.Sprintf("[%s] %s - %s", event.Severity, event.Service, event.Message),
	}

	if err := correlationService.incidentStore.AddIncident(newIncident); err != nil {
		return models.Incident{}, err
	}

	return newIncident, nil
}
