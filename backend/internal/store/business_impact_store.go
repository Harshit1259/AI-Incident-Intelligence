package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type BusinessImpactStore struct {
	db *sql.DB
}

func NewBusinessImpactStore(db *sql.DB) *BusinessImpactStore {
	return &BusinessImpactStore{db: db}
}

// ── Service Profiles ─────────────────────────────────────────────────────────

func (s *BusinessImpactStore) UpsertProfile(p models.ServiceProfile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO biz_service_profiles
		 (id, tenant_id, service, tier, cost_model_type,
		  hourly_revenue, transactions_per_hour, avg_order_value,
		  users_per_hour, employee_cost_per_hour,
		  sla_penalty_per_minute, sla_threshold_minutes,
		  business_hour_multiplier, peak_multiplier,
		  infra_cost_per_hour, confidence_mode, currency,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		 ON CONFLICT (id) DO UPDATE SET
		   tenant_id = EXCLUDED.tenant_id,
		   service = EXCLUDED.service,
		   tier = EXCLUDED.tier,
		   cost_model_type = EXCLUDED.cost_model_type,
		   hourly_revenue = EXCLUDED.hourly_revenue,
		   transactions_per_hour = EXCLUDED.transactions_per_hour,
		   avg_order_value = EXCLUDED.avg_order_value,
		   users_per_hour = EXCLUDED.users_per_hour,
		   employee_cost_per_hour = EXCLUDED.employee_cost_per_hour,
		   sla_penalty_per_minute = EXCLUDED.sla_penalty_per_minute,
		   sla_threshold_minutes = EXCLUDED.sla_threshold_minutes,
		   business_hour_multiplier = EXCLUDED.business_hour_multiplier,
		   peak_multiplier = EXCLUDED.peak_multiplier,
		   infra_cost_per_hour = EXCLUDED.infra_cost_per_hour,
		   confidence_mode = EXCLUDED.confidence_mode,
		   currency = EXCLUDED.currency,
		   updated_at = EXCLUDED.updated_at`,
		p.ID, p.TenantID, p.Service, p.Tier, p.CostModelType,
		p.HourlyRevenue, p.TransactionsPerHour, p.AvgOrderValue,
		p.UsersPerHour, p.EmployeeCostPerHour,
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes,
		p.BusinessHourMultiplier, p.PeakMultiplier,
		p.InfraCostPerHour, p.ConfidenceMode, p.Currency,
		p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *BusinessImpactStore) GetProfileByService(tenantID, service string) (*models.ServiceProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var p models.ServiceProfile
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service, tier, cost_model_type,
		        hourly_revenue, transactions_per_hour, avg_order_value,
		        users_per_hour, employee_cost_per_hour,
		        sla_penalty_per_minute, sla_threshold_minutes,
		        business_hour_multiplier, peak_multiplier,
		        infra_cost_per_hour, confidence_mode, currency,
		        created_at, updated_at
		 FROM biz_service_profiles
		 WHERE tenant_id = $1 AND service = $2`, tenantID, service,
	).Scan(
		&p.ID, &p.TenantID, &p.Service, &p.Tier, &p.CostModelType,
		&p.HourlyRevenue, &p.TransactionsPerHour, &p.AvgOrderValue,
		&p.UsersPerHour, &p.EmployeeCostPerHour,
		&p.SLAPenaltyPerMinute, &p.SLAThresholdMinutes,
		&p.BusinessHourMultiplier, &p.PeakMultiplier,
		&p.InfraCostPerHour, &p.ConfidenceMode, &p.Currency,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *BusinessImpactStore) GetProfiles(tenantID string) ([]models.ServiceProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, tier, cost_model_type,
		        hourly_revenue, transactions_per_hour, avg_order_value,
		        users_per_hour, employee_cost_per_hour,
		        sla_penalty_per_minute, sla_threshold_minutes,
		        business_hour_multiplier, peak_multiplier,
		        infra_cost_per_hour, confidence_mode, currency,
		        created_at, updated_at
		 FROM biz_service_profiles
		 WHERE tenant_id = $1
		 ORDER BY service`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []models.ServiceProfile
	for rows.Next() {
		var p models.ServiceProfile
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Service, &p.Tier, &p.CostModelType,
			&p.HourlyRevenue, &p.TransactionsPerHour, &p.AvgOrderValue,
			&p.UsersPerHour, &p.EmployeeCostPerHour,
			&p.SLAPenaltyPerMinute, &p.SLAThresholdMinutes,
			&p.BusinessHourMultiplier, &p.PeakMultiplier,
			&p.InfraCostPerHour, &p.ConfidenceMode, &p.Currency,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (s *BusinessImpactStore) DeleteProfile(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM biz_service_profiles WHERE id = $1`, id)
	return err
}

