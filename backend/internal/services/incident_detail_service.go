package services

import (
	"sort"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type IncidentDetailService struct {
	incidentStore *store.IncidentStore
	eventStore    *store.EventStore
}

func NewIncidentDetailService(incidentStore *store.IncidentStore, eventStore *store.EventStore) *IncidentDetailService {
	return &IncidentDetailService{
		incidentStore: incidentStore,
		eventStore:    eventStore,
	}
}

func (incidentDetailService *IncidentDetailService) GetIncidentDetail(incidentID string) (models.IncidentDetail, bool) {
	incident, found := incidentDetailService.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return models.IncidentDetail{}, false
	}

	events := incidentDetailService.eventStore.GetEventsByIDs(incident.EventIDs)

	sort.Slice(events, func(leftIndex, rightIndex int) bool {
		return events[leftIndex].Timestamp < events[rightIndex].Timestamp
	})

	detail := models.IncidentDetail{
		Incident: incident,
		Events:   events,
		Summary: models.IncidentSummary{
			EventCount:      len(events),
			Service:         incident.Service,
			Severity:        incident.Severity,
			LatestEventTime: incident.LastEventTime,
		},
	}

	return detail, true
}
