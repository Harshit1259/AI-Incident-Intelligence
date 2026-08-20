package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// CircuitBreakerStore manages per-policy circuit-breaker state.
type CircuitBreakerStore struct {
	db *sql.DB
}

// NewCircuitBreakerStore creates a new CircuitBreakerStore.
func NewCircuitBreakerStore(db *sql.DB) *CircuitBreakerStore {
	return &CircuitBreakerStore{db: db}
}

// Get returns the circuit breaker state for a policy, or nil if no record exists.
func (s *CircuitBreakerStore) Get(policyID string) (*models.CircuitBreakerState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var st models.CircuitBreakerState
	var lastFailure, openedAt, willReset sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT policy_id, tenant_id, is_open, consecutive_failures,
			last_failure_at, opened_at, will_reset_at, updated_at
		FROM policy_circuit_breaker_states
		WHERE policy_id = $1`, policyID).Scan(
		&st.PolicyID, &st.TenantID, &st.IsOpen, &st.ConsecutiveFailures,
		&lastFailure, &openedAt, &willReset, &st.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastFailure.Valid {
		st.LastFailureAt = &lastFailure.Time
	}
	if openedAt.Valid {
		st.OpenedAt = &openedAt.Time
	}
	if willReset.Valid {
		st.WillResetAt = &willReset.Time
	}
	return &st, nil
}

// GetByTenant returns all circuit breaker states for a tenant.
func (s *CircuitBreakerStore) GetByTenant(tenantID string) ([]models.CircuitBreakerState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT policy_id, tenant_id, is_open, consecutive_failures,
			last_failure_at, opened_at, will_reset_at, updated_at
		FROM policy_circuit_breaker_states
		WHERE tenant_id = $1
		ORDER BY is_open DESC, consecutive_failures DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []models.CircuitBreakerState
	for rows.Next() {
		var st models.CircuitBreakerState
		var lastFailure, openedAt, willReset sql.NullTime
		if err := rows.Scan(
			&st.PolicyID, &st.TenantID, &st.IsOpen, &st.ConsecutiveFailures,
			&lastFailure, &openedAt, &willReset, &st.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastFailure.Valid {
			st.LastFailureAt = &lastFailure.Time
		}
		if openedAt.Valid {
			st.OpenedAt = &openedAt.Time
		}
		if willReset.Valid {
			st.WillResetAt = &willReset.Time
		}
		states = append(states, st)
	}
	return states, rows.Err()
}

// RecordFailure increments the consecutive failure count. If the threshold is
// reached the breaker opens and will_reset_at is set to now + cooldown.
func (s *CircuitBreakerStore) RecordFailure(policyID, tenantID string, threshold, cooldownMinutes int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_circuit_breaker_states
			(policy_id, tenant_id, is_open, consecutive_failures, last_failure_at, updated_at)
		VALUES ($1, $2, false, 1, $3, $3)
		ON CONFLICT (policy_id) DO UPDATE SET
			consecutive_failures = policy_circuit_breaker_states.consecutive_failures + 1,
			last_failure_at      = $3,
			updated_at           = $3,
			is_open = CASE
				WHEN policy_circuit_breaker_states.consecutive_failures + 1 >= $4 THEN true
				ELSE policy_circuit_breaker_states.is_open
			END,
			opened_at = CASE
				WHEN policy_circuit_breaker_states.consecutive_failures + 1 >= $4
					AND policy_circuit_breaker_states.is_open = false THEN $3
				ELSE policy_circuit_breaker_states.opened_at
			END,
			will_reset_at = CASE
				WHEN policy_circuit_breaker_states.consecutive_failures + 1 >= $4
					AND policy_circuit_breaker_states.is_open = false
					THEN $3 + ($5 * INTERVAL '1 minute')
				ELSE policy_circuit_breaker_states.will_reset_at
			END`,
		policyID, tenantID, now, threshold, cooldownMinutes,
	)
	return err
}

// RecordSuccess resets the consecutive failure count (circuit stays closed).
func (s *CircuitBreakerStore) RecordSuccess(policyID, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_circuit_breaker_states
			(policy_id, tenant_id, is_open, consecutive_failures, updated_at)
		VALUES ($1, $2, false, 0, $3)
		ON CONFLICT (policy_id) DO UPDATE SET
			consecutive_failures = 0,
			updated_at           = $3`,
		policyID, tenantID, now,
	)
	return err
}

// Reset manually resets the circuit breaker to closed with zero failures.
func (s *CircuitBreakerStore) Reset(policyID, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_circuit_breaker_states
			(policy_id, tenant_id, is_open, consecutive_failures, updated_at)
		VALUES ($1, $2, false, 0, $3)
		ON CONFLICT (policy_id) DO UPDATE SET
			is_open              = false,
			consecutive_failures = 0,
			opened_at            = NULL,
			will_reset_at        = NULL,
			updated_at           = $3`,
		policyID, tenantID, now,
	)
	return err
}

// AutoReset closes any circuit breakers whose will_reset_at has passed.
// Called opportunistically on each evaluation.
func (s *CircuitBreakerStore) AutoReset(policyID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE policy_circuit_breaker_states
		SET is_open = false, consecutive_failures = 0, opened_at = NULL, will_reset_at = NULL,
			updated_at = NOW()
		WHERE policy_id = $1 AND is_open = true AND will_reset_at <= NOW()`,
		policyID,
	)
	return err
}
