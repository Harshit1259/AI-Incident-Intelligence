package models

import "time"

// ActionExecution tracks the execution of an action within an incident response chain.
//
// Status state machine:
//   pending_approval → approved → running → completed|failed
//   Any state → rolled_back (when RequiresApproval and rollback is invoked)
type ActionExecution struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	IncidentID         string     `json:"incident_id"`
	ActionID           string     `json:"action_id"`
	ActionLabel        string     `json:"action_label"`
	ActionDesc         string     `json:"action_desc"`
	ChainPosition      int        `json:"chain_position"`
	Status             string     `json:"status"`
	ExecutionMode      string     `json:"execution_mode"`
	ResultOutput       string     `json:"result_output"`
	ErrorMessage       string     `json:"error_message"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	Verified           bool       `json:"verified"`
	VerificationResult string     `json:"verification_result"`
	CreatedAt          time.Time  `json:"created_at"`

	// Approval workflow — populated when RequiresApproval=true
	RequiresApproval bool       `json:"requires_approval"`
	ApprovalStatus   string     `json:"approval_status,omitempty"` // pending | approved | rejected
	ApprovedBy       string     `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	RejectedBy       string     `json:"rejected_by,omitempty"`
	RejectionReason  string     `json:"rejection_reason,omitempty"`

	// Rollback tracking
	RollbackRef    string `json:"rollback_ref,omitempty"`    // e.g. previous version / snapshot ref
	RollbackReason string `json:"rollback_reason,omitempty"` // why rollback was triggered
	RolledBackBy   string `json:"rolled_back_by,omitempty"`
}
