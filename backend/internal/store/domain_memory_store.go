package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// DomainMemoryStore persists the three pillars of incident-domain memory:
// remediation patterns, deploy signatures, and runbook preferences.
type DomainMemoryStore struct {
	db *sql.DB
}

func NewDomainMemoryStore(db *sql.DB) *DomainMemoryStore {
	return &DomainMemoryStore{db: db}
}

// ── Remediation Patterns ──────────────────────────────────────────────────────

func (s *DomainMemoryStore) SaveRemediationPattern(p models.RemediationPattern) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stepsJSON, _ := json.Marshal(p.RemediationSteps)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO remediation_patterns
			(id, tenant_id, service, error_signature, root_cause_category,
			 remediation_summary, remediation_steps, success_count, failure_count,
			 avg_resolution_mins, last_used_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			service              = EXCLUDED.service,
			error_signature      = EXCLUDED.error_signature,
			root_cause_category  = EXCLUDED.root_cause_category,
			remediation_summary  = EXCLUDED.remediation_summary,
			remediation_steps    = EXCLUDED.remediation_steps,
			success_count        = EXCLUDED.success_count,
			failure_count        = EXCLUDED.failure_count,
			avg_resolution_mins  = EXCLUDED.avg_resolution_mins,
			last_used_at         = EXCLUDED.last_used_at`,
		p.ID, p.TenantID, p.Service, p.ErrorSignature, p.RootCauseCategory,
		p.RemediationSummary, string(stepsJSON), p.SuccessCount, p.FailureCount,
		p.AvgResolutionMins, time.Now(), time.Now(),
	)
	return err
}

func (s *DomainMemoryStore) ListRemediationPatterns(tenantID, service string) ([]models.RemediationPattern, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `SELECT id, tenant_id, service, error_signature, root_cause_category,
		remediation_summary, remediation_steps, success_count, failure_count,
		avg_resolution_mins, last_used_at, created_at
		FROM remediation_patterns WHERE tenant_id = $1`
	args := []any{tenantID}
	if service != "" {
		query += " AND service = $2"
		args = append(args, service)
	}
	query += " ORDER BY success_count DESC, last_used_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemediationPatterns(rows)
}

func (s *DomainMemoryStore) GetRemediationPattern(tenantID, id string) (models.RemediationPattern, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `SELECT id, tenant_id, service, error_signature, root_cause_category,
		remediation_summary, remediation_steps, success_count, failure_count,
		avg_resolution_mins, last_used_at, created_at
		FROM remediation_patterns WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	p, err := scanRemediationPattern(row)
	return p, err == nil
}

func (s *DomainMemoryStore) UpdateRemediationPattern(tenantID, id string, p models.RemediationPattern) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stepsJSON, _ := json.Marshal(p.RemediationSteps)
	_, err := s.db.ExecContext(ctx, `
		UPDATE remediation_patterns SET
			service = $3, error_signature = $4, root_cause_category = $5,
			remediation_summary = $6, remediation_steps = $7, last_used_at = NOW()
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, p.Service, p.ErrorSignature, p.RootCauseCategory,
		p.RemediationSummary, string(stepsJSON),
	)
	return err
}

func (s *DomainMemoryStore) DeleteRemediationPattern(tenantID, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM remediation_patterns WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

func (s *DomainMemoryStore) MarkRemediationOutcome(tenantID, id string, success bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if success {
		_, err := s.db.ExecContext(ctx, `UPDATE remediation_patterns SET success_count = success_count + 1, last_used_at = NOW() WHERE tenant_id = $1 AND id = $2`, tenantID, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE remediation_patterns SET failure_count = failure_count + 1, last_used_at = NOW() WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

func (s *DomainMemoryStore) FindRemediationsByService(tenantID, service string, limit int) ([]models.RemediationPattern, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, service, error_signature, root_cause_category,
			remediation_summary, remediation_steps, success_count, failure_count,
			avg_resolution_mins, last_used_at, created_at
		FROM remediation_patterns
		WHERE tenant_id = $1 AND service = $2
		ORDER BY success_count DESC, last_used_at DESC LIMIT $3`,
		tenantID, service, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemediationPatterns(rows)
}

// ── Deploy Signatures ─────────────────────────────────────────────────────────

