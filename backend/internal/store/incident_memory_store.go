package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// IncidentMemoryStore persists resolution records keyed by fingerprint/service.
type IncidentMemoryStore struct {
	db *sql.DB
}

func NewIncidentMemoryStore(db *sql.DB) *IncidentMemoryStore {
	return &IncidentMemoryStore{db: db}
}

// RecordResolution saves a resolution record for future pattern matching.
func (s *IncidentMemoryStore) RecordResolution(tenantID, fingerprint, service, incidentID string, ttrSeconds int, actionsTaken []string, note string, resolvedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actionsJSON, err := json.Marshal(actionsTaken)
	if err != nil {
		actionsJSON = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO incident_resolutions
			(tenant_id, fingerprint, service, incident_id, ttr_seconds, actions_taken, resolution_note, resolved_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (incident_id) DO UPDATE
			SET ttr_seconds = EXCLUDED.ttr_seconds,
			    actions_taken = EXCLUDED.actions_taken,
			    resolution_note = EXCLUDED.resolution_note,
			    resolved_at = EXCLUDED.resolved_at`,
		tenantID, fingerprint, service, incidentID, ttrSeconds, string(actionsJSON), note, resolvedAt,
	)
	return err
}

// GetPatternHistory returns past resolutions for a fingerprint or service, most recent first.
func (s *IncidentMemoryStore) GetPatternHistory(tenantID, fingerprint, service string, limit int) ([]models.ResolutionRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT incident_id, resolved_at, ttr_seconds, actions_taken, resolution_note
		FROM incident_resolutions
		WHERE tenant_id = $1
		  AND (fingerprint = $2 OR ($2 = '' AND service = $3))
		ORDER BY resolved_at DESC
		LIMIT $4`,
		tenantID, fingerprint, service, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.ResolutionRecord
	for rows.Next() {
		var r models.ResolutionRecord
		var resolvedAt time.Time
		var actionsJSON string
		if err := rows.Scan(&r.IncidentID, &resolvedAt, &r.TTRSeconds, &actionsJSON, &r.ResolutionNote); err != nil {
			return nil, err
		}
		r.ResolvedAt = resolvedAt.Format(time.RFC3339)
		_ = json.Unmarshal([]byte(actionsJSON), &r.ActionsTaken)
		if r.ActionsTaken == nil {
			r.ActionsTaken = []string{}
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// CountResolutions returns how many times a fingerprint/service has been resolved.
func (s *IncidentMemoryStore) CountResolutions(tenantID, fingerprint, service string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM incident_resolutions
		WHERE tenant_id = $1 AND (fingerprint = $2 OR ($2 = '' AND service = $3))`,
		tenantID, fingerprint, service,
	).Scan(&count)
	if err != nil {
		log.Printf("incident_memory: count resolutions error: %v", err)
	}
	return count
}
