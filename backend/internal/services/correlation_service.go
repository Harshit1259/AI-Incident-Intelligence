package services

import (
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type CorrelationService struct {
	incidentStore *store.IncidentStore
	changeStore   *store.ChangeStore
}

func NewCorrelationService(
	incidentStore *store.IncidentStore,
	changeStore *store.ChangeStore,
) *CorrelationService {
	return &CorrelationService{
		incidentStore: incidentStore,
		changeStore:   changeStore,
	}
}

// ENTRYPOINT
func (s *CorrelationService) ProcessEvent(event models.Event) {
	incident := s.simpleCreateIncident(event)
	s.updateIncident(incident, event)
}

// ================= SIMPLE SAFE LOGIC =================

func (s *CorrelationService) simpleCreateIncident(event models.Event) models.Incident {
	incident := models.Incident{
		ID:             generateID(),
		Title:          event.Title,
		Service:        event.Service,
		Severity:       event.Severity,
		Status:         "open",
		FirstEventTime: event.Timestamp.Format(time.RFC3339),
		LastEventTime:  event.Timestamp.Format(time.RFC3339),
		EventCount:     1,
	}

	// Use existing method only
	s.incidentStore.AddIncident(incident)

	return incident
}

func (s *CorrelationService) updateIncident(incident models.Incident, event models.Event) {
	lastEventTime, _ := time.Parse(time.RFC3339, incident.LastEventTime)
if event.Timestamp.After(lastEventTime) {
incident.LastEventTime = event.Timestamp.Format(time.RFC3339)
	}

	incident.EventCount++

	if event.Severity == "critical" {
		incident.Severity = "critical"
	}

	s.incidentStore.UpdateIncident(incident)
}

// ================= UTIL =================

func generateID() string {
	return "incident-" + time.Now().Format("20060102150405")
}
