package models

import "time"

// BillingPlan represents a billing tier available to tenants.
type BillingPlan struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Tier         string   `json:"tier"`
	PriceCents   int      `json:"price_cents"`
	MaxIncidents int      `json:"max_incidents"` // -1 = unlimited
	MaxUsers     int      `json:"max_users"`
	MaxServices  int      `json:"max_services"`
	Features     []string `json:"features"`
	Active       bool     `json:"active"`
}

// TenantSubscription tracks which plan a tenant is on.
type TenantSubscription struct {
	ID                 string       `json:"id"`
	TenantID           string       `json:"tenant_id"`
	PlanID             string       `json:"plan_id"`
	Status             string       `json:"status"` // active, canceled, past_due
	StripeCustomerID   string       `json:"stripe_customer_id,omitempty"`
	StripeSubID        string       `json:"stripe_sub_id,omitempty"`
	CurrentPeriodStart *time.Time   `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *time.Time   `json:"current_period_end,omitempty"`
	Plan               *BillingPlan `json:"plan,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// UsageSummary shows current usage vs plan limits.
type UsageSummary struct {
	TenantID      string      `json:"tenant_id"`
	IncidentCount int         `json:"incident_count"`
	UserCount     int         `json:"user_count"`
	ServiceCount  int         `json:"service_count"`
	PlanLimit     BillingPlan `json:"plan_limit"`
	AtLimit       bool        `json:"at_limit"`
	UpgradeNeeded bool        `json:"upgrade_needed"`
	UsagePercent  int         `json:"usage_percent"`
}

// CheckoutRequest is the JSON body for POST /api/v1/billing/checkout.
type CheckoutRequest struct {
	PlanID string `json:"plan_id"`
}

// CheckoutResponse is returned after a successful checkout.
type CheckoutResponse struct {
	SubscriptionID string `json:"subscription_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	// CheckoutURL is populated when Stripe is configured — frontend must redirect here.
	CheckoutURL string `json:"checkout_url,omitempty"`
}
