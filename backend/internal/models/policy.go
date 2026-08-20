package models

import (
	"strings"
	"time"
)

// ExecutionPolicy defines rules governing how and when actions can be executed.
// Policies are matched by (tenant, service, environment, severity) with a
// priority + specificity scoring system — the most specific enabled policy wins.
type ExecutionPolicy struct {
	// ── Identity ─────────────────────────────────────────────────────────────
	ID       string    `json:"id"`
	TenantID string    `json:"tenant_id"`
	Name     string    `json:"name"`
	Enabled  bool      `json:"enabled"`
	Priority int       `json:"priority"` // 0-100; higher wins on equal specificity score

	// ── Matching dimensions ───────────────────────────────────────────────────
	// Empty string = wildcard (matches any value).
	Service     string `json:"service"`
	Environment string `json:"environment"` // prod | staging | dev | ""
	Severity    string `json:"severity"`    // critical | high | medium | low | ""

	// ── Execution mode ────────────────────────────────────────────────────────
	// manual     — operator runs the action manually
	// auto        — system executes immediately on match
	// approval    — queued until an approver accepts/rejects
	// simulation  — action is recorded as simulated but not executed
	// dry_run     — policy is evaluated and logged; no action record is created
	ExecutionMode string `json:"execution_mode"`

	// ── Approval workflow extensions ──────────────────────────────────────────
	RequiresApproval       bool   `json:"requires_approval"`
	ApprovalGroups         string `json:"approval_groups"`          // comma-separated role/group names
	ApprovalTimeoutMinutes int    `json:"approval_timeout_minutes"` // 0 = never auto-reject

	// ── Time windows ─────────────────────────────────────────────────────────
	// Blackout: deny all execution during this daily window.
	BlackoutStart string `json:"blackout_start"` // HH:MM (UTC unless timezone set)
	BlackoutEnd   string `json:"blackout_end"`   // HH:MM

	// Business hours: if EnforceBusinessHours=true, deny execution outside this window.
	BusinessHoursStart    string `json:"business_hours_start"`    // HH:MM
	BusinessHoursEnd      string `json:"business_hours_end"`      // HH:MM
	BusinessHoursTimezone string `json:"business_hours_timezone"` // IANA e.g. "America/New_York"; "" = UTC
	EnforceBusinessHours  bool   `json:"enforce_business_hours"`

	// ── Blast-radius controls ─────────────────────────────────────────────────
	MaxPerHour           int `json:"max_per_hour"`            // 0 = unlimited
	MaxConcurrentActions int `json:"max_concurrent_actions"`  // 0 = unlimited
	MaxImpactedServices  int `json:"max_impacted_services"`   // 0 = unlimited; deny if incident.impact_count > this

	// ── Retry / circuit-breaker ───────────────────────────────────────────────
	MaxRetries      int    `json:"max_retries"`
	RetryStrategy   string `json:"retry_strategy"`        // fixed | linear | exponential
	RetryBackoffSeconds int `json:"retry_backoff_seconds"` // base backoff seconds
	CooldownSeconds int    `json:"cooldown_seconds"`       // minimum gap between executions
	TimeoutSeconds  int    `json:"timeout_seconds"`        // per-action execution timeout

	CircuitBreakerEnabled         bool `json:"circuit_breaker_enabled"`
	CircuitBreakerThreshold       int  `json:"circuit_breaker_threshold"`        // consecutive failures before open
	CircuitBreakerWindowSeconds   int  `json:"circuit_breaker_window_seconds"`   // counting window
	CircuitBreakerCooldownMinutes int  `json:"circuit_breaker_cooldown_minutes"` // open duration

	// ── Simulation / dry-run modes ────────────────────────────────────────────
	// SimulationMode: action record is created with status="simulation"; not actually executed.
	// DryRunMode: policy is evaluated and a trail entry is written; no action record created.
	SimulationMode bool `json:"simulation_mode"`
	DryRunMode     bool `json:"dry_run_mode"`

	// ── Rollback hooks ────────────────────────────────────────────────────────
	RollbackWebhookURL string `json:"rollback_webhook_url"` // POST on rollback event
	RollbackScriptRef  string `json:"rollback_script_ref"`  // runbook/script identifier

	// ── Audit ─────────────────────────────────────────────────────────────────
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ApprovalGroupList returns ApprovalGroups as a parsed slice.
func (p *ExecutionPolicy) ApprovalGroupList() []string {
	if p.ApprovalGroups == "" {
		return nil
	}
	parts := strings.Split(p.ApprovalGroups, ",")
	groups := make([]string, 0, len(parts))
	for _, g := range parts {
		if t := strings.TrimSpace(g); t != "" {
			groups = append(groups, t)
		}
	}
	return groups
}

// PolicyEvalContext carries all inputs needed for a single policy evaluation.
type PolicyEvalContext struct {
	TenantID         string `json:"tenant_id"`
	IncidentID       string `json:"incident_id"`
	ActionID         string `json:"action_id"`
	Service          string `json:"service"`
	Environment      string `json:"environment"` // prod | staging | dev | ""
	Severity         string `json:"severity"`
	ImpactedServices int    `json:"impacted_services"` // incident.impact_count
	Actor            string `json:"actor"`
	IsDryRun         bool   `json:"is_dry_run"`
}

// PolicyDecision is the result of evaluating policies for a given context.
type PolicyDecision struct {
	Allowed          bool     `json:"allowed"`
	ExecutionMode    string   `json:"execution_mode"`
	Reason           string   `json:"reason"`
	PolicyID         string   `json:"policy_id,omitempty"`
	IsSimulation     bool     `json:"is_simulation"`
	IsDryRun         bool     `json:"is_dry_run"`
	RequiresApproval bool     `json:"requires_approval"`
	ApprovalGroups   []string `json:"approval_groups,omitempty"`
	ApprovalTimeout  int      `json:"approval_timeout_minutes,omitempty"`
	TrailID          string   `json:"trail_id,omitempty"`
	CircuitOpen      bool     `json:"circuit_open"`
	// DenialCode is machine-readable reason for denial.
	// Values: BLACKOUT | BUSINESS_HOURS | RATE_LIMIT | BLAST_RADIUS | CIRCUIT_OPEN | POLICY_DENY
	DenialCode string `json:"denial_code,omitempty"`
}

// PolicyExecutionTrail is an immutable record of every policy evaluation.
// Once written it must never be modified or deleted — it forms the
// enterprise-grade execution audit trail.
type PolicyExecutionTrail struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	PolicyID      string    `json:"policy_id"`
	PolicyName    string    `json:"policy_name"`
	IncidentID    string    `json:"incident_id"`
	ActionID      string    `json:"action_id"`
	Service       string    `json:"service"`
	Environment   string    `json:"environment"`
	Severity      string    `json:"severity"`
	Decision      string    `json:"decision"`       // allowed | denied | simulation | dry_run
	DenialCode    string    `json:"denial_code"`    // BLACKOUT | RATE_LIMIT | etc.
	ExecutionMode string    `json:"execution_mode"`
	Reason        string    `json:"reason"`
	IsSimulation  bool      `json:"is_simulation"`
	IsDryRun      bool      `json:"is_dry_run"`
	Actor         string    `json:"actor"`
	ContextJSON   string    `json:"context_json"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

// CircuitBreakerState tracks the open/closed state of a policy's circuit breaker.
type CircuitBreakerState struct {
	PolicyID            string     `json:"policy_id"`
	TenantID            string     `json:"tenant_id"`
	IsOpen              bool       `json:"is_open"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	OpenedAt            *time.Time `json:"opened_at,omitempty"`
	WillResetAt         *time.Time `json:"will_reset_at,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
