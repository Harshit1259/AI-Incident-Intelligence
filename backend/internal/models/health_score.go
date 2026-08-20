package models

import "time"

// ChurnRisk is the overall churn risk level derived from a tenant's health score.
type ChurnRisk string

const (
	ChurnRiskLow      ChurnRisk = "low"      // score 76-100
	ChurnRiskMedium   ChurnRisk = "medium"   // score 51-75
	ChurnRiskHigh     ChurnRisk = "high"     // score 26-50
	ChurnRiskCritical ChurnRisk = "critical" // score 0-25
)

// HealthSignalStatus is the qualitative label for an individual signal.
type HealthSignalStatus string

const (
	HealthSignalGood    HealthSignalStatus = "good"
	HealthSignalWarning HealthSignalStatus = "warning"
	HealthSignalPoor    HealthSignalStatus = "poor"
)

// HealthSignal represents one of the four scored dimensions of tenant health.
type HealthSignal struct {
	Name     string             `json:"name"`
	Score    int                `json:"score"`
	MaxScore int                `json:"max_score"`
	Value    string             `json:"value"`  // human-readable measurement
	Status   HealthSignalStatus `json:"status"` // "good" | "warning" | "poor"
}

// RecommendedAction is a prescriptive CSM action generated from the lowest signals.
type RecommendedAction struct {
	Type     string `json:"type"`     // "email" | "call" | "review" | "training"
	Priority string `json:"priority"` // "urgent" | "normal"
	Label    string `json:"label"`
	Reason   string `json:"reason"`
}

// TenantHealthScore is the full computed health record for one tenant.
type TenantHealthScore struct {
	TenantID   string              `json:"tenant_id"`
	TenantName string              `json:"tenant_name"`
	Score      int                 `json:"score"`
	MaxScore   int                 `json:"max_score"`
	ChurnRisk  ChurnRisk           `json:"churn_risk"`
	Signals    []HealthSignal      `json:"signals"`
	Actions    []RecommendedAction `json:"recommended_actions"`
	ComputedAt time.Time           `json:"computed_at"`
}

// HealthAction is a CSM action (call, email, note) recorded against a tenant.
type HealthAction struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id"`
	Actor      string    `json:"actor"`
	ActionType string    `json:"action_type"`
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`
}
