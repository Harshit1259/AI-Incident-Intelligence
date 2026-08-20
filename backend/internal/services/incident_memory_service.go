package services

// IncidentMemoryService learns from past incident resolutions and surfaces playbooks.
//
// Every time an incident is resolved:
//   - The resolution is recorded (fingerprint → actions taken, TTR, note)
//
// When an incident is opened:
//   - Past resolutions for the same fingerprint/service are queried
//   - A suggested playbook is derived from the most common action sequence

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// IncidentMemoryService wires together memory store and incident store.
type IncidentMemoryService struct {
	memoryStore   *store.IncidentMemoryStore
	incidentStore *store.IncidentStore
}

func NewIncidentMemoryService(ms *store.IncidentMemoryStore, is *store.IncidentStore) *IncidentMemoryService {
	return &IncidentMemoryService{memoryStore: ms, incidentStore: is}
}

// RecordResolution captures how an incident was resolved so future incidents can learn from it.
// actionsTaken is the ordered list of action IDs/labels that were executed.
func (s *IncidentMemoryService) RecordResolution(incidentID, tenantID, note string) error {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return fmt.Errorf("incident not found: %s", incidentID)
	}

	// Compute TTR from first event time to now
	ttrSeconds := 0
	if !incident.FirstEventTime.IsZero() {
		ttrSeconds = int(time.Since(incident.FirstEventTime).Seconds())
	}

	// Use incident reasoning as the action record (what we know was done)
	actionsTaken := incident.Reasoning
	if len(actionsTaken) == 0 {
		actionsTaken = []string{"Manual resolution"}
	}

	return s.memoryStore.RecordResolution(
		tenantID,
		incident.Fingerprint,
		incident.Service,
		incidentID,
		ttrSeconds,
		actionsTaken,
		note,
		time.Now(),
	)
}

// GetPatternHistory returns past resolutions and a suggested playbook for an incident.
func (s *IncidentMemoryService) GetPatternHistory(incidentID, tenantID string) (*models.PatternHistory, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	records, err := s.memoryStore.GetPatternHistory(tenantID, incident.Fingerprint, incident.Service, 10)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []models.ResolutionRecord{}
	}

	count := s.memoryStore.CountResolutions(tenantID, incident.Fingerprint, incident.Service)

	playbook := buildPlaybook(records)

	return &models.PatternHistory{
		Fingerprint:       incident.Fingerprint,
		Service:           incident.Service,
		RecurrenceCount:   count,
		SuggestedPlaybook: playbook,
		PastResolutions:   records,
	}, nil
}

// buildPlaybook derives an ordered list of action steps from past resolution records
// by scoring action frequency across all resolutions.
func buildPlaybook(records []models.ResolutionRecord) []string {
	if len(records) == 0 {
		return []string{}
	}

	// Count action occurrence frequency
	freq := map[string]int{}
	for _, r := range records {
		for _, action := range r.ActionsTaken {
			action = strings.TrimSpace(action)
			if action != "" {
				freq[action]++
			}
		}
	}
	if len(freq) == 0 {
		return []string{}
	}

	type scored struct {
		action string
		count  int
	}
	var ranked []scored
	for action, count := range freq {
		ranked = append(ranked, scored{action, count})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})

	// Return top 5 most frequent actions as the playbook
	playbook := make([]string, 0, 5)
	for i, s := range ranked {
		if i >= 5 {
			break
		}
		playbook = append(playbook, s.action)
	}
	return playbook
}
