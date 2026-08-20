package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ActionExecutionStore provides persistence for action execution tracking.
type ActionExecutionStore struct {
	db *sql.DB
}

// NewActionExecutionStore creates a new ActionExecutionStore.
func NewActionExecutionStore(db *sql.DB) *ActionExecutionStore {
	return &ActionExecutionStore{db: db}
}

// Create inserts a new action execution record.
func (s *ActionExecutionStore) Create(e models.ActionExecution) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO action_executions (id, tenant_id, incident_id, action_id, action_label, action_desc,
			chain_position, status, execution_mode, result_output, error_message,
			started_at, completed_at, verified, verification_result, created_at,
			requires_approval, approval_status, approved_by, approved_at,
			rejected_by, rejection_reason, rollback_ref, rollback_reason, rolled_back_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		e.ID, e.TenantID, e.IncidentID, e.ActionID, e.ActionLabel, e.ActionDesc,
		e.ChainPosition, e.Status, e.ExecutionMode, e.ResultOutput, e.ErrorMessage,
		e.StartedAt, e.CompletedAt, e.Verified, e.VerificationResult, e.CreatedAt,
		e.RequiresApproval, e.ApprovalStatus, e.ApprovedBy, e.ApprovedAt,
		e.RejectedBy, e.RejectionReason, e.RollbackRef, e.RollbackReason, e.RolledBackBy,
	)
	return err
}

// GetByIncidentID returns all action executions for an incident, ordered by chain position.
func (s *ActionExecutionStore) GetByIncidentID(incidentID string) ([]models.ActionExecution, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, incident_id, action_id, action_label, action_desc,
			chain_position, status, execution_mode, result_output, error_message,
			started_at, completed_at, verified, verification_result, created_at
		FROM action_executions
		WHERE incident_id = $1
		ORDER BY chain_position ASC, created_at ASC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []models.ActionExecution
	for rows.Next() {
		var e models.ActionExecution
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.TenantID, &e.IncidentID, &e.ActionID,
			&e.ActionLabel, &e.ActionDesc, &e.ChainPosition, &e.Status,
			&e.ExecutionMode, &e.ResultOutput, &e.ErrorMessage,
			&startedAt, &completedAt, &e.Verified, &e.VerificationResult, &e.CreatedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			e.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			e.CompletedAt = &completedAt.Time
		}
		executions = append(executions, e)
	}
	return executions, rows.Err()
}

// GetByID returns a single execution record by ID.
func (s *ActionExecutionStore) GetByID(id string) (*models.ActionExecution, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var e models.ActionExecution
	var startedAt, completedAt, approvedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, incident_id, action_id, action_label, action_desc,
			chain_position, status, execution_mode, result_output, error_message,
			started_at, completed_at, verified, verification_result, created_at,
			requires_approval, approval_status, approved_by, approved_at,
			rejected_by, rejection_reason, rollback_ref, rollback_reason, rolled_back_by
		FROM action_executions WHERE id = $1`, id).Scan(
		&e.ID, &e.TenantID, &e.IncidentID, &e.ActionID, &e.ActionLabel, &e.ActionDesc,
		&e.ChainPosition, &e.Status, &e.ExecutionMode, &e.ResultOutput, &e.ErrorMessage,
		&startedAt, &completedAt, &e.Verified, &e.VerificationResult, &e.CreatedAt,
		&e.RequiresApproval, &e.ApprovalStatus, &e.ApprovedBy, &approvedAt,
		&e.RejectedBy, &e.RejectionReason, &e.RollbackRef, &e.RollbackReason, &e.RolledBackBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if startedAt.Valid {
		e.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		e.CompletedAt = &completedAt.Time
	}
	if approvedAt.Valid {
		e.ApprovedAt = &approvedAt.Time
	}
	return &e, nil
}

// Update updates an existing action execution record.
func (s *ActionExecutionStore) Update(e models.ActionExecution) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE action_executions
		SET status = $1, result_output = $2, error_message = $3,
			started_at = $4, completed_at = $5, verified = $6, verification_result = $7
		WHERE id = $8`,
		e.Status, e.ResultOutput, e.ErrorMessage,
		e.StartedAt, e.CompletedAt, e.Verified, e.VerificationResult, e.ID,
	)
	return err
}

// Approve sets an execution to approved status with approver identity.
func (s *ActionExecutionStore) Approve(id, approvedBy string, approvedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE action_executions
		SET approval_status = 'approved', approved_by = $1, approved_at = $2, status = 'approved'
		WHERE id = $3 AND approval_status = 'pending'`,
		approvedBy, approvedAt, id,
	)
	return err
}

// Reject records a rejection of a pending approval.
func (s *ActionExecutionStore) Reject(id, rejectedBy, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE action_executions
		SET approval_status = 'rejected', rejected_by = $1, rejection_reason = $2,
			status = 'rejected'
		WHERE id = $3 AND approval_status = 'pending'`,
		rejectedBy, reason, id,
	)
	return err
}

// Rollback records a rollback event for a completed execution.
func (s *ActionExecutionStore) Rollback(id, rolledBackBy, reason, rollbackRef string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE action_executions
		SET status = 'rolled_back', rolled_back_by = $1, rollback_reason = $2,
			rollback_ref = $3
		WHERE id = $4`,
		rolledBackBy, reason, rollbackRef, id,
	)
	return err
}
