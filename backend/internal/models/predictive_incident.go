package models

import "time"

// MetricDataPoint is a single timestamped measurement of a metric.
type MetricDataPoint struct {
	RecordedAt time.Time `json:"recorded_at"`
	Value      float64   `json:"value"`
}

// TrendAnalysis holds the result of a weighted linear regression over recent metric data.
type TrendAnalysis struct {
	Slope      float64 `json:"slope"`       // change per minute (positive = rising)
	Intercept  float64 `json:"intercept"`   // projected value at t=0 (oldest point)
	RSquared   float64 `json:"r_squared"`   // goodness of fit 0–1
	StdError   float64 `json:"std_error"`   // std error of prediction (in metric units)
	DataPoints int     `json:"data_points"` // number of points used
}

// PredictiveIncident is a forward-looking warning raised before an SLO actually breaches.
type PredictiveIncident struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	Service            string     `json:"service"`
	MetricName         string     `json:"metric_name"`
	CurrentValue       float64    `json:"current_value"`
	SLOThreshold       float64    `json:"slo_threshold"`
	TrendSlope         float64    `json:"trend_slope"`         // change per minute
	BreachProbability  float64    `json:"breach_probability"`  // 0–100
	ConfidenceInterval float64    `json:"confidence_interval"` // e.g. 95
	PredictedBreachMin int        `json:"predicted_breach_at_min"` // lower bound minutes
	PredictedBreachMax int        `json:"predicted_breach_at_max"` // upper bound minutes
	Status             string     `json:"status"`              // open | resolved | false_alarm
	Message            string     `json:"message"`
	DataPointsUsed     int        `json:"data_points_used"`
	CreatedAt          time.Time  `json:"created_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
}

type PredictiveEvaluateRequest struct {
	Service       string  `json:"service"`
	MetricName    string  `json:"metric_name"`
	Threshold     float64 `json:"threshold"`
	WindowMinutes int     `json:"window_minutes"`
}

type MetricRecordRequest struct {
	Service    string  `json:"service"`
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
}
