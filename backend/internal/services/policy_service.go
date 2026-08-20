package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// PolicyService is the enterprise-grade automation policy evaluation engine.
// It enforces allow/deny by service/environment/severity, approval flows,
// business-hour and blackout windows, blast-radius limits, retry/circuit-breaker
// controls, simulation/dry-run modes, rollback hooks, and writes an immutable
// execution trail for every evaluation.
type PolicyService struct {
	store    *store.PolicyStore
	trail    *store.PolicyExecutionTrailStore
	breaker  *store.CircuitBreakerStore
}

// NewPolicyService creates a new PolicyService.
func NewPolicyService(
	ps *store.PolicyStore,
	trail *store.PolicyExecutionTrailStore,
	breaker *store.CircuitBreakerStore,
) *PolicyService {
	return &PolicyService{store: ps, trail: trail, breaker: breaker}
}

// Evaluate runs the full policy engine for the given context.
// Evaluation order (first failure wins):
//  1. Circuit breaker open?
//  2. Blackout window?
//  3. Business hours enforcement?
//  4. Rate limit (max_per_hour)?
//  5. Blast-radius (concurrent actions)?
//  6. Blast-radius (impacted services)?
//  7. Simulation / dry-run mode override?
//  8. Approval requirement?
//
// A trail entry is always written unless IsDryRun is true and no policy matched
// (i.e., the evaluation had nothing to record).
func (s *PolicyService) Evaluate(ctx models.PolicyEvalContext) models.PolicyDecision {
	policy, err := s.store.FindMatchingPolicy(ctx.TenantID, ctx.Service, ctx.Environment, ctx.Severity)
	if err != nil {
		log.Printf("policy: FindMatchingPolicy error: %v", err)
		return s.defaultDecision("policy lookup failed, defaulting to manual", "")
	}

	if policy == nil {
		d := s.defaultDecision("no matching policy, defaulting to manual execution", "")
		// Still write a trail entry so operators can see unevaluated actions.
		s.writeTrail(ctx, d, nil)
		return d
	}

	// ── 1. Circuit breaker ────────────────────────────────────────────────────
	if policy.CircuitBreakerEnabled {
		_ = s.breaker.AutoReset(policy.ID) // auto-close if cooldown elapsed
		state, err := s.breaker.Get(policy.ID)
		if err != nil {
			log.Printf("policy: circuit breaker state error: %v", err)
		}
		if state != nil && state.IsOpen {
			d := models.PolicyDecision{
				Allowed:       false,
				ExecutionMode: policy.ExecutionMode,
				PolicyID:      policy.ID,
				Reason:        fmt.Sprintf("circuit breaker open (%d consecutive failures)", state.ConsecutiveFailures),
				DenialCode:    "CIRCUIT_OPEN",
				CircuitOpen:   true,
			}
			s.writeTrail(ctx, d, policy)
			return d
		}
	}

	// ── 2. Blackout window ────────────────────────────────────────────────────
	if policy.BlackoutStart != "" && policy.BlackoutEnd != "" {
		if isInTimeWindow(policy.BlackoutStart, policy.BlackoutEnd, "UTC") {
			d := models.PolicyDecision{
				Allowed:       false,
				ExecutionMode: policy.ExecutionMode,
				PolicyID:      policy.ID,
				Reason:        fmt.Sprintf("blackout window active (%s – %s UTC)", policy.BlackoutStart, policy.BlackoutEnd),
				DenialCode:    "BLACKOUT",
			}
			s.writeTrail(ctx, d, policy)
			return d
		}
	}

	// ── 3. Business hours enforcement ─────────────────────────────────────────
	if policy.EnforceBusinessHours && policy.BusinessHoursStart != "" && policy.BusinessHoursEnd != "" {
		tz := policy.BusinessHoursTimezone
		if tz == "" {
			tz = "UTC"
		}
		if !isInTimeWindow(policy.BusinessHoursStart, policy.BusinessHoursEnd, tz) {
			d := models.PolicyDecision{
				Allowed:       false,
				ExecutionMode: policy.ExecutionMode,
				PolicyID:      policy.ID,
				Reason:        fmt.Sprintf("outside business hours (%s – %s %s)", policy.BusinessHoursStart, policy.BusinessHoursEnd, tz),
				DenialCode:    "BUSINESS_HOURS",
			}
			s.writeTrail(ctx, d, policy)
			return d
		}
	}

	// ── 4. Rate limit ─────────────────────────────────────────────────────────
	if policy.MaxPerHour > 0 {
		count, err := s.store.CountRecentExecutions(ctx.TenantID, time.Now().Add(-time.Hour))
		if err != nil {
			log.Printf("policy: CountRecentExecutions error: %v", err)
		} else if count >= policy.MaxPerHour {
			d := models.PolicyDecision{
				Allowed:       false,
				ExecutionMode: policy.ExecutionMode,
				PolicyID:      policy.ID,
				Reason:        fmt.Sprintf("rate limit: %d/%d executions in the last hour", count, policy.MaxPerHour),
				DenialCode:    "RATE_LIMIT",
			}
			s.writeTrail(ctx, d, policy)
			return d
		}
	}

	// ── 5. Concurrent actions blast-radius ────────────────────────────────────
	if policy.MaxConcurrentActions > 0 {
		running, err := s.store.CountRunningExecutions(ctx.TenantID)
		if err != nil {
			log.Printf("policy: CountRunningExecutions error: %v", err)
		} else if running >= policy.MaxConcurrentActions {
			d := models.PolicyDecision{
				Allowed:       false,
				ExecutionMode: policy.ExecutionMode,
				PolicyID:      policy.ID,
				Reason:        fmt.Sprintf("blast-radius: %d/%d concurrent actions already running", running, policy.MaxConcurrentActions),
				DenialCode:    "BLAST_RADIUS",
			}
			s.writeTrail(ctx, d, policy)
			return d
		}
	}

	// ── 6. Impacted-services blast-radius ─────────────────────────────────────
	if policy.MaxImpactedServices > 0 && ctx.ImpactedServices > policy.MaxImpactedServices {
		d := models.PolicyDecision{
			Allowed:       false,
			ExecutionMode: policy.ExecutionMode,
			PolicyID:      policy.ID,
			Reason:        fmt.Sprintf("blast-radius: incident affects %d services (limit %d)", ctx.ImpactedServices, policy.MaxImpactedServices),
			DenialCode:    "BLAST_RADIUS",
		}
		s.writeTrail(ctx, d, policy)
		return d
	}

	// ── 7. Simulation / dry-run mode ──────────────────────────────────────────
	if policy.DryRunMode || ctx.IsDryRun {
		d := models.PolicyDecision{
			Allowed:       true,
			ExecutionMode: "dry_run",
			PolicyID:      policy.ID,
			IsDryRun:      true,
			Reason:        fmt.Sprintf("policy %q — dry-run mode: evaluation logged, no action created", policy.Name),
		}
		s.writeTrail(ctx, d, policy)
		return d
	}

	if policy.SimulationMode {
		d := models.PolicyDecision{
			Allowed:       true,
			ExecutionMode: "simulation",
			PolicyID:      policy.ID,
			IsSimulation:  true,
			Reason:        fmt.Sprintf("policy %q — simulation mode: action recorded but not executed", policy.Name),
		}
		s.writeTrail(ctx, d, policy)
		return d
	}

	// ── 8. Approval requirement ────────────────────────────────────────────────
	mode := policy.ExecutionMode
	requiresApproval := policy.RequiresApproval
	if requiresApproval && mode == "auto" {
		mode = "approval"
	}

	d := models.PolicyDecision{
		Allowed:          true,
		ExecutionMode:    mode,
		PolicyID:         policy.ID,
		Reason:           fmt.Sprintf("policy %q allows execution in %s mode", policy.Name, mode),
		RequiresApproval: requiresApproval,
		ApprovalGroups:   policy.ApprovalGroupList(),
		ApprovalTimeout:  policy.ApprovalTimeoutMinutes,
	}
	s.writeTrail(ctx, d, policy)
	return d
}

