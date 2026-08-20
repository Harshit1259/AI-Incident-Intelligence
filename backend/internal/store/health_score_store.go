package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// HealthScoreStore provides raw signal queries and snapshot persistence for the
// Customer Health Score feature. It reads from existing tables (incidents, events,
// source_registry, action_executions, platform_audit_log) so no data is duplicated.
type HealthScoreStore struct {
	db *sql.DB
}

func NewHealthScoreStore(db *sql.DB) *HealthScoreStore {
	return &HealthScoreStore{db: db}
}

// HealthSignalsRaw holds the raw numeric signals queried from the DB.
// The service layer converts these into scored dimensions.
type HealthSignalsRaw struct {
	LastIncidentTime     *time.Time
	IntegrationCount     int
	EventsThisWeek       int
	EventsLastWeek       int
	AutoRemediationCount int
	ExplainUseCount      int
	CopilotUseCount      int
	TenantCreatedAt      time.Time
	TenantName           string
}

// GetRawSignals queries all four health signal dimensions for the given tenant.
func (s *HealthScoreStore) GetRawSignals(tenantID string) (*HealthSignalsRaw, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	raw := &HealthSignalsRaw{}

	// Tenant metadata (name + creation date for context)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(name,''), COALESCE(created_at, NOW()) FROM tenants WHERE id=$1`,
		tenantID,
	).Scan(&raw.TenantName, &raw.TenantCreatedAt)

	// Last incident time (most recent event time across all incidents for this tenant)
	var lastIncident sql.NullTime
	_ = s.db.QueryRowContext(ctx,
		`SELECT MAX(last_event_time) FROM incidents WHERE COALESCE(tenant_id,'default') = $1`,
		tenantID,
	).Scan(&lastIncident)
	if lastIncident.Valid {
		raw.LastIncidentTime = &lastIncident.Time
	}

	// Active integration count (healthy or error — both mean connected)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM source_registry WHERE tenant_id=$1`,
		tenantID,
	).Scan(&raw.IntegrationCount)

	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	twoWeeksAgo := now.Add(-14 * 24 * time.Hour)

	// Alert volume: this week vs the prior week (trend signal)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE tenant_id=$1 AND timestamp >= $2`,
		tenantID, weekAgo,
	).Scan(&raw.EventsThisWeek)

	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE tenant_id=$1 AND timestamp >= $2 AND timestamp < $3`,
		tenantID, twoWeeksAgo, weekAgo,
	).Scan(&raw.EventsLastWeek)

	// Auto-remediation: action executions in the last 14 days
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM action_executions WHERE tenant_id=$1 AND created_at >= $2`,
		tenantID, twoWeeksAgo,
	).Scan(&raw.AutoRemediationCount)

	// AI feature usage from audit log (explain and copilot handlers write these)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM platform_audit_log
		 WHERE tenant_id=$1 AND action='incident.explain' AND created_at >= $2`,
		tenantID, twoWeeksAgo,
	).Scan(&raw.ExplainUseCount)

	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM platform_audit_log
		 WHERE tenant_id=$1 AND action='incident.copilot' AND created_at >= $2`,
		tenantID, twoWeeksAgo,
	).Scan(&raw.CopilotUseCount)

	return raw, nil
}

// GetAllTenantIDs returns all tenant IDs, for the all-tenants health dashboard.
func (s *HealthScoreStore) GetAllTenantIDs() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM tenants WHERE COALESCE(state,'active') = 'active' ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SaveSnapshot persists a computed health score so trend data is retained.
func (s *HealthScoreStore) SaveSnapshot(score models.TenantHealthScore) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	signalsJSON, _ := json.Marshal(score.Signals)
	actionsJSON, _ := json.Marshal(score.Actions)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tenant_health_snapshots
		    (tenant_id, score, churn_risk, signals_json, actions_json, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		score.TenantID, score.Score, string(score.ChurnRisk),
		string(signalsJSON), string(actionsJSON), score.ComputedAt,
	)
	return err
}

// LogAction records a CSM action (call, email, note) taken on a tenant.
func (s *HealthScoreStore) LogAction(a models.HealthAction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tenant_health_actions (tenant_id, actor, action_type, notes)
		VALUES ($1, $2, $3, $4)`,
		a.TenantID, a.Actor, a.ActionType, a.Notes,
	)
	return err
}

// ListActions returns all CSM actions for a tenant, newest first.
func (s *HealthScoreStore) ListActions(tenantID string) ([]models.HealthAction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, actor, action_type, notes, created_at
		FROM tenant_health_actions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT 100`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []models.HealthAction
	for rows.Next() {
		var a models.HealthAction
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Actor, &a.ActionType, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
