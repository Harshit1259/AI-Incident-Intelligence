package models

import "time"

// Runbook is a stored operational playbook for incident response.
type Runbook struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Service       string    `json:"service"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          string    `json:"tags"`
	SeverityMatch string    `json:"severity_match"`
	PatternMatch  string    `json:"pattern_match"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RunbookCreateRequest is the API request body for creating/updating runbooks.
type RunbookCreateRequest struct {
	Service       string `json:"service"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Tags          string `json:"tags"`
	SeverityMatch string `json:"severity_match"`
	PatternMatch  string `json:"pattern_match"`
}

// AutoResolveRule defines criteria for automatically resolving incidents.
type AutoResolveRule struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	Name            string    `json:"name"`
	Pattern         string    `json:"pattern"`
	Service         string    `json:"service"`
	Severity        string    `json:"severity"`
	Action          string    `json:"action"` // resolve, ack
	CooldownMinutes int       `json:"cooldown_minutes"`
	Enabled         bool      `json:"enabled"`
	TimesFired      int       `json:"times_fired"`
	CreatedAt       time.Time `json:"created_at"`
}

// AutoResolveRuleRequest is the API request body for creating auto-resolve rules.
type AutoResolveRuleRequest struct {
	Name            string `json:"name"`
	Pattern         string `json:"pattern"`
	Service         string `json:"service"`
	Severity        string `json:"severity"`
	Action          string `json:"action"`
	CooldownMinutes int    `json:"cooldown_minutes"`
}

// DependencyCatalogEntry represents a service dependency in the catalog.
type DependencyCatalogEntry struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Service        string    `json:"service"`
	DependencyName string    `json:"dependency_name"`
	DependencyType string    `json:"dependency_type"` // internal, vendor, third_party
	Vendor         string    `json:"vendor"`
	HealthURL      string    `json:"health_url"`
	StatusPageURL  string    `json:"status_page_url"`
	CreatedAt      time.Time `json:"created_at"`
}

// DependencyAttribution attributes an incident to a specific dependency.
type DependencyAttribution struct {
	IncidentID     string `json:"incident_id"`
	Attribution    string `json:"attribution"` // internal, vendor, third_party, unknown
	DependencyName string `json:"dependency_name"`
	Vendor         string `json:"vendor"`
	Confidence     int    `json:"confidence"` // 0-100
	Reason         string `json:"reason"`
}

// WhatsAppConfig stores WhatsApp notification settings for a tenant.
type WhatsAppConfig struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	PhoneNumberID string    `json:"phone_number_id"`
	AccessToken   string    `json:"access_token,omitempty"`
	VerifyToken   string    `json:"verify_token,omitempty"`
	NotifyOn      string    `json:"notify_on"` // critical, high, all
	Recipients    []string  `json:"recipients"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

// ComplianceReport is a regulatory/compliance-ready summary of incident data.
type ComplianceReport struct {
	TenantID       string              `json:"tenant_id"`
	PeriodStart    time.Time           `json:"period_start"`
	PeriodEnd      time.Time           `json:"period_end"`
	TotalIncidents int                 `json:"total_incidents"`
	MTTR           float64             `json:"mttr_seconds"`
	MTTA           float64             `json:"mtta_seconds"`
	SLOCompliance  []SLOComplianceLine `json:"slo_compliance"`
	UptimePercent  float64             `json:"uptime_percent"`
	BreachedSLOs   int                 `json:"breached_slos"`
	GeneratedAt    time.Time           `json:"generated_at"`
}

// SLOComplianceLine is a single row in the compliance report's SLO section.
type SLOComplianceLine struct {
	SLOName         string  `json:"slo_name"`
	Service         string  `json:"service"`
	Target          float64 `json:"target_percent"`
	Actual          float64 `json:"actual_percent"`
	Compliant       bool    `json:"compliant"`
	ErrorBudgetUsed float64 `json:"error_budget_used_percent"`
}
