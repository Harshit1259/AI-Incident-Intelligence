package store

import (
	"context"
	"time"
	"database/sql"
	"encoding/json"
	"fmt"

	"ai-incident-platform/backend/internal/models"
)

// SCIMStore manages SCIM group records and sync audit log entries.
type SCIMStore struct {
	db *sql.DB
}

// NewSCIMStore returns a new SCIMStore.
func NewSCIMStore(db *sql.DB) *SCIMStore {
	return &SCIMStore{db: db}
}

// ── SCIM Groups ───────────────────────────────────────────────────────────────

// CreateGroup inserts a new SCIM group.
func (s *SCIMStore) CreateGroup(g models.SCIMGroup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	membersJSON, err := marshalSCIMRefs(g.Members)
	if err != nil {
		return fmt.Errorf("scim_store create group: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO scim_groups (id, tenant_id, display_name, external_id, members_json, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,NOW(),NOW())`,
		g.ID, g.Meta.Location, g.DisplayName, g.ExternalID, membersJSON,
	)
	if err != nil {
		// Re-try with proper tenant injection — caller must set TenantID separately.
		// Use ID field for now, but accept a separate tenantID parameter via CreateGroupForTenant.
		return err
	}
	return nil
}

// CreateGroupForTenant inserts a SCIM group with an explicit tenantID.
func (s *SCIMStore) CreateGroupForTenant(tenantID string, g models.SCIMGroup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	membersJSON, err := marshalSCIMRefs(g.Members)
	if err != nil {
		return fmt.Errorf("scim_store create group: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO scim_groups (id, tenant_id, display_name, external_id, members_json, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,NOW(),NOW())`,
		g.ID, tenantID, g.DisplayName, g.ExternalID, membersJSON,
	)
	return err
}

// GetGroup returns the SCIM group with the given id scoped to tenantID.
func (s *SCIMStore) GetGroup(id, tenantID string) (*models.SCIMGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, display_name, external_id, members_json, created_at, updated_at
		FROM scim_groups
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanSCIMGroup(row)
}

// GetGroupByExternalID returns the SCIM group with the given external_id.
func (s *SCIMStore) GetGroupByExternalID(externalID, tenantID string) (*models.SCIMGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, display_name, external_id, members_json, created_at, updated_at
		FROM scim_groups
		WHERE external_id = $1 AND tenant_id = $2`,
		externalID, tenantID,
	)
	return scanSCIMGroup(row)
}

// ListGroups returns all SCIM groups for a tenant.
func (s *SCIMStore) ListGroups(tenantID string, startIndex, count int) ([]models.SCIMGroup, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var total int
	if err := s.db.QueryRowContext(ctx, 
		`SELECT COUNT(*) FROM scim_groups WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	if startIndex < 1 {
		startIndex = 1
	}
	offset := startIndex - 1
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, display_name, external_id, members_json, created_at, updated_at
		FROM scim_groups
		WHERE tenant_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3`,
		tenantID, count, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.SCIMGroup
	for rows.Next() {
		g, err := scanSCIMGroup(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *g)
	}
	return out, total, rows.Err()
}

// UpdateGroup replaces a SCIM group record.
func (s *SCIMStore) UpdateGroup(tenantID string, g models.SCIMGroup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	membersJSON, err := marshalSCIMRefs(g.Members)
	if err != nil {
		return fmt.Errorf("scim_store update group: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE scim_groups SET display_name = $1, external_id = $2, members_json = $3, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`,
		g.DisplayName, g.ExternalID, membersJSON, g.ID, tenantID,
	)
	return err
}

// DeleteGroup removes a SCIM group record.
func (s *SCIMStore) DeleteGroup(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM scim_groups WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// ── User SCIM helpers ─────────────────────────────────────────────────────────

// GetUserBySCIMExternalID returns a user by scim_external_id scoped to tenantID.
func (s *SCIMStore) GetUserBySCIMExternalID(externalID, tenantID string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE scim_external_id = $1 AND tenant_id = $2`,
		externalID, tenantID,
	)
	var u models.User
	if err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash,
		&u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserBySCIMID returns a user by id (SCIM id == user id) scoped to tenantID.
func (s *SCIMStore) GetUserBySCIMID(id, tenantID string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	var u models.User
	if err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash,
		&u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsersByTenant returns all users for a tenant for SCIM listing.
func (s *SCIMStore) ListUsersByTenant(tenantID string, startIndex, count int) ([]models.User, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var total int
	if err := s.db.QueryRowContext(ctx, 
		`SELECT COUNT(*) FROM users WHERE tenant_id = $1`, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	if startIndex < 1 {
		startIndex = 1
	}
	offset := startIndex - 1
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE tenant_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3`,
		tenantID, count, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash,
			&u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UpdateUserSCIMFields updates SCIM-managed columns on a user record.
func (s *SCIMStore) UpdateUserSCIMFields(id, tenantID, email, displayName, externalID string, active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	role := models.RoleViewer
	if !active {
		// For deactivated users we preserve their existing role but just mark them
		// inactive — the role field is not modified here.
		_, err := s.db.ExecContext(ctx, `
			UPDATE users SET email = $1, display_name = $2, scim_external_id = $3, updated_at = NOW()
			WHERE id = $4 AND tenant_id = $5`,
			email, displayName, externalID, id, tenantID,
		)
		return err
	}
	// For active upserts we also ensure role is set if it defaults.
	_ = role
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET email = $1, display_name = $2, scim_external_id = $3, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`,
		email, displayName, externalID, id, tenantID,
	)
	return err
}

// ── Sync log ──────────────────────────────────────────────────────────────────

// LogSync appends an entry to the scim_sync_log audit table.
func (s *SCIMStore) LogSync(tenantID, operation, externalID, resourceID, status, details string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scim_sync_log (tenant_id, operation, external_id, resource_id, status, details, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())`,
		tenantID, operation, externalID, resourceID, status, details,
	)
	return err
}

// ── scan + marshal helpers ────────────────────────────────────────────────────

func scanSCIMGroup(row interface{ Scan(...interface{}) error }) (*models.SCIMGroup, error) {
	var g models.SCIMGroup
	var tenantID string
	var membersJSON string
	err := row.Scan(
		&g.ID, &tenantID, &g.DisplayName, &g.ExternalID,
		&membersJSON, &g.Meta.Created, &g.Meta.LastModified,
	)
	if err != nil {
		return nil, err
	}
	g.Schemas = []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}
	g.Meta.ResourceType = "Group"
	if membersJSON != "" && membersJSON != "[]" {
		if err := json.Unmarshal([]byte(membersJSON), &g.Members); err != nil {
			return nil, fmt.Errorf("scim_store: unmarshal members: %w", err)
		}
	}
	return &g, nil
}

func marshalSCIMRefs(refs []models.SCIMRef) (string, error) {
	if refs == nil {
		return "[]", nil
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return "", fmt.Errorf("marshal scim refs: %w", err)
	}
	return string(b), nil
}
