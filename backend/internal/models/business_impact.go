package models

import "time"

// ServiceProfile holds financial/business context for a service.
type ServiceProfile struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	Service                string    `json:"service"`
	Tier                   string    `json:"tier"` // TIER_0, TIER_1, TIER_2, TIER_3
	CostModelType          string    `json:"cost_model_type"` // revenue, productivity, sla, infra, mixed
	HourlyRevenue          float64   `json:"hourly_revenue"`
	TransactionsPerHour    float64   `json:"transactions_per_hour"`
	AvgOrderValue          float64   `json:"avg_order_value"`
	UsersPerHour           float64   `json:"users_per_hour"`
	EmployeeCostPerHour    float64   `json:"employee_cost_per_hour"`
	SLAPenaltyPerMinute    float64   `json:"sla_penalty_per_minute"`
	SLAThresholdMinutes    int       `json:"sla_threshold_minutes"`
	BusinessHourMultiplier float64   `json:"business_hour_multiplier"`
	PeakMultiplier         float64   `json:"peak_multiplier"`
	InfraCostPerHour       float64   `json:"infra_cost_per_hour"`
	ConfidenceMode         string    `json:"confidence_mode"` // conservative, balanced, aggressive
	Currency               string    `json:"currency"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// IncidentBaseline stores historical MTTR for counterfactual estimation.
type IncidentBaseline struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	Service           string    `json:"service"`
	IncidentType      string    `json:"incident_type"`
	AvgMTTRMinutes    float64   `json:"avg_mttr_minutes"`
	MedianMTTRMinutes float64   `json:"median_mttr_minutes"`
	P90MTTRMinutes    float64   `json:"p90_mttr_minutes"`
	SampleSize        int       `json:"sample_size"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// FinancialEstimate is the computed impact for a single incident.
type FinancialEstimate struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	IncidentID         string    `json:"incident_id"`
	Service            string    `json:"service"`
	ActualLoss         float64   `json:"actual_loss"`
	CounterfactualLoss float64   `json:"counterfactual_loss"`
	AvoidedLoss        float64   `json:"avoided_loss"`
	ConfidenceLevel    string    `json:"confidence_level"` // HIGH, MEDIUM, LOW
	ConfidenceScore    float64   `json:"confidence_score"` // 1.0, 0.7, 0.4
	MethodUsed         string    `json:"method_used"`      // historical_baseline, static_fallback
	Currency           string    `json:"currency"`
	BreakdownJSON      string    `json:"breakdown_json"`
	Explanation        string    `json:"explanation"`
	CreatedAt          time.Time `json:"created_at"`
}

// LossBreakdown is the detailed calculation for auditability.
type LossBreakdown struct {
	// Actual
	ActualDurationMinutes  float64 `json:"actual_duration_minutes"`
	ActualRevenueLoss      float64 `json:"actual_revenue_loss"`
	ActualSLAPenalty       float64 `json:"actual_sla_penalty"`
	ActualProductivityLoss float64 `json:"actual_productivity_loss"`
	ActualInfraLoss        float64 `json:"actual_infra_loss"`
	TotalActualLoss        float64 `json:"total_actual_loss"`

	// Counterfactual
	CounterfactualDurationMinutes  float64 `json:"counterfactual_duration_minutes"`
	Phase1Minutes                  float64 `json:"phase1_minutes"`
	Phase2Minutes                  float64 `json:"phase2_minutes"`
	Phase1Impact                   float64 `json:"phase1_impact"`
	Phase2Impact                   float64 `json:"phase2_impact"`
	CounterfactualRevenueLoss      float64 `json:"counterfactual_revenue_loss"`
	CounterfactualSLAPenalty       float64 `json:"counterfactual_sla_penalty"`
	CounterfactualProductivityLoss float64 `json:"counterfactual_productivity_loss"`
	CounterfactualInfraLoss        float64 `json:"counterfactual_infra_loss"`
	TotalCounterfactualLoss        float64 `json:"total_counterfactual_loss"`

	// Result
	AvoidedLoss        float64 `json:"avoided_loss"`
	SeverityMultiplier float64 `json:"severity_multiplier"`
	TierMultiplier     float64 `json:"tier_multiplier"`
}

