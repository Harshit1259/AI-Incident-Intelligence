package models

import "time"

// AlertRule is a registered alert rule, keyed by fingerprint.
type AlertRule struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	Name          string     `json:"name"`
	Source        string     `json:"source"`
	Service       string     `json:"service"`
	Team          string     `json:"team"`
	Severity      string     `json:"severity"`
	ConditionExpr string     `json:"condition_expr"`
	FirstSeenAt   time.Time  `json:"first_seen_at"`
	LastFiredAt   *time.Time `json:"last_fired_at,omitempty"`
	FireCount7d   int        `json:"fire_count_7d"`
	FireCount30d  int        `json:"fire_count_30d"`
	FPCount       int        `json:"fp_count"`
	NoiseCount    int        `json:"noise_count"`
	IsActive      bool       `json:"is_active"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// AlertRuleRegistration is the request body to register or update an alert rule.
type AlertRuleRegistration struct {
	ID            string `json:"id"` // fingerprint — required
	Name          string `json:"name"`
	Source        string `json:"source"`
	Service       string `json:"service"`
	Team          string `json:"team"`
	Severity      string `json:"severity"`
	ConditionExpr string `json:"condition_expr"`
}

// NoisyAlertRule describes an alert that fires too often relative to its usefulness.
type NoisyAlertRule struct {
	Fingerprint    string  `json:"fingerprint"`
	Title          string  `json:"title"`
	Source         string  `json:"source"`
	Service        string  `json:"service"`
	Team           string  `json:"team"`
	FireCount7d    int     `json:"fire_count_7d"`
	FPCount        int     `json:"fp_count"`
	NoiseCount     int     `json:"noise_count"`
	UsefulCount    int     `json:"useful_count"`
	FPRate         float64 `json:"fp_rate"`    // 0.0–1.0
	NoiseRate      float64 `json:"noise_rate"` // 0.0–1.0
	QualityScore   int     `json:"quality_score"` // 0–100 (lower = worse)
	Recommendation string  `json:"recommendation"` // delete | tune_threshold | consolidate | review
}

// DuplicatePair describes two alerts that consistently co-fire within a short window.
type DuplicatePair struct {
	Fingerprint1       string  `json:"fingerprint1"`
	Title1             string  `json:"title1"`
	Service1           string  `json:"service1"`
	Fingerprint2       string  `json:"fingerprint2"`
	Title2             string  `json:"title2"`
	Service2           string  `json:"service2"`
	CoOccurrenceCount  int     `json:"co_occurrence_count"`
	CoOccurrenceRate   float64 `json:"co_occurrence_rate"` // pct of fires that overlap
	Recommendation     string  `json:"recommendation"`     // merge | add_dedup_window
}

// StaleRule describes an alert rule that hasn't fired recently.
type StaleRule struct {
	Fingerprint       string    `json:"fingerprint"`
	Title             string    `json:"title"`
	Source            string    `json:"source"`
	Service           string    `json:"service"`
	LastSeen          time.Time `json:"last_seen"`
	DaysSinceLastFire int       `json:"days_since_last_fire"`
	TotalFireCount    int       `json:"total_fire_count"`
	Recommendation    string    `json:"recommendation"` // archive | review_condition
}

// TeamAlertDebt aggregates quality debt for a team/service.
type TeamAlertDebt struct {
	Service           string `json:"service"`
	Team              string `json:"team"`
	TotalAlerts30d    int    `json:"total_alerts_30d"`
	UniqueRules       int    `json:"unique_rules"`
	NoisyCount        int    `json:"noisy_count"`
	StaleCount        int    `json:"stale_count"`
	DuplicateCount    int    `json:"duplicate_count"`
	FPFeedbackCount   int    `json:"fp_feedback_count"`
	NoiseFeedbackCount int   `json:"noise_feedback_count"`
	DebtScore         int    `json:"debt_score"`         // 0–100 (higher = worse)
	TopRecommendation string `json:"top_recommendation"`
}

// AlertQualitySummary is a quick stats block at the top of the report.
type AlertQualitySummary struct {
	TotalFired30d        int    `json:"total_fired_30d"`
	UniqueRules          int    `json:"unique_rules"`
	NoisyCount           int    `json:"noisy_count"`
	DuplicatePairCount   int    `json:"duplicate_pair_count"`
	StaleCount           int    `json:"stale_count"`
	HighFPCount          int    `json:"high_fp_count"`
	TotalRecommendations int    `json:"total_recommendations"`
	EstimatedNoisePct    int    `json:"estimated_noise_pct"` // % of fires estimated as noise
}

// AlertQualityReport is the full governance report.
type AlertQualityReport struct {
	TenantID    string              `json:"tenant_id"`
	GeneratedAt string              `json:"generated_at"`
	WindowDays  int                 `json:"window_days"`
	Summary     AlertQualitySummary `json:"summary"`
	NoisyAlerts []NoisyAlertRule    `json:"noisy_alerts"`
	Duplicates  []DuplicatePair     `json:"duplicates"`
	StaleRules  []StaleRule         `json:"stale_rules"`
	AlertDebt   []TeamAlertDebt     `json:"alert_debt"`
}
