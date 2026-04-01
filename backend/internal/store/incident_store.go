package store

import (
	"sync"

	"ai-incident-platform/backend/internal/models"
)

type IncidentStore struct {
	mu        sync.RWMutex
	incidents []models.Incident
}

func NewIncidentStore() *IncidentStore {
	return &IncidentStore{
		incidents: []models.Incident{},
	}
}

func (incidentStore *IncidentStore) AddIncident(incident models.Incident) {
	incidentStore.mu.Lock()
	defer incidentStore.mu.Unlock()

	incidentStore.incidents = append(incidentStore.incidents, incident)
}

func (incidentStore *IncidentStore) GetIncidents() []models.Incident {
	incidentStore.mu.RLock()
	defer incidentStore.mu.RUnlock()

	incidentsCopy := make([]models.Incident, len(incidentStore.incidents))
	copy(incidentsCopy, incidentStore.incidents)

	return incidentsCopy
}

func (incidentStore *IncidentStore) UpdateIncident(updatedIncident models.Incident) {
	incidentStore.mu.Lock()
	defer incidentStore.mu.Unlock()

	for incidentIndex, incident := range incidentStore.incidents {
		if incident.ID == updatedIncident.ID {
			incidentStore.incidents[incidentIndex] = updatedIncident
			return
		}
	}
}
