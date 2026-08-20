package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// PostMortemStore handles persistence for post-mortem documents.
type PostMortemStore struct {
	db *sql.DB
}

func NewPostMortemStore(db *sql.DB) *PostMortemStore {
	return &PostMortemStore{db: db}
}

// GetByIncidentID returns the post-mortem for a given incident, or nil if not found.
func (s *PostMortemStore) GetByIncidentID(incidentID string) (*models.PostMortem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, incident_id, tenant_id, title, status, executive_summary,
		       timeline_narrative, root_cause, impact, resolution,
		       action_items_json, lessons, generated_by, created_at, updated_at
		FROM post_mortems
		WHERE incident_id = $1
		ORDER BY created_at DESC
		LIMIT 1`,
		incidentID,
	)
	return s.scanRow(row)
}

// GetByID returns a post-mortem by its primary key, or nil if not found.
func (s *PostMortemStore) GetByID(id string) (*models.PostMortem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, incident_id, tenant_id, title, status, executive_summary,
		       timeline_narrative, root_cause, impact, resolution,
		       action_items_json, lessons, generated_by, created_at, updated_at
		FROM post_mortems
		WHERE id = $1`,
		id,
	)
	return s.scanRow(row)
}

// Create inserts a new post-mortem record.
func (s *PostMortemStore) Create(pm models.PostMortem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actionItemsJSON, err := json.Marshal(pm.ActionItems)
	if err != nil {
		return fmt.Errorf("postmortem_store: marshal action_items: %w", err)
	}

	now := time.Now()
	if pm.CreatedAt.IsZero() {
		pm.CreatedAt = now
	}
	if pm.UpdatedAt.IsZero() {
		pm.UpdatedAt = now
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO post_mortems (
			id, incident_id, tenant_id, title, status, executive_summary,
			timeline_narrative, root_cause, impact, resolution,
			action_items_json, lessons, generated_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		pm.ID, pm.IncidentID, pm.TenantID, pm.Title, pm.Status,
		pm.ExecutiveSummary, pm.TimelineNarrative, pm.RootCause,
		pm.Impact, pm.Resolution, string(actionItemsJSON),
		pm.Lessons, pm.GeneratedBy, pm.CreatedAt, pm.UpdatedAt,
	)
	return err
}

// Update saves changes to an existing post-mortem.
func (s *PostMortemStore) Update(pm models.PostMortem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actionItemsJSON, err := json.Marshal(pm.ActionItems)
	if err != nil {
		return fmt.Errorf("postmortem_store: marshal action_items: %w", err)
	}

	pm.UpdatedAt = time.Now()

	_, err = s.db.ExecContext(ctx, `
		UPDATE post_mortems SET
			title              = $2,
			status             = $3,
			executive_summary  = $4,
			timeline_narrative = $5,
			root_cause         = $6,
			impact             = $7,
			resolution         = $8,
			action_items_json  = $9,
			lessons            = $10,
			updated_at         = $11
		WHERE id = $1`,
		pm.ID, pm.Title, pm.Status, pm.ExecutiveSummary,
		pm.TimelineNarrative, pm.RootCause, pm.Impact, pm.Resolution,
		string(actionItemsJSON), pm.Lessons, pm.UpdatedAt,
	)
	return err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (s *PostMortemStore) scanRow(row *sql.Row) (*models.PostMortem, error) {
	var pm models.PostMortem
	var actionItemsJSON string

	err := row.Scan(
		&pm.ID, &pm.IncidentID, &pm.TenantID, &pm.Title, &pm.Status,
		&pm.ExecutiveSummary, &pm.TimelineNarrative, &pm.RootCause,
		&pm.Impact, &pm.Resolution, &actionItemsJSON,
		&pm.Lessons, &pm.GeneratedBy, &pm.CreatedAt, &pm.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(actionItemsJSON), &pm.ActionItems); err != nil {
		pm.ActionItems = []string{}
	}

	return &pm, nil
}
