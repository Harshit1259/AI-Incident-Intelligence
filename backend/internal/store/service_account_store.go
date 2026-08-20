package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ServiceAccountStore manages machine identity records.
type ServiceAccountStore struct {
	db *sql.DB
}

// NewServiceAccountStore returns a new ServiceAccountStore.
func NewServiceAccountStore(db *sql.DB) *ServiceAccountStore {
	return &ServiceAccountStore{db: db}
}

// Create inserts a new service account record.
func (s *ServiceAccountStore) Create(sa models.ServiceAccount) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO service_accounts (
			id, tenant_id, name, description, role, custom_role_id,
			enabled, created_by, last_used_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		sa.ID, sa.TenantID, sa.Name, sa.Description, sa.Role,
		sa.CustomRoleID, sa.Enabled, sa.CreatedBy, sa.LastUsedAt,
		now, now,
	)
	return err
}

// GetByID returns the service account with the given id scoped to tenantID.
func (s *ServiceAccountStore) GetByID(id, tenantID string) (*models.ServiceAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, description, role, custom_role_id,
		       enabled, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanServiceAccount(row)
}

// List returns all service accounts for a tenant.
func (s *ServiceAccountStore) List(tenantID string) ([]models.ServiceAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, description, role, custom_role_id,
		       enabled, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE tenant_id = $1
		ORDER BY created_at ASC LIMIT 200`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ServiceAccount
	for rows.Next() {
		sa, err := scanServiceAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sa)
	}
	return out, rows.Err()
}

// SetEnabled enables or disables a service account.
func (s *ServiceAccountStore) SetEnabled(id, tenantID string, enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE service_accounts SET enabled = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		enabled, id, tenantID,
	)
	return err
}

// UpdateLastUsed records the last time a service account was used.
func (s *ServiceAccountStore) UpdateLastUsed(id string, t time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE service_accounts SET last_used_at = $1 WHERE id = $2`,
		t, id,
	)
	return err
}

// Delete removes a service account record.
func (s *ServiceAccountStore) Delete(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM service_accounts WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// ── scan helper ───────────────────────────────────────────────────────────────

func scanServiceAccount(row interface{ Scan(...interface{}) error }) (*models.ServiceAccount, error) {
	var sa models.ServiceAccount
	err := row.Scan(
		&sa.ID, &sa.TenantID, &sa.Name, &sa.Description,
		&sa.Role, &sa.CustomRoleID, &sa.Enabled, &sa.CreatedBy,
		&sa.LastUsedAt, &sa.CreatedAt, &sa.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &sa, nil
}
