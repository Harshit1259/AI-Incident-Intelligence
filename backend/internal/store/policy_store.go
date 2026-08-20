package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// PolicyStore provides persistence for execution policies.
type PolicyStore struct {
	db *sql.DB
}

// NewPolicyStore creates a new PolicyStore.
func NewPolicyStore(db *sql.DB) *PolicyStore {
	return &PolicyStore{db: db}
}

// Create inserts a new execution policy.
func (s *PolicyStore) Create(p models.ExecutionPolicy) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO execution_policies (
			id, tenant_id, name, enabled, priority,
			service, environment, severity,
			execution_mode, requires_approval, approval_groups, approval_timeout_minutes,
			blackout_start, blackout_end,
			business_hours_start, business_hours_end, business_hours_timezone, enforce_business_hours,
			max_per_hour, max_concurrent_actions, max_impacted_services,
			max_retries, retry_strategy, retry_backoff_seconds, cooldown_seconds, timeout_seconds,
			circuit_breaker_enabled, circuit_breaker_threshold, circuit_breaker_window_seconds, circuit_breaker_cooldown_minutes,
			simulation_mode, dry_run_mode,
			rollback_webhook_url, rollback_script_ref,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,
			$9,$10,$11,$12,
			$13,$14,
			$15,$16,$17,$18,
			$19,$20,$21,
			$22,$23,$24,$25,$26,
			$27,$28,$29,$30,
			$31,$32,
			$33,$34,
			$35,$36
		)`,
		p.ID, p.TenantID, p.Name, p.Enabled, p.Priority,
		p.Service, p.Environment, p.Severity,
		p.ExecutionMode, p.RequiresApproval, p.ApprovalGroups, p.ApprovalTimeoutMinutes,
		p.BlackoutStart, p.BlackoutEnd,
		p.BusinessHoursStart, p.BusinessHoursEnd, p.BusinessHoursTimezone, p.EnforceBusinessHours,
		p.MaxPerHour, p.MaxConcurrentActions, p.MaxImpactedServices,
		p.MaxRetries, p.RetryStrategy, p.RetryBackoffSeconds, p.CooldownSeconds, p.TimeoutSeconds,
		p.CircuitBreakerEnabled, p.CircuitBreakerThreshold, p.CircuitBreakerWindowSeconds, p.CircuitBreakerCooldownMinutes,
		p.SimulationMode, p.DryRunMode,
		p.RollbackWebhookURL, p.RollbackScriptRef,
		p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// Update updates all mutable fields of an existing policy.
func (s *PolicyStore) Update(p models.ExecutionPolicy) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE execution_policies SET
			name=$1, enabled=$2, priority=$3,
			service=$4, environment=$5, severity=$6,
			execution_mode=$7, requires_approval=$8, approval_groups=$9, approval_timeout_minutes=$10,
			blackout_start=$11, blackout_end=$12,
			business_hours_start=$13, business_hours_end=$14, business_hours_timezone=$15, enforce_business_hours=$16,
			max_per_hour=$17, max_concurrent_actions=$18, max_impacted_services=$19,
			max_retries=$20, retry_strategy=$21, retry_backoff_seconds=$22, cooldown_seconds=$23, timeout_seconds=$24,
			circuit_breaker_enabled=$25, circuit_breaker_threshold=$26, circuit_breaker_window_seconds=$27, circuit_breaker_cooldown_minutes=$28,
			simulation_mode=$29, dry_run_mode=$30,
			rollback_webhook_url=$31, rollback_script_ref=$32,
			updated_at=$33
		WHERE id=$34`,
		p.Name, p.Enabled, p.Priority,
		p.Service, p.Environment, p.Severity,
		p.ExecutionMode, p.RequiresApproval, p.ApprovalGroups, p.ApprovalTimeoutMinutes,
		p.BlackoutStart, p.BlackoutEnd,
		p.BusinessHoursStart, p.BusinessHoursEnd, p.BusinessHoursTimezone, p.EnforceBusinessHours,
		p.MaxPerHour, p.MaxConcurrentActions, p.MaxImpactedServices,
		p.MaxRetries, p.RetryStrategy, p.RetryBackoffSeconds, p.CooldownSeconds, p.TimeoutSeconds,
		p.CircuitBreakerEnabled, p.CircuitBreakerThreshold, p.CircuitBreakerWindowSeconds, p.CircuitBreakerCooldownMinutes,
		p.SimulationMode, p.DryRunMode,
		p.RollbackWebhookURL, p.RollbackScriptRef,
		p.UpdatedAt,
		p.ID,
	)
	return err
}

