package store

import (
	"database/sql"
	"database/sql/driver"

	"ai-incident-platform/backend/internal/models"
)

type EventStore struct {
	db *sql.DB
}

func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

func (eventStore *EventStore) AddEvent(event models.Event) error {
	_, err := eventStore.db.Exec(
		`INSERT INTO events (id, source, type, service, severity, message, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID,
		event.Source,
		event.Type,
		event.Service,
		event.Severity,
		event.Message,
		event.Timestamp,
	)

	return err
}

func (eventStore *EventStore) GetEvents() ([]models.Event, error) {
	rows, err := eventStore.db.Query(
		`SELECT id, source, type, service, severity, message, timestamp
		 FROM events
		 ORDER BY timestamp DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)

	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Message,
			&event.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, nil
}

func (eventStore *EventStore) GetEventsByIDs(eventIDs []string) ([]models.Event, error) {
	if len(eventIDs) == 0 {
		return []models.Event{}, nil
	}

	rows, err := eventStore.db.Query(
		`SELECT id, source, type, service, severity, message, timestamp
		 FROM events
		 WHERE id = ANY($1)`,
		stringArray(eventIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)

	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Message,
			&event.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, nil
}

type stringArray []string

func (array stringArray) Value() (driver.Value, error) {
	if len(array) == 0 {
		return "{}", nil
	}

	result := "{"
	for index, value := range array {
		if index > 0 {
			result += ","
		}
		result += `"` + value + `"`
	}
	result += "}"

	return result, nil
}
