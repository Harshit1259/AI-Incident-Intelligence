package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// TenantStore manages the enriched tenant master records.
type TenantStore struct {
	db *sql.DB
}

func NewTenantStore(db *sql.DB) *TenantStore {
	return &TenantStore{db: db}
}

// Upsert creates or updates a tenant (business profile fields only).
func (s *TenantStore) Upsert(t models.TenantMaster) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	if t.DataRegion == "" {
		t.DataRegion = "us"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tenants
		 (id, name, slug, deployment_mode, industry_type, default_currency,
		  timezone, estimation_mode, plan, state, owner_email, data_region, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 ON CONFLICT (id) DO UPDATE SET
		   name             = EXCLUDED.name,
		   slug             = EXCLUDED.slug,
		   deployment_mode  = EXCLUDED.deployment_mode,
		   industry_type    = EXCLUDED.industry_type,
		   default_currency = EXCLUDED.default_currency,
		   timezone         = EXCLUDED.timezone,
		   estimation_mode  = EXCLUDED.estimation_mode,
		   owner_email      = EXCLUDED.owner_email,
		   data_region      = EXCLUDED.data_region,
		   updated_at       = EXCLUDED.updated_at`,
		t.ID, t.Name, t.Slug, t.DeploymentMode, t.IndustryType, t.DefaultCurrency,
		t.Timezone, t.EstimationMode,
		coalesce(t.Plan, "trial"), coalesce(t.State, "active"), t.OwnerEmail,
		t.DataRegion, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// GetByID returns a tenant with all SaaS fields.
func (s *TenantStore) GetByID(id string) (*models.TenantMaster, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var t models.TenantMaster
	var suspendedAt, disabledAt *time.Time
	var ingestRate, queryRate sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug,
		       COALESCE(deployment_mode,'cloud'),
		       COALESCE(industry_type,'general'),
		       COALESCE(default_currency,'USD'),
		       COALESCE(timezone,'UTC'),
		       COALESCE(estimation_mode,'balanced'),
		       created_at,
		       COALESCE(updated_at, created_at),
		       COALESCE(plan,'trial'),
		       COALESCE(state,'active'),
		       COALESCE(owner_email,''),
		       suspended_at, disabled_at,
		       ingest_rate_min, query_rate_min,
		       COALESCE(max_incidents,0), COALESCE(max_events,0),
		       COALESCE(encryption_enabled,false),
		       COALESCE(audit_retain_days,90),
		       COALESCE(data_region,'us')
		FROM tenants WHERE id = $1`, id,
	).Scan(
		&t.ID, &t.Name, &t.Slug,
		&t.DeploymentMode, &t.IndustryType, &t.DefaultCurrency,
		&t.Timezone, &t.EstimationMode,
		&t.CreatedAt, &t.UpdatedAt,
		&t.Plan, &t.State, &t.OwnerEmail,
		&suspendedAt, &disabledAt,
		&ingestRate, &queryRate,
		&t.MaxIncidents, &t.MaxEvents,
		&t.EncryptionEnabled, &t.AuditRetainDays,
		&t.DataRegion,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.SuspendedAt = suspendedAt
	t.DisabledAt = disabledAt
	if ingestRate.Valid {
		t.IngestRatePerMin = int(ingestRate.Int64)
	}
	if queryRate.Valid {
		t.QueryRatePerMin = int(queryRate.Int64)
	}
	return &t, nil
}

// List returns all tenants with SaaS fields, ordered by creation date.
func (s *TenantStore) List() ([]models.TenantMaster, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, slug,
		       COALESCE(deployment_mode,'cloud'),
		       COALESCE(industry_type,'general'),
		       COALESCE(default_currency,'USD'),
		       COALESCE(timezone,'UTC'),
		       COALESCE(estimation_mode,'balanced'),
		       created_at, COALESCE(updated_at, created_at),
		       COALESCE(plan,'trial'), COALESCE(state,'active'),
		       COALESCE(owner_email,''),
		       suspended_at, disabled_at,
		       ingest_rate_min, query_rate_min,
		       COALESCE(max_incidents,0), COALESCE(max_events,0),
		       COALESCE(encryption_enabled,false),
		       COALESCE(audit_retain_days,90),
		       COALESCE(data_region,'us')
		FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []models.TenantMaster
	for rows.Next() {
		var t models.TenantMaster
		var suspendedAt, disabledAt *time.Time
		var ingestRate, queryRate sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug,
			&t.DeploymentMode, &t.IndustryType, &t.DefaultCurrency,
			&t.Timezone, &t.EstimationMode,
			&t.CreatedAt, &t.UpdatedAt,
			&t.Plan, &t.State, &t.OwnerEmail,
			&suspendedAt, &disabledAt,
			&ingestRate, &queryRate,
			&t.MaxIncidents, &t.MaxEvents,
			&t.EncryptionEnabled, &t.AuditRetainDays,
			&t.DataRegion,
		); err != nil {
			return nil, err
		}
		t.SuspendedAt = suspendedAt
		t.DisabledAt = disabledAt
		if ingestRate.Valid {
			t.IngestRatePerMin = int(ingestRate.Int64)
		}
		if queryRate.Valid {
			t.QueryRatePerMin = int(queryRate.Int64)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// ── Lifecycle management ──────────────────────────────────────────────────────

// SetDataRegion permanently assigns a geographic data residency region to a tenant.
// This should only be called at onboarding — changing region post-onboarding
// requires data migration and must be done outside this method.
func (s *TenantStore) SetDataRegion(id, region string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET data_region=$2, updated_at=$3 WHERE id=$1`,
		id, region, time.Now(),
	)
	return err
}

// GetDataRegion returns the data_region for a tenant (fast single-column query).
func (s *TenantStore) GetDataRegion(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var region string
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(data_region,'us') FROM tenants WHERE id=$1`, id,
	).Scan(&region)
	if region == "" {
		region = "us"
	}
	return region
}

// SetState transitions a tenant to a new lifecycle state.
// Valid transitions: active ↔ suspended, active → disabled.
func (s *TenantStore) SetState(id, state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	switch state {
	case "suspended":
		_, err := s.db.ExecContext(ctx, `UPDATE tenants SET state=$2, suspended_at=$3, updated_at=$3 WHERE id=$1`, id, state, now)
		return err
	case "disabled":
		_, err := s.db.ExecContext(ctx, `UPDATE tenants SET state=$2, disabled_at=$3, updated_at=$3 WHERE id=$1`, id, state, now)
		return err
	case "active":
		_, err := s.db.ExecContext(ctx, `UPDATE tenants SET state=$2, suspended_at=NULL, updated_at=$3 WHERE id=$1`, id, state, now)
		return err
	default:
		return fmt.Errorf("unknown tenant state: %s", state)
	}
}

// SetPlan updates a tenant's subscription plan.
func (s *TenantStore) SetPlan(id, plan string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `UPDATE tenants SET plan=$2, updated_at=$3 WHERE id=$1`, id, plan, time.Now())
	return err
}

// SetRateLimits applies per-tenant rate limit overrides (0 = use plan defaults).
func (s *TenantStore) SetRateLimits(id string, ingestPerMin, queryPerMin int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var ingest, query any = nil, nil
	if ingestPerMin > 0 {
		ingest = ingestPerMin
	}
	if queryPerMin > 0 {
		query = queryPerMin
	}
	_, err := s.db.ExecContext(ctx, 
		`UPDATE tenants SET ingest_rate_min=$2, query_rate_min=$3, updated_at=$4 WHERE id=$1`,
		id, ingest, query, time.Now(),
	)
	return err
}

// SetEncryption enables or disables field-level encryption for a tenant.
func (s *TenantStore) SetEncryption(id string, enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `UPDATE tenants SET encryption_enabled=$2, updated_at=$3 WHERE id=$1`, id, enabled, time.Now())
	return err
}

