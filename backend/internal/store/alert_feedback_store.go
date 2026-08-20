package store

import (
	"context"
	"time"
	"database/sql"
	"log"

	"ai-incident-platform/backend/internal/models"
)

type AlertFeedbackStore struct {
	db *sql.DB
}

func NewAlertFeedbackStore(db *sql.DB) *AlertFeedbackStore {
	return &AlertFeedbackStore{db: db}
}

func (s *AlertFeedbackStore) Create(f models.AlertFeedback) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO alert_feedback (event_id, incident_id, tenant_id, feedback, fingerprint, reason, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		f.EventID, f.IncidentID, f.TenantID, f.Feedback, f.Fingerprint, f.Reason, f.CreatedBy,
	)
	return err
}

func (s *AlertFeedbackStore) GetByTenant(tenantID string, limit int) ([]models.AlertFeedback, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, event_id, incident_id, tenant_id, feedback, fingerprint, reason, created_by, created_at
		 FROM alert_feedback WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.AlertFeedback
	for rows.Next() {
		var f models.AlertFeedback
		if err := rows.Scan(&f.ID, &f.EventID, &f.IncidentID, &f.TenantID, &f.Feedback,
			&f.Fingerprint, &f.Reason, &f.CreatedBy, &f.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, f)
	}
	return results, rows.Err()
}

// CountNoisyFingerprints returns fingerprints with >= 3 noise feedbacks.
func (s *AlertFeedbackStore) CountNoisyFingerprints(tenantID string) ([]models.NoisyAlert, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT af.fingerprint, COUNT(*) as noise_count,
		        COALESCE(MAX(e.service), '') as service,
		        COALESCE(MAX(af.created_at)::TEXT, '') as last_seen
		 FROM alert_feedback af
		 LEFT JOIN events e ON e.fingerprint = af.fingerprint
		 WHERE af.tenant_id = $1 AND af.feedback = 'noise'
		 GROUP BY af.fingerprint
		 HAVING COUNT(*) >= 3
		 ORDER BY noise_count DESC
		 LIMIT 20`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.NoisyAlert
	for rows.Next() {
		var n models.NoisyAlert
		if err := rows.Scan(&n.Fingerprint, &n.NoiseCount, &n.Service, &n.LastSeen); err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	return results, rows.Err()
}

// GetSourceFeedbackCounts returns per-source quality statistics derived from
// events joined with alert_feedback. Sources with no feedback are excluded.
func (s *AlertFeedbackStore) GetSourceFeedbackCounts(tenantID string) ([]models.SourceQualityStat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			e.source,
			COUNT(DISTINCT af.id)                                   AS total_alerts,
			COUNT(DISTINCT af.id) FILTER (WHERE af.feedback = 'useful') AS useful_count,
			COUNT(DISTINCT af.id) FILTER (WHERE af.feedback = 'noise')  AS noise_count
		FROM alert_feedback af
		JOIN events e ON e.id = af.event_id
		WHERE af.tenant_id = $1
		GROUP BY e.source
		ORDER BY noise_count DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.SourceQualityStat
	for rows.Next() {
		var s2 models.SourceQualityStat
		if err := rows.Scan(&s2.Source, &s2.TotalAlerts, &s2.UsefulCount, &s2.NoiseCount); err != nil {
			return nil, err
		}
		if s2.TotalAlerts > 0 {
			s2.NoiseRatio = float64(s2.NoiseCount) / float64(s2.TotalAlerts)
			s2.QualityScore = 100 - int(s2.NoiseRatio*100)
		}
		stats = append(stats, s2)
	}
	return stats, rows.Err()
}

// IsSuppressed returns true if a fingerprint has >= 5 noise feedbacks.
func (s *AlertFeedbackStore) IsSuppressed(fingerprint string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if fingerprint == "" {
		return false
	}
	var count int
	err := s.db.QueryRowContext(ctx, 
		`SELECT COUNT(*) FROM alert_feedback WHERE fingerprint = $1 AND feedback = 'noise'`,
		fingerprint,
	).Scan(&count)
	if err != nil {
		log.Printf("alert_feedback: error checking suppression for %s: %v", fingerprint, err)
		return false
	}
	return count >= 5
}
