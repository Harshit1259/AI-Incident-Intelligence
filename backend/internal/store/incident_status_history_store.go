package store

import (
	"context"
	"database/sql"
	"time"
)

type IncidentStatusHistoryRecord struct {
	ID             int       `json:"id"`
	TenantID       string    `json:"tenant_id"`
	IncidentID     string    `json:"incident_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	Note           string    `json:"note"`
	ChangedBy      string    `json:"changed_by"`
	ChangedAt      time.Time `json:"changed_at"`
}

type IncidentStatusHistoryStore struct {
	db *sql.DB
}

func NewIncidentStatusHistoryStore(db *sql.DB) *IncidentStatusHistoryStore {
	return &IncidentStatusHistoryStore{db: db}
}

// AddRecord inserts a status transition record scoped to the owning tenant.
func (s *IncidentStatusHistoryStore) AddRecord(tenantID, incidentID, previousStatus, newStatus, note, changedBy string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO incident_status_history (
			tenant_id,
			incident_id,
			previous_status,
			new_status,
			note,
			changed_by,
			changed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID,
		incidentID,
		previousStatus,
		newStatus,
		note,
		changedBy,
		time.Now().UTC(),
	)
	return err
}

// GetByIncidentID returns history records for an incident, filtered by tenant.
// The tenantID parameter is mandatory — omitting it would allow cross-tenant reads.
func (s *IncidentStatusHistoryStore) GetByIncidentID(tenantID, incidentID string) ([]IncidentStatusHistoryRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT
			id,
			tenant_id,
			incident_id,
			previous_status,
			new_status,
			note,
			changed_by,
			changed_at
		 FROM incident_status_history
		 WHERE tenant_id = $1 AND incident_id = $2
		 ORDER BY changed_at DESC`,
		tenantID,
		incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]IncidentStatusHistoryRecord, 0)
	for rows.Next() {
		var r IncidentStatusHistoryRecord
		if err := rows.Scan(
			&r.ID,
			&r.TenantID,
			&r.IncidentID,
			&r.PreviousStatus,
			&r.NewStatus,
			&r.Note,
			&r.ChangedBy,
			&r.ChangedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