func (s *DomainMemoryStore) SaveDeploySignature(d models.DeploySignature) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indicatorsJSON, _ := json.Marshal(d.Indicators)
	impactedJSON, _ := json.Marshal(d.ImpactedServices)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deploy_signatures
			(id, tenant_id, service, signature_name, description, indicators,
			 impacted_services, typical_severity, occurrence_count, first_seen_at, last_seen_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			signature_name    = EXCLUDED.signature_name,
			description       = EXCLUDED.description,
			indicators        = EXCLUDED.indicators,
			impacted_services = EXCLUDED.impacted_services,
			typical_severity  = EXCLUDED.typical_severity,
			last_seen_at      = NOW()`,
		d.ID, d.TenantID, d.Service, d.SignatureName, d.Description,
		string(indicatorsJSON), string(impactedJSON), d.TypicalSeverity,
		d.OccurrenceCount, time.Now(), time.Now(), time.Now(),
	)
	return err
}

func (s *DomainMemoryStore) ListDeploySignatures(tenantID, service string) ([]models.DeploySignature, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `SELECT id, tenant_id, service, signature_name, description, indicators,
		impacted_services, typical_severity, occurrence_count, first_seen_at, last_seen_at, created_at
		FROM deploy_signatures WHERE tenant_id = $1`
	args := []any{tenantID}
	if service != "" {
		query += " AND service = $2"
		args = append(args, service)
	}
	query += " ORDER BY occurrence_count DESC, last_seen_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeploySignatures(rows)
}

func (s *DomainMemoryStore) GetDeploySignature(tenantID, id string) (models.DeploySignature, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `SELECT id, tenant_id, service, signature_name, description, indicators,
		impacted_services, typical_severity, occurrence_count, first_seen_at, last_seen_at, created_at
		FROM deploy_signatures WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	d, err := scanDeploySignature(row)
	return d, err == nil
}

func (s *DomainMemoryStore) UpdateDeploySignature(tenantID, id string, d models.DeploySignature) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indicatorsJSON, _ := json.Marshal(d.Indicators)
	impactedJSON, _ := json.Marshal(d.ImpactedServices)
	_, err := s.db.ExecContext(ctx, `
		UPDATE deploy_signatures SET
			signature_name = $3, description = $4, indicators = $5,
			impacted_services = $6, typical_severity = $7, last_seen_at = NOW()
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, d.SignatureName, d.Description,
		string(indicatorsJSON), string(impactedJSON), d.TypicalSeverity,
	)
	return err
}

func (s *DomainMemoryStore) DeleteDeploySignature(tenantID, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM deploy_signatures WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

func (s *DomainMemoryStore) BumpDeploySignatureOccurrence(tenantID, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE deploy_signatures SET occurrence_count = occurrence_count + 1, last_seen_at = NOW()
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

func (s *DomainMemoryStore) FindDeploySignaturesByService(tenantID, service string, limit int) ([]models.DeploySignature, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, service, signature_name, description, indicators,
			impacted_services, typical_severity, occurrence_count, first_seen_at, last_seen_at, created_at
		FROM deploy_signatures
		WHERE tenant_id = $1 AND (service = $2 OR service = '')
		ORDER BY occurrence_count DESC LIMIT $3`,
		tenantID, service, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeploySignatures(rows)
}

// ── Runbook Preferences ───────────────────────────────────────────────────────

func (s *DomainMemoryStore) SaveRunbookPreference(p models.RunbookPreference) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runbook_preferences
			(id, tenant_id, team_name, service, preference_key, preference_value, context, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (tenant_id, team_name, service, preference_key) DO UPDATE SET
			preference_value = EXCLUDED.preference_value,
			context          = EXCLUDED.context,
			updated_at       = NOW()`,
		p.ID, p.TenantID, p.TeamName, p.Service, p.PreferenceKey, p.PreferenceValue, p.Context,
		time.Now(), time.Now(),
	)
	return err
}

func (s *DomainMemoryStore) ListRunbookPreferences(tenantID, team, service string) ([]models.RunbookPreference, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `SELECT id, tenant_id, team_name, service, preference_key, preference_value, context, created_at, updated_at
		FROM runbook_preferences WHERE tenant_id = $1`
	args := []any{tenantID}
	if team != "" {
		args = append(args, team)
		query += fmt.Sprintf(" AND team_name = $%d", len(args))
	}
	if service != "" {
		args = append(args, service)
		query += fmt.Sprintf(" AND service = $%d", len(args))
	}
	query += " ORDER BY team_name, service, preference_key"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunbookPreferences(rows)
}

func (s *DomainMemoryStore) GetRunbookPreference(tenantID, id string) (models.RunbookPreference, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `SELECT id, tenant_id, team_name, service, preference_key, preference_value, context, created_at, updated_at
		FROM runbook_preferences WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	p, err := scanRunbookPreference(row)
	return p, err == nil
}

