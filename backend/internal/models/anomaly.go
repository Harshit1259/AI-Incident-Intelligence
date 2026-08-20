package models

import "time"

type AnomalyAlert struct {
	ID           int       `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Service      string    `json:"service"`
	MetricName   string    `json:"metric_name"`
	CurrentValue float64   `json:"current_value"`
	Threshold    float64   `json:"threshold"`
	Trend        string    `json:"trend"` // rising, falling, spike
	Severity     string    `json:"severity"`
	Message      string    `json:"message"`
	FiredAt      time.Time `json:"fired_at"`
	Acknowledged bool      `json:"acknowledged"`
}
