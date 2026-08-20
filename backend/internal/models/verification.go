package models

import "time"

// VerificationSnapshot captures a point-in-time health reading of the service.
type VerificationSnapshot struct {
	CapturedAt         string  `json:"captured_at"`
	ErrorRatePct       float64 `json:"error_rate_pct"`
	CPUPercent         float64 `json:"cpu_percent"`
	MemoryPercent      float64 `json:"memory_percent"`
	ActiveAlerts       int     `json:"active_alerts"`
	ErrorCount         int     `json:"error_count"`
	LatencyMsP99       float64 `json:"latency_ms_p99"`
	IncidentConfidence int     `json:"incident_confidence"`
}

// ProofItem is a single evidence artefact demonstrating action effectiveness.
type ProofItem struct {
	// Type: metric_drop | error_clear | alert_resolved | latency_drop | manual
	Type       string  `json:"type"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	Unit       string  `json:"unit"`
	// Status: improved | degraded | neutral
	Status     string  `json:"status"`
	CapturedAt string  `json:"captured_at"`
}

// VerificationRecord represents a closed-loop verification run for an incident or action.
type VerificationRecord struct {
	ID                string               `json:"id"`
	TenantID          string               `json:"tenant_id"`
	IncidentID        string               `json:"incident_id"`
	ExecutionID       string               `json:"execution_id"`
	Strategy          string               `json:"strategy"`
	Status            string               `json:"status"`
	Checks            []VerificationCheck  `json:"checks"`
	Result            string               `json:"result"`
	BeforeSnapshot    *VerificationSnapshot `json:"before_snapshot,omitempty"`
	AfterSnapshot     *VerificationSnapshot `json:"after_snapshot,omitempty"`
	ConfidenceBefore  int                  `json:"confidence_before"`
	ConfidenceAfter   int                  `json:"confidence_after"`
	ProofItems        []ProofItem          `json:"proof_items"`
	RollbackTriggered bool                 `json:"rollback_triggered"`
	AutoCloseEligible bool                 `json:"auto_close_eligible"`
	VerifiedAt        *time.Time           `json:"verified_at,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
}

// VerificationCheck represents a single check within a verification record.
type VerificationCheck struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}