// GetEffectiveLimits returns the effective rate limit profile, merging plan
// defaults with per-tenant overrides stored in the DB.
func (s *TenantStore) GetEffectiveLimits(id string) (models.RateLimitProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var plan string
	var ingestRate, queryRate sql.NullInt64
	err := s.db.QueryRowContext(ctx, 
		`SELECT COALESCE(plan,'trial'), ingest_rate_min, query_rate_min FROM tenants WHERE id=$1`, id,
	).Scan(&plan, &ingestRate, &queryRate)
	if err != nil {
		return models.RateLimitProfile{}, err
	}
	profile := models.PlanDefaults(plan)
	if ingestRate.Valid && ingestRate.Int64 > 0 {
		profile.IngestPerMin = int(ingestRate.Int64)
	}
	if queryRate.Valid && queryRate.Int64 > 0 {
		profile.QueryPerMin = int(queryRate.Int64)
	}
	return profile, nil
}

// GetState returns only the lifecycle state of a tenant.
// Returns "active" for unknown tenants (graceful degradation).
func (s *TenantStore) GetState(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var state string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(state,'active') FROM tenants WHERE id=$1`, id).Scan(&state)
	if err != nil {
		return "active"
	}
	return state
}

// GetUsage returns the current resource usage counts for a tenant.
func (s *TenantStore) GetUsage(id string) (models.TenantUsage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u := models.TenantUsage{TenantID: id}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents WHERE tenant_id=$1`, id).Scan(&u.IncidentCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE tenant_id=$1`, id).Scan(&u.EventCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=$1`, id).Scan(&u.UserCount)
	return u, nil
}

// RecordRateLimitEvent increments the throttle event counter for a tenant+category.
func (s *TenantStore) RecordRateLimitEvent(tenantID, category string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = s.db.ExecContext(ctx, 
		`INSERT INTO tenant_rate_limit_events (tenant_id, category, count) VALUES ($1,$2,1)`,
		tenantID, category,
	)
}

// helper
func coalesce(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