// Simulate runs the evaluation engine in read-only mode: no trail is written,
// no circuit-breaker counters are touched. Returns what the decision would be.
func (s *PolicyService) Simulate(ctx models.PolicyEvalContext) models.PolicyDecision {
	// Force dry-run so the normal Evaluate path exits cleanly, but capture
	// the decision before any side-effects fire.
	simCtx := ctx
	simCtx.IsDryRun = false // we'll intercept before trail write

	policy, err := s.store.FindMatchingPolicy(simCtx.TenantID, simCtx.Service, simCtx.Environment, simCtx.Severity)
	if err != nil {
		return s.defaultDecision("policy lookup failed", "")
	}
	if policy == nil {
		return s.defaultDecision("no matching policy", "")
	}

	// Run the full check sequence without side-effects.
	d := s.evaluatePure(simCtx, policy)
	d.Reason = "[SIMULATION] " + d.Reason
	return d
}

// evaluatePure performs all checks against the supplied policy without writing
// any trail entries or mutating circuit-breaker state.
func (s *PolicyService) evaluatePure(ctx models.PolicyEvalContext, policy *models.ExecutionPolicy) models.PolicyDecision {
	// Circuit breaker
	if policy.CircuitBreakerEnabled {
		state, _ := s.breaker.Get(policy.ID)
		if state != nil && state.IsOpen && (state.WillResetAt == nil || time.Now().Before(*state.WillResetAt)) {
			return models.PolicyDecision{
				Allowed: false, PolicyID: policy.ID,
				Reason: "circuit breaker open", DenialCode: "CIRCUIT_OPEN", CircuitOpen: true,
			}
		}
	}
	// Blackout
	if policy.BlackoutStart != "" && policy.BlackoutEnd != "" && isInTimeWindow(policy.BlackoutStart, policy.BlackoutEnd, "UTC") {
		return models.PolicyDecision{Allowed: false, PolicyID: policy.ID, Reason: "blackout window", DenialCode: "BLACKOUT"}
	}
	// Business hours
	if policy.EnforceBusinessHours && policy.BusinessHoursStart != "" && policy.BusinessHoursEnd != "" {
		tz := policy.BusinessHoursTimezone
		if tz == "" {
			tz = "UTC"
		}
		if !isInTimeWindow(policy.BusinessHoursStart, policy.BusinessHoursEnd, tz) {
			return models.PolicyDecision{Allowed: false, PolicyID: policy.ID, Reason: "outside business hours", DenialCode: "BUSINESS_HOURS"}
		}
	}
	// Rate limit
	if policy.MaxPerHour > 0 {
		count, _ := s.store.CountRecentExecutions(ctx.TenantID, time.Now().Add(-time.Hour))
		if count >= policy.MaxPerHour {
			return models.PolicyDecision{Allowed: false, PolicyID: policy.ID, Reason: fmt.Sprintf("rate limit %d/%d", count, policy.MaxPerHour), DenialCode: "RATE_LIMIT"}
		}
	}
	// Concurrent blast radius
	if policy.MaxConcurrentActions > 0 {
		running, _ := s.store.CountRunningExecutions(ctx.TenantID)
		if running >= policy.MaxConcurrentActions {
			return models.PolicyDecision{Allowed: false, PolicyID: policy.ID, Reason: "concurrent action limit", DenialCode: "BLAST_RADIUS"}
		}
	}
	// Impacted services blast radius
	if policy.MaxImpactedServices > 0 && ctx.ImpactedServices > policy.MaxImpactedServices {
		return models.PolicyDecision{Allowed: false, PolicyID: policy.ID, Reason: "impacted services limit", DenialCode: "BLAST_RADIUS"}
	}
	// Simulation/dry-run
	if policy.DryRunMode || ctx.IsDryRun {
		return models.PolicyDecision{Allowed: true, ExecutionMode: "dry_run", PolicyID: policy.ID, IsDryRun: true, Reason: "dry-run mode"}
	}
	if policy.SimulationMode {
		return models.PolicyDecision{Allowed: true, ExecutionMode: "simulation", PolicyID: policy.ID, IsSimulation: true, Reason: "simulation mode"}
	}
	// Approval
	mode := policy.ExecutionMode
	if policy.RequiresApproval && mode == "auto" {
		mode = "approval"
	}
	return models.PolicyDecision{
		Allowed: true, ExecutionMode: mode, PolicyID: policy.ID,
		RequiresApproval: policy.RequiresApproval,
		ApprovalGroups:   policy.ApprovalGroupList(),
		ApprovalTimeout:  policy.ApprovalTimeoutMinutes,
		Reason:           fmt.Sprintf("policy %q allows %s", policy.Name, mode),
	}
}

