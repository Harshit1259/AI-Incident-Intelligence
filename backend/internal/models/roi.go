package models

import "time"

type ROIDashboard struct {
	TenantID          string   `json:"tenant_id"`
	PeriodDays        int      `json:"period_days"`
	TotalIncidents    int      `json:"total_incidents"`
	AutoResolved      int      `json:"auto_resolved"`
	HoursSaved        float64  `json:"hours_saved"`
	EngineerTimeSaved float64  `json:"engineer_time_saved_hours"`
	SRECostAvoided    float64  `json:"sre_cost_avoided_usd"`
	MTTRReduction     float64  `json:"mttr_reduction_percent"`
	IncidentReduction float64  `json:"incident_reduction_percent"`
	CostPerIncident   float64  `json:"cost_per_incident_usd"`
	MonthlyROI        float64  `json:"monthly_roi_usd"`
	TopWins           []ROIWin `json:"top_wins"`
}

type ROIWin struct {
	Label       string  `json:"label"`
	Value       float64 `json:"value"`
	Description string  `json:"description"`
}

type WeeklyDigest struct {
	TenantID         string           `json:"tenant_id"`
	WeekStart        time.Time        `json:"week_start"`
	WeekEnd          time.Time        `json:"week_end"`
	TotalIncidents   int              `json:"total_incidents"`
	CriticalCount    int              `json:"critical_count"`
	MTTRSeconds      float64          `json:"mttr_seconds"`
	MTTRTrend        string           `json:"mttr_trend"` // improving, stable, degrading
	ReliabilityScore float64          `json:"reliability_score"`
	TopRecurring     []RecurringIssue `json:"top_recurring"`
	Highlights       []string         `json:"highlights"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

type RecurringIssue struct {
	Pattern  string `json:"pattern"`
	Service  string `json:"service"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}
