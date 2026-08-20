package models

import "time"

// PlanTier defines the SaaS subscription tier for a tenant.
type PlanTier string

const (
	PlanTrial      PlanTier = "trial"
	PlanStarter    PlanTier = "starter"
	PlanGrowth     PlanTier = "growth"
	PlanEnterprise PlanTier = "enterprise"
)

// TenantState is the lifecycle state of a tenant account.
type TenantState string

const (
	TenantStateActive    TenantState = "active"
	TenantStateSuspended TenantState = "suspended"
	TenantStateDisabled  TenantState = "disabled"
	TenantStateTrial     TenantState = "trial"
)

// RateLimitProfile holds the effective rate limits for a tenant.
type RateLimitProfile struct {
	IngestPerMin int `json:"ingest_per_min"` // 0 = unlimited
	QueryPerMin  int `json:"query_per_min"`
	MaxIncidents int `json:"max_incidents"` // 0 = unlimited
	MaxEvents    int `json:"max_events"`
}

// PlanDefaults returns the default rate limit profile for a plan tier.
func PlanDefaults(plan string) RateLimitProfile {
	switch plan {
	case string(PlanStarter):
		return RateLimitProfile{IngestPerMin: 500, QueryPerMin: 1000, MaxIncidents: 5_000, MaxEvents: 100_000}
	case string(PlanGrowth):
		return RateLimitProfile{IngestPerMin: 5000, QueryPerMin: 10_000, MaxIncidents: 50_000, MaxEvents: 1_000_000}
	case string(PlanEnterprise):
		return RateLimitProfile{IngestPerMin: 0, QueryPerMin: 0, MaxIncidents: 0, MaxEvents: 0}
	default: // trial
		return RateLimitProfile{IngestPerMin: 50, QueryPerMin: 100, MaxIncidents: 500, MaxEvents: 10_000}
	}
}

// TenantUsage holds current resource usage for a tenant.
type TenantUsage struct {
	TenantID      string `json:"tenant_id"`
	IncidentCount int    `json:"incident_count"`
	EventCount    int    `json:"event_count"`
	UserCount     int    `json:"user_count"`
}

// TenantMaster is the enriched tenant record loaded from DB.
type TenantMaster struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	DeploymentMode  string    `json:"deployment_mode"` // cloud | on_prem | hybrid
	IndustryType    string    `json:"industry_type"`   // fintech | ecommerce | saas | healthcare | manufacturing | general
	DefaultCurrency string    `json:"default_currency"`
	Timezone        string    `json:"timezone"`
	EstimationMode  string    `json:"estimation_mode"` // conservative | balanced | aggressive
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// SaaS multi-tenancy fields (Feature B1)
	Plan              string     `json:"plan"`
	State             string     `json:"state"`
	OwnerEmail        string     `json:"owner_email"`
	SuspendedAt       *time.Time `json:"suspended_at,omitempty"`
	DisabledAt        *time.Time `json:"disabled_at,omitempty"`
	IngestRatePerMin  int        `json:"ingest_rate_per_min"`
	QueryRatePerMin   int        `json:"query_rate_per_min"`
	MaxIncidents      int        `json:"max_incidents"`
	MaxEvents         int        `json:"max_events"`
	EncryptionEnabled bool       `json:"encryption_enabled"`
	AuditRetainDays   int        `json:"audit_retain_days"`

	// Multi-Region Data Residency — SaaS Feature 2
	DataRegion string `json:"data_region"` // "us" | "eu" | "apac"
}

// ServiceCatalogEntry represents a registered service in the tenant's catalog.
type ServiceCatalogEntry struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ServiceName      string    `json:"service_name"`
	Environment      string    `json:"environment"`      // prod | staging | dev
	BusinessUnit     string    `json:"business_unit"`
	Owner            string    `json:"owner"`
	IsCustomerFacing bool      `json:"is_customer_facing"`
	Tier             string    `json:"tier"` // TIER_0 | TIER_1 | TIER_2 | TIER_3
	Region           string    `json:"region"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TenantSettings holds per-tenant overrides for multipliers and calculation preferences.
type TenantSettings struct {
	ID                  string                 `json:"id"`
	TenantID            string                 `json:"tenant_id"`
	SeverityMultipliers map[string]float64     `json:"severity_multipliers"`
	TierMultipliers     map[string]float64     `json:"tier_multipliers"`
	ConfidenceMode      string                 `json:"confidence_mode"` // conservative | balanced | aggressive
	MonthlyReportPrefs  map[string]interface{} `json:"monthly_report_prefs"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// IndustryTemplateInfo describes an available industry template (read-only, for UI).
type IndustryTemplateInfo struct {
	IndustryType string `json:"industry_type"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
}

// ImportResult summarises a CSV bulk-import operation.
type ImportResult struct {
	TotalRows int      `json:"total_rows"`
	Imported  int      `json:"imported"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors"`
}

// OnboardingConfig is the full configuration bundle for a new tenant.
type OnboardingConfig struct {
	Tenant    TenantMaster          `json:"tenant"`
	Services  []ServiceCatalogEntry `json:"services"`
	Profiles  []ServiceProfile      `json:"profiles"`
	Baselines []IncidentBaseline    `json:"baselines"`
	Settings  *TenantSettings       `json:"settings,omitempty"`
}