// writeTrail appends one immutable trail entry for the evaluation. Errors are
// logged and swallowed — a trail write failure must never block execution.
func (s *PolicyService) writeTrail(ctx models.PolicyEvalContext, d models.PolicyDecision, policy *models.ExecutionPolicy) {
	if s.trail == nil {
		return
	}
	ctxBytes, _ := json.Marshal(ctx)

	decision := "allowed"
	if !d.Allowed {
		decision = "denied"
	}
	if d.IsSimulation {
		decision = "simulation"
	}
	if d.IsDryRun {
		decision = "dry_run"
	}

	policyName := ""
	if policy != nil {
		policyName = policy.Name
	}

	t := models.PolicyExecutionTrail{
		ID:            fmt.Sprintf("trail-%d", time.Now().UnixNano()),
		TenantID:      ctx.TenantID,
		PolicyID:      d.PolicyID,
		PolicyName:    policyName,
		IncidentID:    ctx.IncidentID,
		ActionID:      ctx.ActionID,
		Service:       ctx.Service,
		Environment:   ctx.Environment,
		Severity:      ctx.Severity,
		Decision:      decision,
		DenialCode:    d.DenialCode,
		ExecutionMode: d.ExecutionMode,
		Reason:        d.Reason,
		IsSimulation:  d.IsSimulation,
		IsDryRun:      d.IsDryRun,
		Actor:         ctx.Actor,
		ContextJSON:   string(ctxBytes),
		EvaluatedAt:   time.Now(),
	}
	if err := s.trail.Append(t); err != nil {
		log.Printf("policy: trail write error: %v", err)
	}
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// GetPolicies returns policies for a tenant with pagination.
func (s *PolicyService) GetPolicies(tenantID string, limit, offset int) ([]models.ExecutionPolicy, error) {
	return s.store.GetPolicies(tenantID, limit, offset)
}

// GetPolicyByID returns a single policy.
func (s *PolicyService) GetPolicyByID(id string) (*models.ExecutionPolicy, error) {
	return s.store.GetPolicyByID(id)
}

// CreatePolicy persists a new policy with sensible defaults.
func (s *PolicyService) CreatePolicy(p models.ExecutionPolicy) error {
	if p.ID == "" {
		p.ID = fmt.Sprintf("pol-%d", time.Now().UnixNano())
	}
	if p.ExecutionMode == "" {
		p.ExecutionMode = "manual"
	}
	if p.MaxRetries == 0 {
		p.MaxRetries = 2
	}
	if p.CooldownSeconds == 0 {
		p.CooldownSeconds = 300
	}
	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = 90
	}
	if p.MaxPerHour == 0 {
		p.MaxPerHour = 10
	}
	if p.RetryStrategy == "" {
		p.RetryStrategy = "exponential"
	}
	if p.RetryBackoffSeconds == 0 {
		p.RetryBackoffSeconds = 30
	}
	if p.CircuitBreakerThreshold == 0 {
		p.CircuitBreakerThreshold = 5
	}
	if p.CircuitBreakerWindowSeconds == 0 {
		p.CircuitBreakerWindowSeconds = 3600
	}
	if p.CircuitBreakerCooldownMinutes == 0 {
		p.CircuitBreakerCooldownMinutes = 30
	}
	if p.ApprovalTimeoutMinutes == 0 {
		p.ApprovalTimeoutMinutes = 60
	}
	if p.BusinessHoursTimezone == "" {
		p.BusinessHoursTimezone = "UTC"
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	return s.store.Create(p)
}

// UpdatePolicy updates a policy and stamps updated_at.
func (s *PolicyService) UpdatePolicy(p models.ExecutionPolicy) error {
	p.UpdatedAt = time.Now()
	return s.store.Update(p)
}

// DeletePolicy removes a policy by ID.
func (s *PolicyService) DeletePolicy(id string) error {
	return s.store.Delete(id)
}

// ── Circuit breaker management ────────────────────────────────────────────────

// GetCircuitBreakerStates returns all circuit-breaker states for a tenant.
func (s *PolicyService) GetCircuitBreakerStates(tenantID string) ([]models.CircuitBreakerState, error) {
	return s.breaker.GetByTenant(tenantID)
}

// ResetCircuitBreaker manually closes a tripped circuit breaker.
func (s *PolicyService) ResetCircuitBreaker(policyID, tenantID string) error {
	return s.breaker.Reset(policyID, tenantID)
}

// RecordExecutionOutcome updates the circuit breaker based on execution result.
func (s *PolicyService) RecordExecutionOutcome(policyID, tenantID string, succeeded bool, policy *models.ExecutionPolicy) {
	if policy == nil || !policy.CircuitBreakerEnabled {
		return
	}
	if succeeded {
		if err := s.breaker.RecordSuccess(policyID, tenantID); err != nil {
			log.Printf("policy: circuit breaker success record error: %v", err)
		}
	} else {
		if err := s.breaker.RecordFailure(policyID, tenantID, policy.CircuitBreakerThreshold, policy.CircuitBreakerCooldownMinutes); err != nil {
			log.Printf("policy: circuit breaker failure record error: %v", err)
		}
	}
}

// ── Execution trail ───────────────────────────────────────────────────────────

// GetTrailByPolicy returns the most recent N trail entries for a policy.
func (s *PolicyService) GetTrailByPolicy(policyID string, limit int) ([]models.PolicyExecutionTrail, error) {
	return s.trail.GetByPolicy(policyID, limit)
}

// GetTrailByTenant returns the most recent N trail entries for a tenant.
func (s *PolicyService) GetTrailByTenant(tenantID string, limit int) ([]models.PolicyExecutionTrail, error) {
	return s.trail.GetByTenant(tenantID, limit)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *PolicyService) defaultDecision(reason, policyID string) models.PolicyDecision {
	return models.PolicyDecision{
		Allowed:       true,
		ExecutionMode: "manual",
		Reason:        reason,
		PolicyID:      policyID,
	}
}

// isInTimeWindow checks whether the current local time in the given IANA timezone
// falls within [start, end] (HH:MM format). Supports overnight windows (e.g. 22:00–06:00).
func isInTimeWindow(start, end, timezone string) bool {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	currentMinutes := now.Hour()*60 + now.Minute()

	startMin, err1 := parseHHMM(start)
	endMin, err2 := parseHHMM(end)
	if err1 != nil || err2 != nil {
		return false
	}

	if startMin <= endMin {
		return currentMinutes >= startMin && currentMinutes <= endMin
	}
	// Overnight window
	return currentMinutes >= startMin || currentMinutes <= endMin
}

func parseHHMM(s string) (int, error) {
	var h, m int
	_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil {
		return 0, err
	}
	return h*60 + m, nil
}
