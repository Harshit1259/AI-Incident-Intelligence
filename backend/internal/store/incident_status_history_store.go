package store

import (
	"database/sql"
	"time"
)

type IncidentStatusHistoryRecord struct {
	ID             int    `json:"id"`
	IncidentID     string `json:"incident_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	Note           string `json:"note"`
	ChangedBy      string `json:"changed_by"`
	ChangedAt      string `json:"changed_at"`
}

type IncidentStatusHistoryStore struct {
	db *sql.DB
}

func NewIncidentStatusHistoryStore(db *sql.DB) *IncidentStatusHistoryStore {
	return &IncidentStatusHistoryStore{db: db}
}

func (historyStore *IncidentStatusHistoryStore) AddRecord(incidentID, previousStatus, newStatus, note, changedBy string) error {
	changedAt := time.Now().UTC().Format(time.RFC3339)

	_, err := historyStore.db.Exec(
		`INSERT INTO incident_status_history (
			incident_id,
			previous_status,
			new_status,
			note,
			changed_by,
			changed_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		incidentID,
		previousStatus,
		newStatus,
		note,
		changedBy,
		changedAt,
	)

	return err
}

func (historyStore *IncidentStatusHistoryStore) GetByIncidentID(incidentID string) ([]IncidentStatusHistoryRecord, error) {
	rows, err := historyStore.db.Query(
		`SELECT
			id,
			incident_id,
			previous_status,
			new_status,
			note,
			changed_by,
			changed_at
		 FROM incident_status_history
		 WHERE incident_id = $1
		 ORDER BY changed_at DESC`,
		incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]IncidentStatusHistoryRecord, 0)

	for rows.Next() {
		var record IncidentStatusHistoryRecord
		if err := rows.Scan(
			&record.ID,
			&record.IncidentID,
			&record.PreviousStatus,
			&record.NewStatus,
			&record.Note,
			&record.ChangedBy,
			&record.ChangedAt,
		); err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}