// ── Baselines ────────────────────────────────────────────────────────────────

func (s *BusinessImpactStore) UpsertBaseline(b models.IncidentBaseline) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO biz_baselines
		 (id, tenant_id, service, incident_type,
		  avg_mttr_minutes, median_mttr_minutes, p90_mttr_minutes,
		  sample_size, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (id) DO UPDATE SET
		   tenant_id = EXCLUDED.tenant_id,
		   service = EXCLUDED.service,
		   incident_type = EXCLUDED.incident_type,
		   avg_mttr_minutes = EXCLUDED.avg_mttr_minutes,
		   median_mttr_minutes = EXCLUDED.median_mttr_minutes,
		   p90_mttr_minutes = EXCLUDED.p90_mttr_minutes,
		   sample_size = EXCLUDED.sample_size,
		   updated_at = EXCLUDED.updated_at`,
		b.ID, b.TenantID, b.Service, b.IncidentType,
		b.AvgMTTRMinutes, b.MedianMTTRMinutes, b.P90MTTRMinutes,
		b.SampleSize, b.CreatedAt, b.UpdatedAt,
	)
	return err
}

func (s *BusinessImpactStore) GetBaseline(tenantID, service, incidentType string) (*models.IncidentBaseline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var b models.IncidentBaseline
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service, incident_type,
		        avg_mttr_minutes, median_mttr_minutes, p90_mttr_minutes,
		        sample_size, created_at, updated_at
		 FROM biz_baselines
		 WHERE tenant_id = $1 AND service = $2 AND incident_type = $3`,
		tenantID, service, incidentType,
	).Scan(
		&b.ID, &b.TenantID, &b.Service, &b.IncidentType,
		&b.AvgMTTRMinutes, &b.MedianMTTRMinutes, &b.P90MTTRMinutes,
		&b.SampleSize, &b.CreatedAt, &b.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *BusinessImpactStore) GetBaselines(tenantID string) ([]models.IncidentBaseline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, incident_type,
		        avg_mttr_minutes, median_mttr_minutes, p90_mttr_minutes,
		        sample_size, created_at, updated_at
		 FROM biz_baselines
		 WHERE tenant_id = $1
		 ORDER BY service, incident_type`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var baselines []models.IncidentBaseline
	for rows.Next() {
		var b models.IncidentBaseline
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.Service, &b.IncidentType,
			&b.AvgMTTRMinutes, &b.MedianMTTRMinutes, &b.P90MTTRMinutes,
			&b.SampleSize, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		baselines = append(baselines, b)
	}
	return baselines, rows.Err()
}

// ── Financial Estimates ──────────────────────────────────────────────────────

func (s *BusinessImpactStore) SaveEstimate(e models.FinancialEstimate) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO biz_financial_estimates
		 (id, tenant_id, incident_id, service,
		  actual_loss, counterfactual_loss, avoided_loss,
		  confidence_level, confidence_score, method_used,
		  currency, breakdown_json, explanation, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 ON CONFLICT (id) DO UPDATE SET
		   actual_loss = EXCLUDED.actual_loss,
		   counterfactual_loss = EXCLUDED.counterfactual_loss,
		   avoided_loss = EXCLUDED.avoided_loss,
		   confidence_level = EXCLUDED.confidence_level,
		   confidence_score = EXCLUDED.confidence_score,
		   method_used = EXCLUDED.method_used,
		   breakdown_json = EXCLUDED.breakdown_json,
		   explanation = EXCLUDED.explanation,
		   created_at = EXCLUDED.created_at`,
		e.ID, e.TenantID, e.IncidentID, e.Service,
		e.ActualLoss, e.CounterfactualLoss, e.AvoidedLoss,
		e.ConfidenceLevel, e.ConfidenceScore, e.MethodUsed,
		e.Currency, e.BreakdownJSON, e.Explanation, e.CreatedAt,
	)
	return err
}

