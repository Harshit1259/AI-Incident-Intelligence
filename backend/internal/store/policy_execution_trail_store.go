package store

import (
	"context"
	"time"
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

// PolicyExecutionTrailStore provides append-only persistence for the policy
// execution audit trail. Rows must never be updated or deleted.
type PolicyExecutionTrailStore struct {
	db *sql.DB
}

// NewPolicyExecutionTrailStore creates a new PolicyExecutionTrailStore.
func NewPolicyExecutionTrailStore(db *sql.DB) *PolicyExecutionTrailStore {
	return &PolicyExecutionTrailStore{db: db}
}

// Append writes a new trail entry. This is the only mutation allowed on this table.
func (s *PolicyExecutionTrailStore) Append(t models.PolicyExecutionTrail) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_execution_trail (
			id, tenant_id, policy_id, policy_name,
			incident_id, action_id,
			service, environment, severity,
			decision, denial_code, execution_mode, reason,
			is_simulation, is_dry_run, actor, context_json, evaluated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		t.ID, t.TenantID, t.PolicyID, t.PolicyName,
		t.IncidentID, t.ActionID,
		t.Service, t.Environment, t.Severity,
		t.Decision, t.DenialCode, t.ExecutionMode, t.Reason,
		t.IsSimulation, t.IsDryRun, t.Actor, t.ContextJSON, t.EvaluatedAt,
	)
	return err
}

// GetByPolicy returns the most recent trail entries for a specific policy (newest first).
func (s *PolicyExecutionTrailStore) GetByPolicy(policyID string, limit int) ([]models.PolicyExecutionTrail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, policy_id, policy_name,
			incident_id, action_id,
			service, environment, severity,
			decision, denial_code, execution_mode, reason,
			is_simulation, is_dry_run, actor, context_json, evaluated_at
		FROM policy_execution_trail
		WHERE policy_id = $1
		ORDER BY evaluated_at DESC
		LIMIT $2`, policyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrailRows(rows)
}

// GetByTenant returns the most recent trail entries for a tenant (newest first).
func (s *PolicyExecutionTrailStore) GetByTenant(tenantID string, limit int) ([]models.PolicyExecutionTrail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, policy_id, policy_name,
			incident_id, action_id,
			service, environment, severity,
			decision, denial_code, execution_mode, reason,
			is_simulation, is_dry_run, actor, context_json, evaluated_at
		FROM policy_execution_trail
		WHERE tenant_id = $1
		ORDER BY evaluated_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrailRows(rows)
}

// GetByIncident returns all trail entries for a specific incident.
func (s *PolicyExecutionTrailStore) GetByIncident(incidentID string) ([]models.PolicyExecutionTrail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, policy_id, policy_name,
			incident_id, action_id,
			service, environment, severity,
			decision, denial_code, execution_mode, reason,
			is_simulation, is_dry_run, actor, context_json, evaluated_at
		FROM policy_execution_trail
		WHERE incident_id = $1
		ORDER BY evaluated_at DESC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrailRows(rows)
}

func scanTrailRows(rows *sql.Rows) ([]models.PolicyExecutionTrail, error) {
	var entries []models.PolicyExecutionTrail
	for rows.Next() {
		var t models.PolicyExecutionTrail
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.PolicyID, &t.PolicyName,
			&t.IncidentID, &t.ActionID,
			&t.Service, &t.Environment, &t.Severity,
			&t.Decision, &t.DenialCode, &t.ExecutionMode, &t.Reason,
			&t.IsSimulation, &t.IsDryRun, &t.Actor, &t.ContextJSON, &t.EvaluatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, t)
	}
	return entries, rows.Err()
}
