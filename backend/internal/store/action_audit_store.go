package store


import (
	"context"
"time"
)
import "database/sql"

// ActionAudit is the enriched record of a single action execution.
// It captures both the operational outcome and the security context
// required for compliance attribution.
type ActionAudit struct {
	ID            int    `json:"id"`
	ActionID      string `json:"action_id"`
	IncidentID    string `json:"incident_id"`
	Approved      bool   `json:"approved"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	ExecutedAt    string `json:"executed_at"`

	// Security + compliance fields
	ActorID       string `json:"actor_id"`
	TenantID      string `json:"tenant_id"`
	SourceIP      string `json:"source_ip"`
	Command       string `json:"command"`
	ActionType    string `json:"action_type"`
	RiskLevel     string `json:"risk_level"`
	ExecutionMode string `json:"execution_mode"`
	PolicyReason  string `json:"policy_reason"`
}

type ActionAuditStore struct {
	db *sql.DB
}

func NewActionAuditStore(db *sql.DB) *ActionAuditStore {
	return &ActionAuditStore{db: db}
}

func (s *ActionAuditStore) AddAudit(audit ActionAudit) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO action_audit (
			action_id, incident_id, approved, status, message, executed_at,
			actor_id, tenant_id, source_ip, command, action_type,
			risk_level, execution_mode, policy_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		audit.ActionID, audit.IncidentID, audit.Approved, audit.Status, audit.Message,
		audit.ExecutedAt, audit.ActorID, audit.TenantID, audit.SourceIP, audit.Command,
		audit.ActionType, audit.RiskLevel, audit.ExecutionMode, audit.PolicyReason,
	)
	return err
}

// GetAuditsByIncident returns action audits for an incident, always filtered by
// the owning tenant to prevent cross-tenant data reads.
func (s *ActionAuditStore) GetAuditsByIncident(tenantID, incidentID string) []ActionAudit {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, action_id, incident_id, approved, status, message, executed_at,
		        actor_id, tenant_id, source_ip, command, action_type,
		        risk_level, execution_mode, policy_reason
		 FROM action_audit
		 WHERE tenant_id = $1 AND incident_id = $2
		 ORDER BY executed_at DESC`,
		tenantID, incidentID,
	)
	if err != nil {
		return []ActionAudit{}
	}
	defer rows.Close()

	result := make([]ActionAudit, 0)
	for rows.Next() {
		var a ActionAudit
		if err := rows.Scan(
			&a.ID, &a.ActionID, &a.IncidentID, &a.Approved, &a.Status, &a.Message,
			&a.ExecutedAt, &a.ActorID, &a.TenantID, &a.SourceIP, &a.Command,
			&a.ActionType, &a.RiskLevel, &a.ExecutionMode, &a.PolicyReason,
		); err != nil {
			return result
		}
		result = append(result, a)
	}
	return result
}

// GetAuditsByTenant returns the most recent N action audits across all incidents
// for a tenant — used for tenant-scoped compliance and activity views.
func (s *ActionAuditStore) GetAuditsByTenant(tenantID string, limit int) []ActionAudit {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, action_id, incident_id, approved, status, message, executed_at,
		        actor_id, tenant_id, source_ip, command, action_type,
		        risk_level, execution_mode, policy_reason
		 FROM action_audit
		 WHERE tenant_id = $1
		 ORDER BY executed_at DESC
		 LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return []ActionAudit{}
	}
	defer rows.Close()

	result := make([]ActionAudit, 0)
	for rows.Next() {
		var a ActionAudit
		if err := rows.Scan(
			&a.ID, &a.ActionID, &a.IncidentID, &a.Approved, &a.Status, &a.Message,
			&a.ExecutedAt, &a.ActorID, &a.TenantID, &a.SourceIP, &a.Command,
			&a.ActionType, &a.RiskLevel, &a.ExecutionMode, &a.PolicyReason,
		); err != nil {
			return result
		}
		result = append(result, a)
	}
	return result
}