// Delete removes a policy by ID.
func (s *PolicyStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM execution_policies WHERE id = $1`, id)
	return err
}

// GetPolicies returns policies for a tenant, ordered by priority desc then created_at desc.
func (s *PolicyStore) GetPolicies(tenantID string, limit, offset int) ([]models.ExecutionPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+policyColumns+`
		FROM execution_policies
		WHERE tenant_id = $1
		ORDER BY priority DESC, created_at DESC
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []models.ExecutionPolicy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// GetPolicyByID returns a single policy by ID.
func (s *PolicyStore) GetPolicyByID(id string) (*models.ExecutionPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM execution_policies WHERE id = $1`, id)
	var p models.ExecutionPolicy
	err := scanPolicyRow(row, &p)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindMatchingPolicy finds the highest-priority enabled policy for the given
// (tenant, service, environment, severity) tuple.
// Specificity scoring:
//   service match    = +4
//   environment match = +2
//   severity match   = +1
//
// Within the same score the policy with higher Priority wins.
func (s *PolicyStore) FindMatchingPolicy(tenantID, service, environment, severity string) (*models.ExecutionPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+policyColumns+`
		FROM execution_policies
		WHERE tenant_id = $1 AND enabled = true
		ORDER BY priority DESC, created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best *models.ExecutionPolicy
	bestScore := -1

	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}

		matchesService := p.Service == "" || p.Service == service
		matchesEnv := p.Environment == "" || p.Environment == environment
		matchesSev := p.Severity == "" || p.Severity == severity

		if !matchesService || !matchesEnv || !matchesSev {
			continue
		}

		score := 0
		if p.Service != "" {
			score += 4
		}
		if p.Environment != "" {
			score += 2
		}
		if p.Severity != "" {
			score += 1
		}

		// Pick highest score; within equal score, higher Priority wins (rows ordered by priority desc).
		if score > bestScore {
			bestScore = score
			cp := p
			best = &cp
		}
	}

	return best, rows.Err()
}

// CountRecentExecutions counts total action executions for a tenant since a given time.
func (s *PolicyStore) CountRecentExecutions(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM action_executions
		WHERE tenant_id = $1 AND created_at >= $2`, tenantID, since).Scan(&count)
	return count, err
}

// CountRunningExecutions counts executions currently in running or pending_approval state.
func (s *PolicyStore) CountRunningExecutions(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM action_executions
		WHERE tenant_id = $1 AND status IN ('running', 'pending_approval', 'approved')`,
		tenantID).Scan(&count)
	return count, err
}

// policyColumns is the shared SELECT column list — keeps Create/Scan in sync.
const policyColumns = `
	id, tenant_id, name, enabled, priority,
	service, environment, severity,
	execution_mode, requires_approval, approval_groups, approval_timeout_minutes,
	blackout_start, blackout_end,
	business_hours_start, business_hours_end, business_hours_timezone, enforce_business_hours,
	max_per_hour, max_concurrent_actions, max_impacted_services,
	max_retries, retry_strategy, retry_backoff_seconds, cooldown_seconds, timeout_seconds,
	circuit_breaker_enabled, circuit_breaker_threshold, circuit_breaker_window_seconds, circuit_breaker_cooldown_minutes,
	simulation_mode, dry_run_mode,
	rollback_webhook_url, rollback_script_ref,
	created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicyRow(row rowScanner, p *models.ExecutionPolicy) error {
	return row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Enabled, &p.Priority,
		&p.Service, &p.Environment, &p.Severity,
		&p.ExecutionMode, &p.RequiresApproval, &p.ApprovalGroups, &p.ApprovalTimeoutMinutes,
		&p.BlackoutStart, &p.BlackoutEnd,
		&p.BusinessHoursStart, &p.BusinessHoursEnd, &p.BusinessHoursTimezone, &p.EnforceBusinessHours,
		&p.MaxPerHour, &p.MaxConcurrentActions, &p.MaxImpactedServices,
		&p.MaxRetries, &p.RetryStrategy, &p.RetryBackoffSeconds, &p.CooldownSeconds, &p.TimeoutSeconds,
		&p.CircuitBreakerEnabled, &p.CircuitBreakerThreshold, &p.CircuitBreakerWindowSeconds, &p.CircuitBreakerCooldownMinutes,
		&p.SimulationMode, &p.DryRunMode,
		&p.RollbackWebhookURL, &p.RollbackScriptRef,
		&p.CreatedAt, &p.UpdatedAt,
	)
}

func scanPolicy(rows *sql.Rows) (models.ExecutionPolicy, error) {
	var p models.ExecutionPolicy
	err := scanPolicyRow(rows, &p)
	return p, err
}
