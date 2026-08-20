package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// VerificationStore provides persistence for closed-loop verification records.
type VerificationStore struct {
	db *sql.DB
}

// NewVerificationStore creates a new VerificationStore.
func NewVerificationStore(db *sql.DB) *VerificationStore {
	return &VerificationStore{db: db}
}

// Create inserts a new verification record.
func (s *VerificationStore) Create(v models.VerificationRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checksJSON, _ := json.Marshal(v.Checks)
	beforeJSON, _ := json.Marshal(v.BeforeSnapshot)
	afterJSON, _ := json.Marshal(v.AfterSnapshot)
	proofJSON, _ := json.Marshal(v.ProofItems)

	if v.ProofItems == nil {
		proofJSON = []byte("[]")
	}
	if v.BeforeSnapshot == nil {
		beforeJSON = []byte("{}")
	}
	if v.AfterSnapshot == nil {
		afterJSON = []byte("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO verification_records
			(id, tenant_id, incident_id, execution_id, strategy, status,
			 checks_json, result,
			 before_snapshot_json, after_snapshot_json,
			 confidence_before, confidence_after,
			 proof_items_json, rollback_triggered, auto_close_eligible,
			 verified_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		v.ID, v.TenantID, v.IncidentID, v.ExecutionID, v.Strategy, v.Status,
		string(checksJSON), v.Result,
		string(beforeJSON), string(afterJSON),
		v.ConfidenceBefore, v.ConfidenceAfter,
		string(proofJSON), v.RollbackTriggered, v.AutoCloseEligible,
		v.VerifiedAt, v.CreatedAt,
	)
	return err
}

// GetByID returns a single verification record by its ID.
func (s *VerificationStore) GetByID(id string) (*models.VerificationRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, incident_id, execution_id, strategy, status,
		       checks_json, result,
		       before_snapshot_json, after_snapshot_json,
		       confidence_before, confidence_after,
		       proof_items_json, rollback_triggered, auto_close_eligible,
		       verified_at, created_at
		FROM verification_records WHERE id = $1`, id)

	r, err := scanVerificationRecord(row.Scan)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("verification record not found: %s", id)
	}
	return r, err
}

// GetByIncidentID returns all verification records for an incident, newest first.
func (s *VerificationStore) GetByIncidentID(incidentID string) ([]models.VerificationRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, incident_id, execution_id, strategy, status,
		       checks_json, result,
		       before_snapshot_json, after_snapshot_json,
		       confidence_before, confidence_after,
		       proof_items_json, rollback_triggered, auto_close_eligible,
		       verified_at, created_at
		FROM verification_records
		WHERE incident_id = $1
		ORDER BY created_at DESC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.VerificationRecord
	for rows.Next() {
		r, err := scanVerificationRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, *r)
	}
	return records, rows.Err()
}

// Update persists status, checks, result, and snapshot/proof fields.
func (s *VerificationStore) Update(v models.VerificationRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checksJSON, _ := json.Marshal(v.Checks)
	beforeJSON, _ := json.Marshal(v.BeforeSnapshot)
	afterJSON, _ := json.Marshal(v.AfterSnapshot)
	proofJSON, _ := json.Marshal(v.ProofItems)

	if v.ProofItems == nil {
		proofJSON = []byte("[]")
	}
	if v.BeforeSnapshot == nil {
		beforeJSON = []byte("{}")
	}
	if v.AfterSnapshot == nil {
		afterJSON = []byte("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE verification_records
		SET status               = $1,
		    checks_json          = $2,
		    result               = $3,
		    before_snapshot_json = $4,
		    after_snapshot_json  = $5,
		    confidence_before    = $6,
		    confidence_after     = $7,
		    proof_items_json     = $8,
		    rollback_triggered   = $9,
		    auto_close_eligible  = $10,
		    verified_at          = $11
		WHERE id = $12`,
		v.Status, string(checksJSON), v.Result,
		string(beforeJSON), string(afterJSON),
		v.ConfidenceBefore, v.ConfidenceAfter,
		string(proofJSON), v.RollbackTriggered, v.AutoCloseEligible,
		v.VerifiedAt, v.ID,
	)
	return err
}

// CountRecentByIncident counts verification records for an incident in the given window.
func (s *VerificationStore) CountRecentByIncident(incidentID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM verification_records
		WHERE incident_id = $1 AND created_at >= $2`, incidentID, since).Scan(&count)
	return count, err
}

// scanVerificationRecord scans a row using the provided Scan func.
func scanVerificationRecord(scan func(...any) error) (*models.VerificationRecord, error) {
	var r models.VerificationRecord
	var checksJSON, beforeJSON, afterJSON, proofJSON string
	var verifiedAt sql.NullTime

	if err := scan(
		&r.ID, &r.TenantID, &r.IncidentID, &r.ExecutionID,
		&r.Strategy, &r.Status, &checksJSON, &r.Result,
		&beforeJSON, &afterJSON,
		&r.ConfidenceBefore, &r.ConfidenceAfter,
		&proofJSON, &r.RollbackTriggered, &r.AutoCloseEligible,
		&verifiedAt, &r.CreatedAt,
	); err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(checksJSON), &r.Checks)
	if r.Checks == nil {
		r.Checks = []models.VerificationCheck{}
	}

	if beforeJSON != "" && beforeJSON != "{}" {
		var snap models.VerificationSnapshot
		if err := json.Unmarshal([]byte(beforeJSON), &snap); err == nil {
			r.BeforeSnapshot = &snap
		}
	}
	if afterJSON != "" && afterJSON != "{}" {
		var snap models.VerificationSnapshot
		if err := json.Unmarshal([]byte(afterJSON), &snap); err == nil {
			r.AfterSnapshot = &snap
		}
	}

	_ = json.Unmarshal([]byte(proofJSON), &r.ProofItems)
	if r.ProofItems == nil {
		r.ProofItems = []models.ProofItem{}
	}

	if verifiedAt.Valid {
		r.VerifiedAt = &verifiedAt.Time
	}
	return &r, nil
}