func (s *BusinessImpactStore) GetEstimateByIncident(incidentID string) (*models.FinancialEstimate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var e models.FinancialEstimate
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, incident_id, service,
		        actual_loss, counterfactual_loss, avoided_loss,
		        confidence_level, confidence_score, method_used,
		        currency, breakdown_json, explanation, created_at
		 FROM biz_financial_estimates
		 WHERE incident_id = $1`, incidentID,
	).Scan(
		&e.ID, &e.TenantID, &e.IncidentID, &e.Service,
		&e.ActualLoss, &e.CounterfactualLoss, &e.AvoidedLoss,
		&e.ConfidenceLevel, &e.ConfidenceScore, &e.MethodUsed,
		&e.Currency, &e.BreakdownJSON, &e.Explanation, &e.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *BusinessImpactStore) GetEstimatesForPeriod(tenantID string, start, end time.Time) ([]models.FinancialEstimate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, incident_id, service,
		        actual_loss, counterfactual_loss, avoided_loss,
		        confidence_level, confidence_score, method_used,
		        currency, breakdown_json, explanation, created_at
		 FROM biz_financial_estimates
		 WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		 ORDER BY actual_loss DESC`, tenantID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var estimates []models.FinancialEstimate
	for rows.Next() {
		var e models.FinancialEstimate
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.IncidentID, &e.Service,
			&e.ActualLoss, &e.CounterfactualLoss, &e.AvoidedLoss,
			&e.ConfidenceLevel, &e.ConfidenceScore, &e.MethodUsed,
			&e.Currency, &e.BreakdownJSON, &e.Explanation, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		estimates = append(estimates, e)
	}
	return estimates, rows.Err()
}

// ── Monthly Rollups ──────────────────────────────────────────────────────────

func (s *BusinessImpactStore) UpsertRollup(r models.MonthlyRollup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO biz_monthly_rollups
		 (id, tenant_id, year, month,
		  total_actual_loss, total_counterfactual_loss, total_avoided_loss,
		  confidence_weighted_loss, currency, top_incidents_json,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (id) DO UPDATE SET
		   total_actual_loss = EXCLUDED.total_actual_loss,
		   total_counterfactual_loss = EXCLUDED.total_counterfactual_loss,
		   total_avoided_loss = EXCLUDED.total_avoided_loss,
		   confidence_weighted_loss = EXCLUDED.confidence_weighted_loss,
		   top_incidents_json = EXCLUDED.top_incidents_json,
		   updated_at = EXCLUDED.updated_at`,
		r.ID, r.TenantID, r.Year, r.Month,
		r.TotalActualLoss, r.TotalCounterfactualLoss, r.TotalAvoidedLoss,
		r.ConfidenceWeightedLoss, r.Currency, r.TopIncidentsJSON,
		r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *BusinessImpactStore) GetRollup(tenantID string, year, month int) (*models.MonthlyRollup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var r models.MonthlyRollup
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, year, month,
		        total_actual_loss, total_counterfactual_loss, total_avoided_loss,
		        confidence_weighted_loss, currency, top_incidents_json,
		        created_at, updated_at
		 FROM biz_monthly_rollups
		 WHERE tenant_id = $1 AND year = $2 AND month = $3`,
		tenantID, year, month,
	).Scan(
		&r.ID, &r.TenantID, &r.Year, &r.Month,
		&r.TotalActualLoss, &r.TotalCounterfactualLoss, &r.TotalAvoidedLoss,
		&r.ConfidenceWeightedLoss, &r.Currency, &r.TopIncidentsJSON,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
