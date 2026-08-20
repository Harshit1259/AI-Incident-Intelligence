package store

import (
	"context"
	"time"
	"database/sql"
	"encoding/json"

	"ai-incident-platform/backend/internal/models"
)

// BillingStore handles billing_plans and tenant_subscriptions persistence.
type BillingStore struct {
	db *sql.DB
}

func NewBillingStore(db *sql.DB) *BillingStore {
	return &BillingStore{db: db}
}

// GetPlans returns all active billing plans.
func (s *BillingStore) GetPlans() ([]models.BillingPlan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, name, tier, price_cents, max_incidents, max_users, max_services, features_json, active
		 FROM billing_plans WHERE active = true ORDER BY price_cents ASC LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]models.BillingPlan, 0)
	for rows.Next() {
		var p models.BillingPlan
		var featuresJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Tier, &p.PriceCents, &p.MaxIncidents, &p.MaxUsers, &p.MaxServices, &featuresJSON, &p.Active); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(featuresJSON), &p.Features); err != nil {
			p.Features = []string{}
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

// GetPlanByID returns a single billing plan by ID.
func (s *BillingStore) GetPlanByID(id string) (*models.BillingPlan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, 
		`SELECT id, name, tier, price_cents, max_incidents, max_users, max_services, features_json, active
		 FROM billing_plans WHERE id = $1`, id,
	)
	var p models.BillingPlan
	var featuresJSON string
	err := row.Scan(&p.ID, &p.Name, &p.Tier, &p.PriceCents, &p.MaxIncidents, &p.MaxUsers, &p.MaxServices, &featuresJSON, &p.Active)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(featuresJSON), &p.Features); err != nil {
		p.Features = []string{}
	}
	return &p, nil
}

// GetSubscription returns the subscription for a tenant. Returns nil if none exists.
func (s *BillingStore) GetSubscription(tenantID string) (*models.TenantSubscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, plan_id, status, stripe_customer_id, stripe_sub_id,
		        current_period_start, current_period_end, created_at, updated_at
		 FROM tenant_subscriptions WHERE tenant_id = $1`, tenantID,
	)

	var sub models.TenantSubscription
	err := row.Scan(
		&sub.ID, &sub.TenantID, &sub.PlanID, &sub.Status,
		&sub.StripeCustomerID, &sub.StripeSubID,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Attach plan details
	plan, err := s.GetPlanByID(sub.PlanID)
	if err == nil && plan != nil {
		sub.Plan = plan
	}

	return &sub, nil
}

// UpsertSubscription creates or updates a tenant subscription.
func (s *BillingStore) UpsertSubscription(sub models.TenantSubscription) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO tenant_subscriptions (id, tenant_id, plan_id, status, stripe_customer_id, stripe_sub_id, current_period_start, current_period_end, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   plan_id = EXCLUDED.plan_id,
		   status = EXCLUDED.status,
		   stripe_customer_id = EXCLUDED.stripe_customer_id,
		   stripe_sub_id = EXCLUDED.stripe_sub_id,
		   current_period_start = EXCLUDED.current_period_start,
		   current_period_end = EXCLUDED.current_period_end,
		   updated_at = EXCLUDED.updated_at`,
		sub.ID, sub.TenantID, sub.PlanID, sub.Status,
		sub.StripeCustomerID, sub.StripeSubID,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt,
	)
	return err
}

// GetUsageSummary computes current usage for a tenant against their plan.
func (s *BillingStore) GetUsageSummary(tenantID string) (*models.UsageSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var incidentCount, userCount, serviceCount int

	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents WHERE tenant_id = $1`, tenantID).Scan(&incidentCount)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&userCount)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT service) FROM incidents WHERE tenant_id = $1`, tenantID).Scan(&serviceCount)
	if err != nil {
		return nil, err
	}

	// Get the tenant's plan
	sub, err := s.GetSubscription(tenantID)
	if err != nil {
		return nil, err
	}

	var plan models.BillingPlan
	if sub != nil && sub.Plan != nil {
		plan = *sub.Plan
	} else {
		// Default to starter plan
		p, err := s.GetPlanByID("plan_starter")
		if err != nil || p == nil {
			plan = models.BillingPlan{
				ID: "plan_starter", Name: "Starter", Tier: "starter",
				MaxIncidents: 100, MaxUsers: 3, MaxServices: 5,
			}
		} else {
			plan = *p
		}
	}

	// Compute usage percent based on the most constrained resource
	usagePercent := 0
	atLimit := false

	if plan.MaxIncidents > 0 {
		pct := (incidentCount * 100) / plan.MaxIncidents
		if pct > usagePercent {
			usagePercent = pct
		}
		if incidentCount >= plan.MaxIncidents {
			atLimit = true
		}
	}
	if plan.MaxUsers > 0 {
		pct := (userCount * 100) / plan.MaxUsers
		if pct > usagePercent {
			usagePercent = pct
		}
		if userCount >= plan.MaxUsers {
			atLimit = true
		}
	}
	if plan.MaxServices > 0 {
		pct := (serviceCount * 100) / plan.MaxServices
		if pct > usagePercent {
			usagePercent = pct
		}
		if serviceCount >= plan.MaxServices {
			atLimit = true
		}
	}

	if usagePercent > 100 {
		usagePercent = 100
	}

	return &models.UsageSummary{
		TenantID:      tenantID,
		IncidentCount: incidentCount,
		UserCount:     userCount,
		ServiceCount:  serviceCount,
		PlanLimit:     plan,
		AtLimit:       atLimit,
		UpgradeNeeded: atLimit && plan.Tier != "scale",
		UsagePercent:  usagePercent,
	}, nil
}
