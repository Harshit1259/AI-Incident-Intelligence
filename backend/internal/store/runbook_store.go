package store

import (
	"context"
	"time"
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

type RunbookStore struct {
	db *sql.DB
}

func NewRunbookStore(db *sql.DB) *RunbookStore {
	return &RunbookStore{db: db}
}

func (s *RunbookStore) Create(r models.Runbook) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO runbooks (id, tenant_id, service, title, content, tags, severity_match, pattern_match, created_by, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		r.ID, r.TenantID, r.Service, r.Title, r.Content, r.Tags, r.SeverityMatch, r.PatternMatch, r.CreatedBy, r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *RunbookStore) GetByService(service string) ([]models.Runbook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, title, content, tags, severity_match, pattern_match, created_by, created_at, updated_at
		 FROM runbooks WHERE service = $1 ORDER BY updated_at DESC`,
		service,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.Runbook
	for rows.Next() {
		var r models.Runbook
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Service, &r.Title, &r.Content, &r.Tags,
			&r.SeverityMatch, &r.PatternMatch, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *RunbookStore) GetByID(id string) (*models.Runbook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var r models.Runbook
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service, title, content, tags, severity_match, pattern_match, created_by, created_at, updated_at
		 FROM runbooks WHERE id = $1`, id,
	).Scan(&r.ID, &r.TenantID, &r.Service, &r.Title, &r.Content, &r.Tags,
		&r.SeverityMatch, &r.PatternMatch, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *RunbookStore) Search(tenantID, query string) ([]models.Runbook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	likePattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, title, content, tags, severity_match, pattern_match, created_by, created_at, updated_at
		 FROM runbooks
		 WHERE tenant_id = $1 AND (title ILIKE $2 OR content ILIKE $2 OR tags ILIKE $2)
		 ORDER BY updated_at DESC
		 LIMIT 50`,
		tenantID, likePattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.Runbook
	for rows.Next() {
		var r models.Runbook
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Service, &r.Title, &r.Content, &r.Tags,
			&r.SeverityMatch, &r.PatternMatch, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *RunbookStore) Update(r models.Runbook) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE runbooks SET service=$2, title=$3, content=$4, tags=$5, severity_match=$6, pattern_match=$7, updated_at=$8
		 WHERE id=$1`,
		r.ID, r.Service, r.Title, r.Content, r.Tags, r.SeverityMatch, r.PatternMatch, r.UpdatedAt,
	)
	return err
}

func (s *RunbookStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM runbooks WHERE id = $1`, id)
	return err
}
