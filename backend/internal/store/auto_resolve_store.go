package store

import (
	"context"
	"time"
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

type AutoResolveStore struct {
	db *sql.DB
}

func NewAutoResolveStore(db *sql.DB) *AutoResolveStore {
	return &AutoResolveStore{db: db}
}

func (s *AutoResolveStore) Create(r models.AutoResolveRule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO auto_resolve_rules (id, tenant_id, name, pattern, service, severity, action, cooldown_minutes, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.TenantID, r.Name, r.Pattern, r.Service, r.Severity, r.Action, r.CooldownMinutes, r.Enabled,
	)
	return err
}

func (s *AutoResolveStore) GetRules(tenantID string, limit, offset int) ([]models.AutoResolveRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, pattern, service, severity, action, cooldown_minutes, enabled, times_fired, created_at
		 FROM auto_resolve_rules WHERE tenant_id = $1 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.AutoResolveRule
	for rows.Next() {
		var r models.AutoResolveRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Pattern, &r.Service, &r.Severity,
			&r.Action, &r.CooldownMinutes, &r.Enabled, &r.TimesFired, &r.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *AutoResolveStore) GetRuleByID(id string) (*models.AutoResolveRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var r models.AutoResolveRule
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, name, pattern, service, severity, action, cooldown_minutes, enabled, times_fired, created_at
		 FROM auto_resolve_rules WHERE id = $1`, id,
	).Scan(&r.ID, &r.TenantID, &r.Name, &r.Pattern, &r.Service, &r.Severity,
		&r.Action, &r.CooldownMinutes, &r.Enabled, &r.TimesFired, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *AutoResolveStore) UpdateRule(r models.AutoResolveRule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE auto_resolve_rules SET name=$2, pattern=$3, service=$4, severity=$5, action=$6, cooldown_minutes=$7, enabled=$8
		 WHERE id=$1`,
		r.ID, r.Name, r.Pattern, r.Service, r.Severity, r.Action, r.CooldownMinutes, r.Enabled,
	)
	return err
}

func (s *AutoResolveStore) DeleteRule(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM auto_resolve_rules WHERE id = $1`, id)
	return err
}

func (s *AutoResolveStore) IncrementFired(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `UPDATE auto_resolve_rules SET times_fired = times_fired + 1 WHERE id = $1`, id)
	return err
}

// FindMatchingRules finds enabled rules that match the given service, severity, and pattern text.
func (s *AutoResolveStore) FindMatchingRules(service, severity, pattern string) ([]models.AutoResolveRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, name, pattern, service, severity, action, cooldown_minutes, enabled, times_fired, created_at
		 FROM auto_resolve_rules
		 WHERE enabled = true
		   AND (service = '' OR service = $1)
		   AND (severity = '' OR severity = $2)
		   AND (pattern = '' OR $3 LIKE '%' || pattern || '%')
		 ORDER BY created_at ASC`,
		service, severity, pattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.AutoResolveRule
	for rows.Next() {
		var r models.AutoResolveRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Pattern, &r.Service, &r.Severity,
			&r.Action, &r.CooldownMinutes, &r.Enabled, &r.TimesFired, &r.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
