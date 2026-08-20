package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// WorkflowStore handles all four workflow tables:
// incident_commanders, stakeholder_templates, ticket_syncs, status_communications.
type WorkflowStore struct {
	db *sql.DB
}

func NewWorkflowStore(db *sql.DB) *WorkflowStore {
	return &WorkflowStore{db: db}
}

// ── Incident Commander ────────────────────────────────────────────────────────

// AssignCommander relieves the current active commander and inserts the new one atomically.
func (s *WorkflowStore) AssignCommander(c *models.IncidentCommander) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `
		UPDATE incident_commanders
		   SET relieved_at = $1
		 WHERE incident_id = $2 AND relieved_at IS NULL`,
		now, c.IncidentID,
	); err != nil {
		return fmt.Errorf("relieve previous commander: %w", err)
	}

	c.AssignedAt = now
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO incident_commanders
		  (id, tenant_id, incident_id, user_id, user_name, user_email,
		   assigned_by, notes, assigned_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID, c.TenantID, c.IncidentID, c.UserID, c.UserName, c.UserEmail,
		c.AssignedBy, c.Notes, c.AssignedAt,
	); err != nil {
		return fmt.Errorf("insert commander: %w", err)
	}
	return tx.Commit()
}

func (s *WorkflowStore) CurrentCommander(tenantID, incidentID string) (*models.IncidentCommander, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var c models.IncidentCommander
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, incident_id, user_id, user_name,
		       COALESCE(user_email,''), assigned_by, COALESCE(notes,''), assigned_at
		FROM incident_commanders
		WHERE tenant_id=$1 AND incident_id=$2 AND relieved_at IS NULL
		ORDER BY assigned_at DESC LIMIT 1`,
		tenantID, incidentID,
	).Scan(&c.ID, &c.TenantID, &c.IncidentID, &c.UserID, &c.UserName,
		&c.UserEmail, &c.AssignedBy, &c.Notes, &c.AssignedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *WorkflowStore) CommanderHistory(tenantID, incidentID string) ([]models.IncidentCommander, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, incident_id, user_id, user_name,
		       COALESCE(user_email,''), assigned_by, COALESCE(notes,''), assigned_at, relieved_at
		FROM incident_commanders
		WHERE tenant_id=$1 AND incident_id=$2
		ORDER BY assigned_at DESC`,
		tenantID, incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.IncidentCommander
	for rows.Next() {
		var c models.IncidentCommander
		if err := rows.Scan(&c.ID, &c.TenantID, &c.IncidentID, &c.UserID, &c.UserName,
			&c.UserEmail, &c.AssignedBy, &c.Notes, &c.AssignedAt, &c.RelievedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Stakeholder Templates ─────────────────────────────────────────────────────

func (s *WorkflowStore) CreateTemplate(t *models.StakeholderTemplate) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stakeholder_templates
		  (id, tenant_id, name, channel, subject, body, is_default, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.TenantID, t.Name, t.Channel, t.Subject, t.Body,
		t.IsDefault, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

func (s *WorkflowStore) GetTemplate(id string) (*models.StakeholderTemplate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, channel, COALESCE(subject,''), body, is_default, created_at, updated_at
		FROM stakeholder_templates WHERE id=$1`, id)
	var t models.StakeholderTemplate
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Channel, &t.Subject, &t.Body,
		&t.IsDefault, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *WorkflowStore) ListTemplates(tenantID string) ([]models.StakeholderTemplate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, channel, COALESCE(subject,''), body, is_default, created_at, updated_at
		FROM stakeholder_templates
		WHERE tenant_id=$1
		ORDER BY is_default DESC, name ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.StakeholderTemplate
	for rows.Next() {
		var t models.StakeholderTemplate
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Channel, &t.Subject, &t.Body,
			&t.IsDefault, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *WorkflowStore) UpdateTemplate(t *models.StakeholderTemplate) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE stakeholder_templates
		   SET name=$1, channel=$2, subject=$3, body=$4, is_default=$5, updated_at=$6
		 WHERE id=$7 AND tenant_id=$8`,
		t.Name, t.Channel, t.Subject, t.Body, t.IsDefault, t.UpdatedAt, t.ID, t.TenantID,
	)
	return err
}

func (s *WorkflowStore) DeleteTemplate(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM stakeholder_templates WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

// ── Ticket Sync ───────────────────────────────────────────────────────────────

func (s *WorkflowStore) SaveTicketSync(ts *models.TicketSync) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	ts.CreatedAt = now
	ts.SyncedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ticket_syncs
		  (id, tenant_id, incident_id, provider, ticket_key, ticket_url,
		   ticket_status, created_by, created_at, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (incident_id, provider) DO UPDATE
		  SET ticket_key    = EXCLUDED.ticket_key,
		      ticket_url    = EXCLUDED.ticket_url,
		      ticket_status = EXCLUDED.ticket_status,
		      synced_at     = EXCLUDED.synced_at`,
		ts.ID, ts.TenantID, ts.IncidentID, ts.Provider, ts.TicketKey,
		ts.TicketURL, ts.TicketStatus, ts.CreatedBy, ts.CreatedAt, ts.SyncedAt,
	)
	return err
}

func (s *WorkflowStore) UpdateTicketStatus(incidentID, provider, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE ticket_syncs SET ticket_status=$1, synced_at=NOW()
		WHERE incident_id=$2 AND provider=$3`, status, incidentID, provider)
	return err
}

func (s *WorkflowStore) GetTicketSyncs(tenantID, incidentID string) ([]models.TicketSync, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, incident_id, provider, ticket_key, ticket_url,
		       COALESCE(ticket_status,''), created_by, created_at, synced_at
		FROM ticket_syncs
		WHERE tenant_id=$1 AND incident_id=$2
		ORDER BY created_at DESC`,
		tenantID, incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TicketSync
	for rows.Next() {
		var ts models.TicketSync
		if err := rows.Scan(&ts.ID, &ts.TenantID, &ts.IncidentID, &ts.Provider,
			&ts.TicketKey, &ts.TicketURL, &ts.TicketStatus, &ts.CreatedBy,
			&ts.CreatedAt, &ts.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// ── Status Communications ─────────────────────────────────────────────────────

func (s *WorkflowStore) AddStatusCommunication(sc *models.StatusCommunication) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sc.PublishedAt = time.Now().UTC()
	b, err := json.Marshal(sc.AffectedServices)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO status_communications
		  (id, tenant_id, incident_id, stage, title, body, affected_services, published_by, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		sc.ID, sc.TenantID, sc.IncidentID, sc.Stage, sc.Title, sc.Body,
		string(b), sc.PublishedBy, sc.PublishedAt,
	)
	return err
}

func (s *WorkflowStore) GetStatusCommunications(tenantID, incidentID string) ([]models.StatusCommunication, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, incident_id, stage, title, body, affected_services, published_by, published_at
		FROM status_communications
		WHERE tenant_id=$1 AND incident_id=$2
		ORDER BY published_at DESC`,
		tenantID, incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.StatusCommunication
	for rows.Next() {
		var sc models.StatusCommunication
		var affectedJSON string
		if err := rows.Scan(&sc.ID, &sc.TenantID, &sc.IncidentID, &sc.Stage,
			&sc.Title, &sc.Body, &affectedJSON, &sc.PublishedBy, &sc.PublishedAt); err != nil {
			return nil, err
		}
		if affectedJSON != "" && affectedJSON != "[]" && affectedJSON != "null" {
			_ = json.Unmarshal([]byte(affectedJSON), &sc.AffectedServices)
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}