func (s *DomainMemoryStore) UpdateRunbookPreference(tenantID, id string, p models.RunbookPreference) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE runbook_preferences SET
			team_name = $3, service = $4, preference_key = $5,
			preference_value = $6, context = $7, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, p.TeamName, p.Service, p.PreferenceKey, p.PreferenceValue, p.Context,
	)
	return err
}

func (s *DomainMemoryStore) DeleteRunbookPreference(tenantID, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM runbook_preferences WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

type rowScannable interface {
	Scan(dest ...any) error
}

func scanRemediationPattern(row rowScannable) (models.RemediationPattern, error) {
	var p models.RemediationPattern
	var stepsJSON string
	var lastUsed, created time.Time
	err := row.Scan(&p.ID, &p.TenantID, &p.Service, &p.ErrorSignature, &p.RootCauseCategory,
		&p.RemediationSummary, &stepsJSON, &p.SuccessCount, &p.FailureCount,
		&p.AvgResolutionMins, &lastUsed, &created)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(stepsJSON), &p.RemediationSteps)
	if p.RemediationSteps == nil {
		p.RemediationSteps = []string{}
	}
	total := p.SuccessCount + p.FailureCount
	if total > 0 {
		p.SuccessRate = float64(p.SuccessCount) / float64(total)
	}
	p.LastUsedAt = lastUsed.Format(time.RFC3339)
	p.CreatedAt = created.Format(time.RFC3339)
	return p, nil
}

func scanRemediationPatterns(rows *sql.Rows) ([]models.RemediationPattern, error) {
	var out []models.RemediationPattern
	for rows.Next() {
		p, err := scanRemediationPattern(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []models.RemediationPattern{}
	}
	return out, rows.Err()
}

func scanDeploySignature(row rowScannable) (models.DeploySignature, error) {
	var d models.DeploySignature
	var indicatorsJSON, impactedJSON string
	var firstSeen, lastSeen, created time.Time
	err := row.Scan(&d.ID, &d.TenantID, &d.Service, &d.SignatureName, &d.Description,
		&indicatorsJSON, &impactedJSON, &d.TypicalSeverity, &d.OccurrenceCount,
		&firstSeen, &lastSeen, &created)
	if err != nil {
		return d, err
	}
	_ = json.Unmarshal([]byte(indicatorsJSON), &d.Indicators)
	_ = json.Unmarshal([]byte(impactedJSON), &d.ImpactedServices)
	if d.Indicators == nil {
		d.Indicators = []string{}
	}
	if d.ImpactedServices == nil {
		d.ImpactedServices = []string{}
	}
	d.FirstSeenAt = firstSeen.Format(time.RFC3339)
	d.LastSeenAt = lastSeen.Format(time.RFC3339)
	d.CreatedAt = created.Format(time.RFC3339)
	return d, nil
}

func scanDeploySignatures(rows *sql.Rows) ([]models.DeploySignature, error) {
	var out []models.DeploySignature
	for rows.Next() {
		d, err := scanDeploySignature(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if out == nil {
		out = []models.DeploySignature{}
	}
	return out, rows.Err()
}

func scanRunbookPreference(row rowScannable) (models.RunbookPreference, error) {
	var p models.RunbookPreference
	var created, updated time.Time
	err := row.Scan(&p.ID, &p.TenantID, &p.TeamName, &p.Service,
		&p.PreferenceKey, &p.PreferenceValue, &p.Context, &created, &updated)
	if err != nil {
		return p, err
	}
	p.CreatedAt = created.Format(time.RFC3339)
	p.UpdatedAt = updated.Format(time.RFC3339)
	return p, nil
}

func scanRunbookPreferences(rows *sql.Rows) ([]models.RunbookPreference, error) {
	var out []models.RunbookPreference
	for rows.Next() {
		p, err := scanRunbookPreference(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []models.RunbookPreference{}
	}
	return out, rows.Err()
}
