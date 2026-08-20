package models

// RemediationPattern is a tested, success-tracked way to fix a class of incident.
type RemediationPattern struct {
	ID                 string   `json:"id"`
	TenantID           string   `json:"tenant_id"`
	Service            string   `json:"service"`
	ErrorSignature     string   `json:"error_signature"`
	RootCauseCategory  string   `json:"root_cause_category"`
	RemediationSummary string   `json:"remediation_summary"`
	RemediationSteps   []string `json:"remediation_steps"`
	SuccessCount       int      `json:"success_count"`
	FailureCount       int      `json:"failure_count"`
	AvgResolutionMins  float64  `json:"avg_resolution_mins"`
	SuccessRate        float64  `json:"success_rate"`
	LastUsedAt         string   `json:"last_used_at"`
	CreatedAt          string   `json:"created_at"`
}

// DeploySignature captures a known-bad deploy pattern that historically caused incidents.
type DeploySignature struct {
	ID               string   `json:"id"`
	TenantID         string   `json:"tenant_id"`
	Service          string   `json:"service"`
	SignatureName    string   `json:"signature_name"`
	Description      string   `json:"description"`
	Indicators       []string `json:"indicators"`
	ImpactedServices []string `json:"impacted_services"`
	TypicalSeverity  string   `json:"typical_severity"`
	OccurrenceCount  int      `json:"occurrence_count"`
	FirstSeenAt      string   `json:"first_seen_at"`
	LastSeenAt       string   `json:"last_seen_at"`
	CreatedAt        string   `json:"created_at"`
}

// RunbookPreference stores a team-specific workflow preference for a service.
type RunbookPreference struct {
	ID              string `json:"id"`
	TenantID        string `json:"tenant_id"`
	TeamName        string `json:"team_name"`
	Service         string `json:"service"`
	PreferenceKey   string `json:"preference_key"`
	PreferenceValue string `json:"preference_value"`
	Context         string `json:"context"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// FullMemoryContext is the complete AI memory answer for an incident.
// It combines: past incidents, proven remediations, deploy risk signals, and team preferences.
type FullMemoryContext struct {
	IncidentID             string               `json:"incident_id"`
	Service                string               `json:"service"`
	SimilarPastIncidents   []ResolutionRecord   `json:"similar_past_incidents"`
	RecurrenceCount        int                  `json:"recurrence_count"`
	SuggestedPlaybook      []string             `json:"suggested_playbook"`
	RemediationSuggestions []RemediationPattern `json:"remediation_suggestions"`
	DeployRisks            []DeploySignature    `json:"deploy_risks"`
	RunbookPreferences     []RunbookPreference  `json:"runbook_preferences"`
	LLMNarrative           string               `json:"llm_narrative"`
	QueriedAt              string               `json:"queried_at"`
}

// DomainResolutionRequest is the rich body for recording a structured resolution.
type DomainResolutionRequest struct {
	ResolutionNote      string   `json:"resolution_note"`
	RemediationSteps    []string `json:"remediation_steps"`
	ErrorSignature      string   `json:"error_signature"`
	RootCauseCategory   string   `json:"root_cause_category"`
	RemediationSummary  string   `json:"remediation_summary"`
	WasSuccessful       bool     `json:"was_successful"`
	DeploySignatureName string   `json:"deploy_signature_name"`
	DeployIndicators    []string `json:"deploy_indicators"`
	ImpactedServices    []string `json:"impacted_services"`
}