// RiskProjection shows "if unresolved for X more minutes" cost.
type RiskProjection struct {
	MinutesExtra float64 `json:"minutes_extra"`
	Label        string  `json:"label"`
	EstimatedLoss float64 `json:"estimated_loss"`
}

// MonthlyRollup aggregates financial estimates for a month.
type MonthlyRollup struct {
	ID                      string    `json:"id"`
	TenantID                string    `json:"tenant_id"`
	Year                    int       `json:"year"`
	Month                   int       `json:"month"`
	TotalActualLoss         float64   `json:"total_actual_loss"`
	TotalCounterfactualLoss float64   `json:"total_counterfactual_loss"`
	TotalAvoidedLoss        float64   `json:"total_avoided_loss"`
	ConfidenceWeightedLoss  float64   `json:"confidence_weighted_loss"`
	Currency                string    `json:"currency"`
	TopIncidentsJSON        string    `json:"top_incidents_json"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// LiveBusinessImpact is the real-time dollar breakdown embedded in every incident detail
// response. Recomputed on each request — figures are always current, never cached.
type LiveBusinessImpact struct {
	// Running meter
	DurationMinutes  float64 `json:"duration_minutes"`
	RevenuePerMinute float64 `json:"revenue_per_minute"`  // $/min at current impact %
	TotalRevenueLoss float64 `json:"total_revenue_loss"`  // cumulative so far

	// Customer exposure
	AffectedSessions int `json:"affected_sessions"` // users/hr × impact %

	// SLA exposure
	SLATierLabel       string  `json:"sla_tier_label"`         // "Platinum" | "Gold" | …
	SLABreachInMinutes float64 `json:"sla_breach_in_minutes"`  // >0 countdown, <0 already breached by N min
	SLAAlreadyBreached bool    `json:"sla_already_breached"`
	SLAPenaltyAccrued  float64 `json:"sla_penalty_accrued"`    // penalty so far (if already breached)
	SLAPenaltyPerMin   float64 `json:"sla_penalty_per_min"`    // ongoing per-minute rate

	// Engineering cost
	EngineerCount        int     `json:"engineer_count"`
	EngineeringCostSoFar float64 `json:"engineering_cost_so_far"`

	// Other components
	InfraCostSoFar        float64 `json:"infra_cost_so_far"`
	ProductivityLossSoFar float64 `json:"productivity_loss_so_far"`

	// Running total (all components combined)
	TotalCostSoFar float64 `json:"total_cost_so_far"`

	// "+15/+30/+60 min" projections if incident stays open
	Projections []RiskProjection `json:"projections"`

	// Metadata
	Currency   string    `json:"currency"`
	ComputedAt time.Time `json:"computed_at"`

	// Human-readable lines for the detail-view UI panel
	NarrativeLines []string `json:"narrative_lines"`
}

// BusinessImpactResponse is the full response for the incident business impact tab.
type BusinessImpactResponse struct {
	Estimate    *FinancialEstimate `json:"estimate"`
	Breakdown   *LossBreakdown     `json:"breakdown"`
	Profile     *ServiceProfile    `json:"profile"`
	Baseline    *IncidentBaseline  `json:"baseline,omitempty"`
	Incident    *Incident          `json:"incident,omitempty"`
	Projections []RiskProjection   `json:"projections"`
}

// MonthlyReportResponse is the response for GET /reports/monthly
type MonthlyReportResponse struct {
	Rollup         *MonthlyRollup    `json:"rollup"`
	Estimates      []FinancialEstimate `json:"estimates"`
	TotalIncidents int               `json:"total_incidents"`
}
