package store

import (
	"database/sql"
	"time"
)

type ChangeRecord struct {
	ID          int    `json:"id"`
	Service     string `json:"service"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type ChangeStore struct {
	db *sql.DB
}

func NewChangeStore(db *sql.DB) *ChangeStore {
	return &ChangeStore{db: db}
}

func (changeStore *ChangeStore) GetRecentChangeByService(service string, incidentTime time.Time) (*ChangeRecord, error) {
	rows, err := changeStore.db.Query(
		`SELECT id, service, type, version, description, timestamp
		 FROM changes
		 WHERE service = $1
		 ORDER BY timestamp DESC`,
		service,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	windowStart := incidentTime.Add(-10 * time.Minute)
	windowEnd := incidentTime.Add(2 * time.Minute)

	for rows.Next() {
		var record ChangeRecord
		if err := rows.Scan(
			&record.ID,
			&record.Service,
			&record.Type,
			&record.Version,
			&record.Description,
			&record.Timestamp,
		); err != nil {
			return nil, err
		}

		parsedTimestamp, err := time.Parse(time.RFC3339, record.Timestamp)
		if err != nil {
			continue
		}

		if (parsedTimestamp.Equal(windowStart) || parsedTimestamp.After(windowStart)) &&
			(parsedTimestamp.Equal(windowEnd) || parsedTimestamp.Before(windowEnd)) {
			return &record, nil
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nil, nil
}
