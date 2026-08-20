package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type SLOStore struct {
	db *sql.DB
}

func NewSLOStore(db *sql.DB) *SLOStore {
	return &SLOStore{db: db}
}

func (s *SLOStore) CreateSLO(def models.SLODefinition) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO slo_definitions (id, tenant_id, service, name, description, target_percent, window_days, metric_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		def.ID, def.TenantID, def.Service, def.Name, def.Description,
		def.TargetPercent, def.WindowDays, def.MetricType,
		def.CreatedAt, def.UpdatedAt,
	)
	return err
}

func (s *SLOStore) GetSLOs(tenantID string) ([]models.SLODefinition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, name, description, target_percent, window_days, metric_type, created_at, updated_at
		 FROM slo_definitions WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 500`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SLODefinition
	for rows.Next() {
		var d models.SLODefinition
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Service, &d.Name, &d.Description,
			&d.TargetPercent, &d.WindowDays, &d.MetricType, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (s *SLOStore) GetSLOByID(id string) (*models.SLODefinition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var d models.SLODefinition
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service, name, description, target_percent, window_days, metric_type, created_at, updated_at
		 FROM slo_definitions WHERE id = $1`, id).Scan(
		&d.ID, &d.TenantID, &d.Service, &d.Name, &d.Description,
		&d.TargetPercent, &d.WindowDays, &d.MetricType, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *SLOStore) UpdateSLO(def models.SLODefinition) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE slo_definitions SET service=$1, name=$2, description=$3, target_percent=$4, window_days=$5, metric_type=$6, updated_at=$7
		 WHERE id=$8`,
		def.Service, def.Name, def.Description, def.TargetPercent, def.WindowDays, def.MetricType, time.Now(), def.ID)
	return err
}

func (s *SLOStore) DeleteSLO(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM slo_definitions WHERE id = $1`, id)
	return err
}

func (s *SLOStore) AddMeasurement(m models.SLOMeasurement) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO slo_measurements (slo_id, timestamp, total_requests, good_requests, bad_minutes, error_budget_remaining)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		m.SLOID, m.Timestamp, m.TotalRequests, m.GoodRequests, m.BadMinutes, m.ErrorBudgetRemaining)
	return err
}

func (s *SLOStore) GetMeasurements(sloID string, since time.Time) ([]models.SLOMeasurement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, slo_id, timestamp, total_requests, good_requests, bad_minutes, error_budget_remaining
		 FROM slo_measurements WHERE slo_id = $1 AND timestamp >= $2 ORDER BY timestamp DESC`, sloID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SLOMeasurement
	for rows.Next() {
		var m models.SLOMeasurement
		if err := rows.Scan(&m.ID, &m.SLOID, &m.Timestamp, &m.TotalRequests, &m.GoodRequests, &m.BadMinutes, &m.ErrorBudgetRemaining); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

func (s *SLOStore) GetLatestMeasurement(sloID string) (*models.SLOMeasurement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var m models.SLOMeasurement
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, slo_id, timestamp, total_requests, good_requests, bad_minutes, error_budget_remaining
		 FROM slo_measurements WHERE slo_id = $1 ORDER BY timestamp DESC LIMIT 1`, sloID).Scan(
		&m.ID, &m.SLOID, &m.Timestamp, &m.TotalRequests, &m.GoodRequests, &m.BadMinutes, &m.ErrorBudgetRemaining)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
