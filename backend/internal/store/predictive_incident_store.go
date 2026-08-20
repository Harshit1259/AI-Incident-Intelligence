package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type PredictiveIncidentStore struct {
	db *sql.DB
}

func NewPredictiveIncidentStore(db *sql.DB) *PredictiveIncidentStore {
	return &PredictiveIncidentStore{db: db}
}

// InsertMetricPoint records a single time-series data point.
func (s *PredictiveIncidentStore) InsertMetricPoint(tenantID, service, metricName string, value float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric_series (tenant_id, service, metric_name, value, recorded_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		tenantID, service, metricName, value)
	return err
}

// GetMetricSeries returns data points for a service+metric since the given time, newest first.
func (s *PredictiveIncidentStore) GetMetricSeries(tenantID, service, metricName string, since time.Time, limit int) ([]models.MetricDataPoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT recorded_at, value FROM metric_series
		 WHERE tenant_id=$1 AND service=$2 AND metric_name=$3 AND recorded_at >= $4
		 ORDER BY recorded_at ASC LIMIT $5`,
		tenantID, service, metricName, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pts []models.MetricDataPoint
	for rows.Next() {
		var p models.MetricDataPoint
		if err := rows.Scan(&p.RecordedAt, &p.Value); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// InsertMetricPointAt records a data point with an explicit timestamp (used for simulation seeding).
func (s *PredictiveIncidentStore) InsertMetricPointAt(tenantID, service, metricName string, value float64, at time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric_series (tenant_id, service, metric_name, value, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, service, metricName, value, at)
	return err
}

// PurgeOldMetrics deletes metric series data older than the given cutoff.
func (s *PredictiveIncidentStore) PurgeOldMetrics(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `DELETE FROM metric_series WHERE recorded_at < $1`, cutoff)
	return err
}

// Create persists a new predictive incident.
func (s *PredictiveIncidentStore) Create(p *models.PredictiveIncident) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO predictive_incidents
		 (id, tenant_id, service, metric_name, current_value, slo_threshold, trend_slope,
		  breach_probability, confidence_interval, predicted_breach_min, predicted_breach_max,
		  status, message, data_points_used, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		p.ID, p.TenantID, p.Service, p.MetricName, p.CurrentValue, p.SLOThreshold,
		p.TrendSlope, p.BreachProbability, p.ConfidenceInterval,
		p.PredictedBreachMin, p.PredictedBreachMax,
		p.Status, p.Message, p.DataPointsUsed, p.CreatedAt)
	return err
}

// GetOpen returns the most recent open prediction for a specific service+metric, or nil.
func (s *PredictiveIncidentStore) GetOpen(tenantID, service, metricName string) (*models.PredictiveIncident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var p models.PredictiveIncident
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, service, metric_name, current_value, slo_threshold, trend_slope,
		        breach_probability, confidence_interval, predicted_breach_min, predicted_breach_max,
		        status, message, data_points_used, created_at, resolved_at
		 FROM predictive_incidents
		 WHERE tenant_id=$1 AND service=$2 AND metric_name=$3 AND status='open'
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, service, metricName).Scan(
		&p.ID, &p.TenantID, &p.Service, &p.MetricName, &p.CurrentValue, &p.SLOThreshold,
		&p.TrendSlope, &p.BreachProbability, &p.ConfidenceInterval,
		&p.PredictedBreachMin, &p.PredictedBreachMax,
		&p.Status, &p.Message, &p.DataPointsUsed, &p.CreatedAt, &p.ResolvedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns predictive incidents for a tenant, ordered by creation time descending.
func (s *PredictiveIncidentStore) List(tenantID, statusFilter string, limit int) ([]models.PredictiveIncident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, tenant_id, service, metric_name, current_value, slo_threshold, trend_slope,
		             breach_probability, confidence_interval, predicted_breach_min, predicted_breach_max,
		             status, message, data_points_used, created_at, resolved_at
		      FROM predictive_incidents WHERE tenant_id=$1`
	args := []any{tenantID}

	if statusFilter != "" {
		query += " AND status=$2 ORDER BY created_at DESC LIMIT $3"
		args = append(args, statusFilter, limit)
	} else {
		query += " ORDER BY created_at DESC LIMIT $2"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.PredictiveIncident
	for rows.Next() {
		var p models.PredictiveIncident
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Service, &p.MetricName, &p.CurrentValue, &p.SLOThreshold,
			&p.TrendSlope, &p.BreachProbability, &p.ConfidenceInterval,
			&p.PredictedBreachMin, &p.PredictedBreachMax,
			&p.Status, &p.Message, &p.DataPointsUsed, &p.CreatedAt, &p.ResolvedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// Resolve marks a predictive incident as resolved or false_alarm.
func (s *PredictiveIncidentStore) Resolve(id, resolution string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`UPDATE predictive_incidents SET status=$1, resolved_at=NOW() WHERE id=$2`,
		resolution, id)
	return err
}
