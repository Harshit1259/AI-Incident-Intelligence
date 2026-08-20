package models

import "time"

type IncidentMetrics struct {
	ID             int        `json:"id"`
	IncidentID     string     `json:"incident_id"`
	TenantID       string     `json:"tenant_id"`
	DetectedAt     *time.Time `json:"detected_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	TTDSeconds     int        `json:"ttd_seconds"`
	TTASeconds     int        `json:"tta_seconds"`
	TTRSeconds     int        `json:"ttr_seconds"`
	Responder      string     `json:"responder"`
	IsAutoResolved bool       `json:"is_auto_resolved"`
	ToilMinutes    int        `json:"toil_minutes"`
}

type EngineeringHealthSummary struct {
	TenantID       string           `json:"tenant_id"`
	PeriodDays     int              `json:"period_days"`
	TotalIncidents int              `json:"total_incidents"`
	MTTR           float64          `json:"mttr_seconds"`
	MTTA           float64          `json:"mtta_seconds"`
	MTTD           float64          `json:"mttd_seconds"`
	AutoResolved   int              `json:"auto_resolved"`
	TotalToilHours float64          `json:"total_toil_hours"`
	TopResponders  []ResponderStats `json:"top_responders"`
	BurnoutRisk    []BurnoutFlag    `json:"burnout_risk"`
	ByTeam         []TeamHealth     `json:"by_team,omitempty"`
}

type ResponderStats struct {
	Responder     string  `json:"responder"`
	IncidentCount int     `json:"incident_count"`
	OnCallHours   float64 `json:"on_call_hours"`
	ToilHours     float64 `json:"toil_hours"`
	AvgTTRSeconds float64 `json:"avg_ttr_seconds"`
}

type BurnoutFlag struct {
	Responder string `json:"responder"`
	Reason    string `json:"reason"`
	RiskLevel string `json:"risk_level"` // low, medium, high
}

type TeamHealth struct {
	TeamName      string  `json:"team_name"`
	IncidentCount int     `json:"incident_count"`
	MTTR          float64 `json:"mttr_seconds"`
	ToilHours     float64 `json:"toil_hours"`
}
