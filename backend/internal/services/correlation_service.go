package services

import (
	"fmt"
	"strings"
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

func normalizeMessagePattern(message string) string {
	lowerMessage := strings.ToLower(message)

	switch {
	case containsAny(lowerMessage, []string{"timeout", "timed out"}):
		return "timeout"
	case containsAny(lowerMessage, []string{"latency", "slow", "delay"}):
		return "latency"
	case containsAny(lowerMessage, []string{"fail", "failure", "error"}):
		return "failure"
	case containsAny(lowerMessage, []string{"db", "database", "query", "sql", "connection refused"}):
		return "database"
	case containsAny(lowerMessage, []string{"auth", "unauthorized", "forbidden", "token", "credential"}):
		return "auth"
	case containsAny(lowerMessage, []string{"network", "dns", "socket", "connection reset"}):
		return "network"
	default:
		return "unknown"
	}
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isIncidentEligibleForCorrelation(incident models.Incident) bool {
	return incident.Status == "open" || incident.Status == "acknowledged"
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

	eventPattern := normalizeMessagePattern(event.Message)

	for _, incident := range existingIncidents {
		if !isIncidentEligibleForCorrelation(incident) {
			continue
		}

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

		if timeDifference > 15*time.Minute {
			continue
		}

		incidentPattern := normalizeMessagePattern(incident.Title)

		patternsCompatible :=
			eventPattern == incidentPattern ||
				eventPattern == "unknown" ||
				incidentPattern == "unknown" ||
				(eventPattern == "timeout" && incidentPattern == "latency") ||
				(eventPattern == "latency" && incidentPattern == "timeout") ||
				(eventPattern == "failure" && incidentPattern == "timeout") ||
				(eventPattern == "timeout" && incidentPattern == "failure")

		if !patternsCompatible {
			continue
		}

		incident.EventIDs = append(incident.EventIDs, event.ID)
		incident.LastEventTime = event.Timestamp

		if err := correlationService.incidentStore.UpdateIncident(incident); err != nil {
			return models.Incident{}, err
		}

		return incident, nil
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

	if err := correlationService.incidentStore.AddIncident(newIncident); err != nil {
		return models.Incident{}, err
	}

	return newIncident, nil
}
