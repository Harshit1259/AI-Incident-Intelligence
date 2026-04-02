package store

import (
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

type IncidentStore struct {
	db *sql.DB
}

func NewIncidentStore(db *sql.DB) *IncidentStore {
	return &IncidentStore{db: db}
}

func (incidentStore *IncidentStore) AddIncident(incident models.Incident) error {
	_, err := incidentStore.db.Exec(
		`INSERT INTO incidents (id, service, severity, status, first_event_time, last_event_time, title)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		incident.ID,
		incident.Service,
		incident.Severity,
		incident.Status,
		incident.FirstEventTime,
		incident.LastEventTime,
		incident.Title,
	)
	if err != nil {
		return err
	}

	for _, eventID := range incident.EventIDs {
		_, err = incidentStore.db.Exec(
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			incident.ID,
			eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (incidentStore *IncidentStore) GetIncidents() ([]models.Incident, error) {
	rows, err := incidentStore.db.Query(
		`SELECT id, service, severity, status, first_event_time, last_event_time, title
		 FROM incidents
		 ORDER BY last_event_time DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	incidents := make([]models.Incident, 0)

	for rows.Next() {
		var incident models.Incident
		err := rows.Scan(
			&incident.ID,
			&incident.Service,
			&incident.Severity,
			&incident.Status,
			&incident.FirstEventTime,
			&incident.LastEventTime,
			&incident.Title,
		)
		if err != nil {
			return nil, err
		}

		eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
		if err != nil {
			return nil, err
		}
		incident.EventIDs = eventIDs

		incidents = append(incidents, incident)
	}

	return incidents, nil
}

func (incidentStore *IncidentStore) UpdateIncident(updatedIncident models.Incident) error {
	_, err := incidentStore.db.Exec(
		`UPDATE incidents
		 SET service = $2, severity = $3, status = $4, first_event_time = $5, last_event_time = $6, title = $7
		 WHERE id = $1`,
		updatedIncident.ID,
		updatedIncident.Service,
		updatedIncident.Severity,
		updatedIncident.Status,
		updatedIncident.FirstEventTime,
		updatedIncident.LastEventTime,
		updatedIncident.Title,
	)
	if err != nil {
		return err
	}

	_, err = incidentStore.db.Exec(`DELETE FROM incident_events WHERE incident_id = $1`, updatedIncident.ID)
	if err != nil {
		return err
	}

	for _, eventID := range updatedIncident.EventIDs {
		_, err = incidentStore.db.Exec(
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			updatedIncident.ID,
			eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (incidentStore *IncidentStore) GetIncidentByID(incidentID string) (models.Incident, bool) {
	row := incidentStore.db.QueryRow(
		`SELECT id, service, severity, status, first_event_time, last_event_time, title
		 FROM incidents
		 WHERE id = $1`,
		incidentID,
	)

	var incident models.Incident
	err := row.Scan(
		&incident.ID,
		&incident.Service,
		&incident.Severity,
		&incident.Status,
		&incident.FirstEventTime,
		&incident.LastEventTime,
		&incident.Title,
	)
	if err != nil {
		return models.Incident{}, false
	}

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err != nil {
		return models.Incident{}, false
	}
	incident.EventIDs = eventIDs

	return incident, true
}

func (incidentStore *IncidentStore) GetEventIDsByIncidentID(incidentID string) ([]string, error) {
	rows, err := incidentStore.db.Query(
		`SELECT event_id FROM incident_events WHERE incident_id = $1`,
		incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eventIDs := make([]string, 0)

	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		eventIDs = append(eventIDs, eventID)
	}

	return eventIDs, nil
}
