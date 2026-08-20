package models

import "time"

// ChangeEvent is the enriched representation of a row in the `changes` table.
// The original store.ChangeRecord (minimal struct) is kept for backward compat
// in existing correlation code; this type is used by the change intelligence layer.
type ChangeEvent struct {
	ID                int               `json:"id"`
	TenantID          string            `json:"tenant_id"`
	Service           string            `json:"service"`
	// Type: deployment | commit | release | config_change | infra | rollback | feature_flag
	Type              string            `json:"type"`
	Environment       string            `json:"environment"`
	Version           string            `json:"version"`
	Description       string            `json:"description"`
	CommitSHA         string            `json:"commit_sha"`
	Author            string            `json:"author"`
	PRNumber          string            `json:"pr_number"`
	ChangedFilesCount int               `json:"changed_files_count"`
	// ChangeSource: github | gitlab | api | manual | terraform | feature_flag
	ChangeSource      string            `json:"change_source"`
	Metadata          map[string]string `json:"metadata"`
	CorrelationScore  int               `json:"correlation_score"`
	LinkedIncidentID  string            `json:"linked_incident_id"`
	Timestamp         time.Time         `json:"timestamp"`
}

// FeatureFlagChange records a feature flag toggle for incident correlation.
type FeatureFlagChange struct {
	ID               int       `json:"id"`
	TenantID         string    `json:"tenant_id"`
	FlagName         string    `json:"flag_name"`
	FlagKey          string    `json:"flag_key"`
	Environment      string    `json:"environment"`
	OldValue         string    `json:"old_value"`
	NewValue         string    `json:"new_value"`
	ChangedBy        string    `json:"changed_by"`
	// AffectedPct is the percentage of traffic/users affected by the flag change.
	AffectedPct      int       `json:"affected_pct"`
	Timestamp        time.Time `json:"timestamp"`
	LinkedIncidentID string    `json:"linked_incident_id"`
	CorrelationScore int       `json:"correlation_score"`
}

// ConfigDriftEvent is detected whenever a config value deviates from its baseline.
type ConfigDriftEvent struct {
	ID               int       `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Service          string    `json:"service"`
	ConfigKey        string    `json:"config_key"`
	BaselineValue    string    `json:"baseline_value"`
	CurrentValue     string    `json:"current_value"`
	// DriftSeverity: low | medium | high | critical
	DriftSeverity    string    `json:"drift_severity"`
	DetectedAt       time.Time `json:"detected_at"`
	LinkedIncidentID string    `json:"linked_incident_id"`
}

// ChangeTimelineEntry is one event in the "what changed first" causality timeline.
type ChangeTimelineEntry struct {
	// EntryType: deployment | commit | release | feature_flag | config_drift | infra | rollback
	EntryType        string            `json:"entry_type"`
	ChangeID         string            `json:"change_id"`
	Service          string            `json:"service"`
	Environment      string            `json:"environment"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	Author           string            `json:"author"`
	Version          string            `json:"version"`
	Timestamp        time.Time         `json:"timestamp"`
	CorrelationScore int               `json:"correlation_score"`
	// LagFromIncident is signed time from this change to the incident's first event.
	// Negative = change happened before the incident; positive = after.
	LagFromIncident  string            `json:"lag_from_incident"`
	// LagSeconds is the raw signed integer, useful for sorting/thresholds.
	LagSeconds       int               `json:"lag_seconds"`
	IsFirstChange    bool              `json:"is_first_change"`
	IsPrimaryCorr    bool              `json:"is_primary_correlation"`
	Metadata         map[string]string `json:"metadata"`
}

// ChangeIntelligenceReport is the full change context assembled for an incident.
type ChangeIntelligenceReport struct {
	IncidentID         string                `json:"incident_id"`
	Service            string                `json:"service"`
	WindowStart        time.Time             `json:"window_start"`
	WindowEnd          time.Time             `json:"window_end"`
	Deployments        []ChangeEvent         `json:"deployments"`
	FeatureFlagChanges []FeatureFlagChange   `json:"feature_flag_changes"`
	ConfigDrift        []ConfigDriftEvent    `json:"config_drift"`
	InfraChanges       []ChangeEvent         `json:"infra_changes"`
	CausalityTimeline  []ChangeTimelineEntry `json:"causality_timeline"`
	FirstChange        *ChangeTimelineEntry  `json:"first_change,omitempty"`
	PrimaryCorrelation *ChangeEvent          `json:"primary_correlation,omitempty"`
	OverallConfidence  int                   `json:"overall_confidence"`
	GeneratedAt        string                `json:"generated_at"`
}
