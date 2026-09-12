package services

import (
	"log/slog"

	"ai-incident-platform/backend/internal/store"
)

type incidentStatusLookup interface {
	OpenIncidentStatusesWithEvent(tenantID, eventID string) (map[string]string, error)
}

type incidentHistoryWriter interface {
	AddRecord(tenantID, incidentID, previousStatus, newStatus, note, changedBy string) error
}

// IncidentNoteService adds a line to the timeline of every open incident that
// contains an alert — used for acknowledgements and comments made in the
// source tool (e.g. Zabbix). The incident's status is not changed.
type IncidentNoteService struct {
	lookup  incidentStatusLookup
	history incidentHistoryWriter
}

func NewIncidentNoteService(lookup *store.IncidentAutoCloseStore, history *store.IncidentStatusHistoryStore) *IncidentNoteService {
	return &IncidentNoteService{lookup: lookup, history: history}
}

// NoteForEvent records note on each open incident containing the event and
// returns how many incidents were annotated.
func (s *IncidentNoteService) NoteForEvent(tenantID, eventID, actor, note string) int {
	incidents, err := s.lookup.OpenIncidentStatusesWithEvent(tenantID, eventID)
	if err != nil {
		slog.Error("incident_note: lookup incidents", "event_id", eventID, "error", err)
		return 0
	}
	n := 0
	for id, status := range incidents {
		if err := s.history.AddRecord(tenantID, id, status, status, note, actor); err != nil {
			slog.Error("incident_note: add note", "incident_id", id, "error", err)
			continue
		}
		n++
	}
	return n
}
