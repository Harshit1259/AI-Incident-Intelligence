package store

import (
	"context"
	"time"
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

type AnomalyStore struct {
	db *sql.DB
}

func NewAnomalyStore(db *sql.DB) *AnomalyStore {
	return &AnomalyStore{db: db}
}

func (s *AnomalyStore) CreateAlert(a models.AnomalyAlert) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO anomaly_alerts (tenant_id, service, metric_name, current_value, threshold, trend, severity, message, fired_at, acknowledged)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.TenantID, a.Service, a.MetricName, a.CurrentValue, a.Threshold,
		a.Trend, a.Severity, a.Message, a.FiredAt, a.Acknowledged)
	return err
}

func (s *AnomalyStore) GetAlerts(tenantID string, limit int) ([]models.AnomalyAlert, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, metric_name, current_value, threshold, trend, severity, message, fired_at, acknowledged
		 FROM anomaly_alerts WHERE tenant_id = $1 ORDER BY fired_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.AnomalyAlert
	for rows.Next() {
		var a models.AnomalyAlert
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Service, &a.MetricName, &a.CurrentValue,
			&a.Threshold, &a.Trend, &a.Severity, &a.Message, &a.FiredAt, &a.Acknowledged); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, rows.Err()
}

func (s *AnomalyStore) AcknowledgeAlert(id int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `UPDATE anomaly_alerts SET acknowledged = true WHERE id = $1`, id)
	return err
}
