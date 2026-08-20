package models

// RiskExposureDashboard is a live aggregate snapshot of incident risk across all open incidents.
type RiskExposureDashboard struct {
	TenantID            string           `json:"tenant_id"`
	ComputedAt          string           `json:"computed_at"`
	TotalOpenIncidents  int              `json:"total_open_incidents"`
	CriticalCount       int              `json:"critical_count"`
	HighCount           int              `json:"high_count"`
	MediumCount         int              `json:"medium_count"`
	LowCount            int              `json:"low_count"`
	TotalBlastRadius    int              `json:"total_blast_radius"`    // unique impacted services across all open incidents
	EstimatedImpactUSD  float64          `json:"estimated_impact_usd"`  // sum of per-incident severity-weighted impact
	MTTRTrendMinutes    float64          `json:"mttr_trend_minutes"`    // rolling 7-day average MTTR
	UnackedCritical     int              `json:"unacked_critical"`      // critical incidents without acknowledgement
	AtRiskServices      []AtRiskService  `json:"at_risk_services"`
	ExposureTrend       []ExposurePoint  `json:"exposure_trend"`        // last 7 days daily open count
}

// AtRiskService represents a service currently experiencing or recently affected by incidents.
type AtRiskService struct {
	Service         string  `json:"service"`
	OpenIncidents   int     `json:"open_incidents"`
	MaxSeverity     string  `json:"max_severity"`
	IncidentCount7d int     `json:"incident_count_7d"`
	AvgTTRMinutes   float64 `json:"avg_ttr_minutes"`
	RiskScore       int     `json:"risk_score"` // 0-100
	IsCustomerFacing bool   `json:"is_customer_facing"`
}

// ExposurePoint is a daily open-incident count for trend visualisation.
type ExposurePoint struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Open  int    `json:"open"`
	New   int    `json:"new"`
	Resolved int `json:"resolved"`
}
