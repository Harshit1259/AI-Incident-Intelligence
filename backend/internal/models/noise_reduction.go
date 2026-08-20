package models

import "time"

// NoiseReductionScore is the primary dashboard model — shows the full alert-to-incident
// funnel and the time/cost savings attributable to the platform's dedup+correlation engine.
type NoiseReductionScore struct {
	TenantID   string    `json:"tenant_id"`
	WindowDays int       `json:"window_days"`
	ComputedAt time.Time `json:"computed_at"`

	// ── The funnel ───────────────────────────────────────────────────────────
	RawAlertsReceived int     `json:"raw_alerts_received"`   // all ingested events
	AfterDedup        int     `json:"after_dedup"`           // distinct alert fingerprints
	DedupReductionPct float64 `json:"dedup_reduction_pct"`   // % removed by dedup

	AfterCorrelation        int     `json:"after_correlation"`         // incidents created
	CorrelationReductionPct float64 `json:"correlation_reduction_pct"` // % of deduped alerts that were merged into incidents

	TrueIncidents    int     `json:"true_incidents"`    // human-confirmed (acked/resolved)
	ConfirmationRate float64 `json:"confirmation_rate"` // % of incidents that were confirmed

	// ── Savings ──────────────────────────────────────────────────────────────
	NoiseAvoided      int     `json:"noise_avoided"`       // raw - true_incidents
	OnCallHoursSaved  float64 `json:"on_call_hours_saved"` // noiseAvoided × avgHandleMin / 60
	EngineerCostSaved float64 `json:"engineer_cost_saved"` // hours × hourlyRate

	// ── Grade ────────────────────────────────────────────────────────────────
	NoiseReductionPct   float64 `json:"noise_reduction_pct"`  // (raw - true) / raw × 100
	NoiseReductionGrade string  `json:"noise_reduction_grade"` // A / B / C / D

	// ── Human-readable lines (rendered verbatim in the dashboard panel) ─────
	FunnelLines  []FunnelLine `json:"funnel_lines"`
	SavingsLines []string     `json:"savings_lines"`
}

// FunnelLine is one row in the funnel table.
type FunnelLine struct {
	Label     string  `json:"label"`
	Count     int     `json:"count"`
	Delta     string  `json:"delta,omitempty"`      // e.g. "-81.8%"
	DeltaPct  float64 `json:"delta_pct,omitempty"`
	Highlight bool    `json:"highlight,omitempty"` // call-out style
}
