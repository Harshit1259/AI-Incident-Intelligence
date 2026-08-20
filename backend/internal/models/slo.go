package models

import "time"

type SLODefinition struct {
	ID            string  `json:"id"`
	TenantID      string  `json:"tenant_id"`
	Service       string  `json:"service"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	TargetPercent float64 `json:"target_percent"`
	WindowDays    int     `json:"window_days"`
	MetricType    string  `json:"metric_type"` // availability, latency, error_rate
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SLOMeasurement struct {
	ID                   int       `json:"id"`
	SLOID                string    `json:"slo_id"`
	Timestamp            time.Time `json:"timestamp"`
	TotalRequests        int64     `json:"total_requests"`
	GoodRequests         int64     `json:"good_requests"`
	BadMinutes           int       `json:"bad_minutes"`
	ErrorBudgetRemaining float64   `json:"error_budget_remaining"`
}

type SLOStatus struct {
	Definition           SLODefinition    `json:"definition"`
	CurrentPercent       float64          `json:"current_percent"`
	ErrorBudgetRemaining float64          `json:"error_budget_remaining"`
	BurnRate             float64          `json:"burn_rate"`
	TimeToBreachHours    float64          `json:"time_to_breach_hours"`
	Status               string           `json:"status"` // healthy, warning, critical, breached
	Measurements         []SLOMeasurement `json:"measurements,omitempty"`
}

type SLOCreateRequest struct {
	Service       string  `json:"service"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	TargetPercent float64 `json:"target_percent"`
	WindowDays    int     `json:"window_days"`
	MetricType    string  `json:"metric_type"`
}
