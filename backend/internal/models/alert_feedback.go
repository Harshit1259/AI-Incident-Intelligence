package models

import "time"

// AlertFeedback stores user feedback on individual alerts.
type AlertFeedback struct {
	ID          int       `json:"id"`
	EventID     string    `json:"event_id"`
	IncidentID  string    `json:"incident_id"`
	TenantID    string    `json:"tenant_id"`
	Feedback    string    `json:"feedback"` // useful, noise, duplicate, false_positive
	Fingerprint string    `json:"fingerprint"`
	Reason      string    `json:"reason"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// AlertFeedbackRequest is the API request body for submitting feedback.
type AlertFeedbackRequest struct {
	EventID    string `json:"event_id"`
	IncidentID string `json:"incident_id"`
	Feedback   string `json:"feedback"`
	Reason     string `json:"reason"`
}

// AlertFeedbackStats aggregates feedback statistics for a tenant.
type AlertFeedbackStats struct {
	TotalFeedback int          `json:"total_feedback"`
	UsefulCount   int          `json:"useful_count"`
	NoiseCount    int          `json:"noise_count"`
	TopNoisy      []NoisyAlert `json:"top_noisy"`
}

// NoisyAlert represents a frequently-noisy alert fingerprint.
type NoisyAlert struct {
	Fingerprint string `json:"fingerprint"`
	NoiseCount  int    `json:"noise_count"`
	Service     string `json:"service"`
	LastSeen    string `json:"last_seen"`
}

// SourceQualityStat reports alert quality metrics per integration source.
type SourceQualityStat struct {
	Source       string  `json:"source"`
	TotalAlerts  int     `json:"total_alerts"`
	UsefulCount  int     `json:"useful_count"`
	NoiseCount   int     `json:"noise_count"`
	NoiseRatio   float64 `json:"noise_ratio"`   // 0.0–1.0
	QualityScore int     `json:"quality_score"` // 0–100 (100 = all useful)
}
