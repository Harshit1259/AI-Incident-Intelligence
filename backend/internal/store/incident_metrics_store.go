package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type IncidentMetricsStore struct {
	db *sql.DB
}

func NewIncidentMetricsStore(db *sql.DB) *IncidentMetricsStore {
	return &IncidentMetricsStore{db: db}
}

func (s *IncidentMetricsStore) Upsert(m models.IncidentMetrics) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO incident_metrics (incident_id, tenant_id, detected_at, acknowledged_at, resolved_at,
		    ttd_seconds, tta_seconds, ttr_seconds, responder, is_autoresolved, toil_minutes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (incident_id) DO UPDATE SET
		    detected_at = COALESCE(EXCLUDED.detected_at, incident_metrics.detected_at),
		    acknowledged_at = COALESCE(EXCLUDED.acknowledged_at, incident_metrics.acknowledged_at),
		    resolved_at = COALESCE(EXCLUDED.resolved_at, incident_metrics.resolved_at),
		    ttd_seconds = CASE WHEN EXCLUDED.ttd_seconds > 0 THEN EXCLUDED.ttd_seconds ELSE incident_metrics.ttd_seconds END,
		    tta_seconds = CASE WHEN EXCLUDED.tta_seconds > 0 THEN EXCLUDED.tta_seconds ELSE incident_metrics.tta_seconds END,
		    ttr_seconds = CASE WHEN EXCLUDED.ttr_seconds > 0 THEN EXCLUDED.ttr_seconds ELSE incident_metrics.ttr_seconds END,
		    responder = CASE WHEN EXCLUDED.responder != '' THEN EXCLUDED.responder ELSE incident_metrics.responder END,
		    is_autoresolved = EXCLUDED.is_autoresolved OR incident_metrics.is_autoresolved,
		    toil_minutes = CASE WHEN EXCLUDED.toil_minutes > 0 THEN EXCLUDED.toil_minutes ELSE incident_metrics.toil_minutes END`,
		m.IncidentID, m.TenantID, m.DetectedAt, m.AcknowledgedAt, m.ResolvedAt,
		m.TTDSeconds, m.TTASeconds, m.TTRSeconds, m.Responder, m.IsAutoResolved, m.ToilMinutes)
	return err
}

func (s *IncidentMetricsStore) GetByIncidentID(incidentID string) (*models.IncidentMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var m models.IncidentMetrics
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, incident_id, tenant_id, detected_at, acknowledged_at, resolved_at,
		    ttd_seconds, tta_seconds, ttr_seconds, responder, is_autoresolved, toil_minutes
		 FROM incident_metrics WHERE incident_id = $1`, incidentID).Scan(
		&m.ID, &m.IncidentID, &m.TenantID, &m.DetectedAt, &m.AcknowledgedAt, &m.ResolvedAt,
		&m.TTDSeconds, &m.TTASeconds, &m.TTRSeconds, &m.Responder, &m.IsAutoResolved, &m.ToilMinutes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *IncidentMetricsStore) GetForPeriod(tenantID string, since time.Time) ([]models.IncidentMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, incident_id, tenant_id, detected_at, acknowledged_at, resolved_at,
		    ttd_seconds, tta_seconds, ttr_seconds, responder, is_autoresolved, toil_minutes
		 FROM incident_metrics WHERE tenant_id = $1 AND detected_at >= $2
		 ORDER BY detected_at DESC`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.IncidentMetrics
	for rows.Next() {
		var m models.IncidentMetrics
		if err := rows.Scan(&m.ID, &m.IncidentID, &m.TenantID, &m.DetectedAt, &m.AcknowledgedAt, &m.ResolvedAt,
			&m.TTDSeconds, &m.TTASeconds, &m.TTRSeconds, &m.Responder, &m.IsAutoResolved, &m.ToilMinutes); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}
