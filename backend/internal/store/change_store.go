package store

import (
	"context"
	"database/sql"
	"time"
)

type ChangeRecord struct {
	ID          int       `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Service     string    `json:"service"`
	Type        string    `json:"type"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

type ChangeStore struct {
	db *sql.DB
}

func NewChangeStore(db *sql.DB) *ChangeStore {
	return &ChangeStore{db: db}
}

// AddChange inserts a new change record into the changes table.
func (changeStore *ChangeStore) AddChange(tenantID, service, changeType, version, description string, ts time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if tenantID == "" {
		tenantID = "default"
	}
	_, err := changeStore.db.ExecContext(ctx, 
		`INSERT INTO changes (tenant_id, service, type, version, description, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, service, changeType, version, description, ts,
	)
	return err
}

func (changeStore *ChangeStore) GetRecentChangeByService(tenantID, service string, incidentTime time.Time) (*ChangeRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if tenantID == "" {
		tenantID = "default"
	}
	windowStart := incidentTime.Add(-10 * time.Minute)
	windowEnd := incidentTime.Add(2 * time.Minute)

	row := changeStore.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service, type, version, description, timestamp
		 FROM changes
		 WHERE tenant_id = $1
		   AND service   = $2
		   AND timestamp >= $3
		   AND timestamp <= $4
		 ORDER BY timestamp DESC
		 LIMIT 1`,
		tenantID, service, windowStart, windowEnd,
	)

	var record ChangeRecord
	err := row.Scan(
		&record.ID,
		&record.TenantID,
		&record.Service,
		&record.Type,
		&record.Version,
		&record.Description,
		&record.Timestamp,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}
