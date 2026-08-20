package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// AccessControlStore manages custom roles, role assignments and attribute policies.
type AccessControlStore struct {
	db *sql.DB
}

// NewAccessControlStore returns a new AccessControlStore.
func NewAccessControlStore(db *sql.DB) *AccessControlStore {
	return &AccessControlStore{db: db}
}

// ── Custom Roles ──────────────────────────────────────────────────────────────

// CreateRole inserts a new custom role.
func (s *AccessControlStore) CreateRole(r models.CustomRole) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	permsJSON, err := marshalStringSlice(r.Permissions)
	if err != nil {
		return fmt.Errorf("access_control_store create role: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO custom_roles (id, tenant_id, name, description, is_system, permissions_json, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.TenantID, r.Name, r.Description, r.IsSystem, permsJSON, r.CreatedBy, now, now,
	)
	return err
}

// GetRole returns the custom role with the given id scoped to tenantID.
func (s *AccessControlStore) GetRole(id, tenantID string) (*models.CustomRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, description, is_system, permissions_json, created_by, created_at, updated_at
		FROM custom_roles
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanCustomRole(row)
}

// GetRoleByName returns the custom role with the given name scoped to tenantID.
func (s *AccessControlStore) GetRoleByName(name, tenantID string) (*models.CustomRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, description, is_system, permissions_json, created_by, created_at, updated_at
		FROM custom_roles
		WHERE name = $1 AND tenant_id = $2`,
		name, tenantID,
	)
	return scanCustomRole(row)
}

// ListRoles returns all custom roles for a tenant.
func (s *AccessControlStore) ListRoles(tenantID string) ([]models.CustomRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, description, is_system, permissions_json, created_by, created_at, updated_at
		FROM custom_roles
		WHERE tenant_id = $1
		ORDER BY name ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CustomRole
	for rows.Next() {
		r, err := scanCustomRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// UpdateRole updates the mutable fields of a custom role.
func (s *AccessControlStore) UpdateRole(r models.CustomRole) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	permsJSON, err := marshalStringSlice(r.Permissions)
	if err != nil {
		return fmt.Errorf("access_control_store update role: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE custom_roles SET name = $1, description = $2, permissions_json = $3, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`,
		r.Name, r.Description, permsJSON, r.ID, r.TenantID,
	)
	return err
}

// DeleteRole removes a custom role and all its assignments.
func (s *AccessControlStore) DeleteRole(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, 
		`DELETE FROM role_assignments WHERE role_id = $1 AND tenant_id = $2`, id, tenantID,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, 
		`DELETE FROM custom_roles WHERE id = $1 AND tenant_id = $2`, id, tenantID,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// GetPermissionsForRole returns the permission list for a custom role.
func (s *AccessControlStore) GetPermissionsForRole(roleID, tenantID string) ([]string, error) {
	role, err := s.GetRole(roleID, tenantID)
	if err != nil {
		return nil, err
	}
	return role.Permissions, nil
}

// ── Role Assignments ──────────────────────────────────────────────────────────

// AssignRole inserts a role assignment record.
func (s *AccessControlStore) AssignRole(ra models.RoleAssignment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO role_assignments (id, tenant_id, user_id, role_id, scope_type, scope_id, assigned_by, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())`,
		ra.ID, ra.TenantID, ra.UserID, ra.RoleID,
		string(ra.ScopeType), ra.ScopeID, ra.AssignedBy, ra.ExpiresAt,
	)
	return err
}

// GetAssignment returns a single role assignment by id scoped to tenantID.
func (s *AccessControlStore) GetAssignment(id, tenantID string) (*models.RoleAssignment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT ra.id, ra.tenant_id, ra.user_id, ra.role_id,
		       COALESCE(cr.name,'') AS role_name,
		       ra.scope_type, ra.scope_id, ra.assigned_by, ra.expires_at, ra.created_at
		FROM role_assignments ra
		LEFT JOIN custom_roles cr ON cr.id = ra.role_id AND cr.tenant_id = ra.tenant_id
		WHERE ra.id = $1 AND ra.tenant_id = $2`,
		id, tenantID,
	)
	return scanRoleAssignment(row)
}

// GetAssignmentsByUser returns all active role assignments for a user.
func (s *AccessControlStore) GetAssignmentsByUser(tenantID, userID string) ([]models.RoleAssignment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT ra.id, ra.tenant_id, ra.user_id, ra.role_id,
		       COALESCE(cr.name,'') AS role_name,
		       ra.scope_type, ra.scope_id, ra.assigned_by, ra.expires_at, ra.created_at
		FROM role_assignments ra
		LEFT JOIN custom_roles cr ON cr.id = ra.role_id AND cr.tenant_id = ra.tenant_id
		WHERE ra.tenant_id = $1 AND ra.user_id = $2
		  AND (ra.expires_at IS NULL OR ra.expires_at > NOW())
		ORDER BY ra.created_at ASC`,
		tenantID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.RoleAssignment
	for rows.Next() {
		ra, err := scanRoleAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ra)
	}
	return out, rows.Err()
}

// DeleteAssignment removes a role assignment record.
func (s *AccessControlStore) DeleteAssignment(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM role_assignments WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// DeleteAssignmentByUserRole removes all assignments for a user+role in a tenant.
func (s *AccessControlStore) DeleteAssignmentByUserRole(tenantID, userID, roleID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM role_assignments WHERE tenant_id = $1 AND user_id = $2 AND role_id = $3`,
		tenantID, userID, roleID,
	)
	return err
}

// ── Attribute Policies (ABAC) ─────────────────────────────────────────────────

// CreatePolicy inserts a new attribute policy.
func (s *AccessControlStore) CreatePolicy(ap models.AttributePolicy) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	condJSON, err := marshalStringMap(ap.Conditions)
	if err != nil {
		return fmt.Errorf("access_control_store create policy: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO attribute_policies (id, tenant_id, role_id, resource_type, action, conditions_json, effect, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())`,
		ap.ID, ap.TenantID, ap.RoleID, ap.ResourceType, ap.Action, condJSON, ap.Effect,
	)
	return err
}

// GetAttributePolicies returns all ABAC policies for a given role scoped to tenantID.
func (s *AccessControlStore) GetAttributePolicies(tenantID, roleID string) ([]models.AttributePolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, role_id, resource_type, action, conditions_json, effect, created_at
		FROM attribute_policies
		WHERE tenant_id = $1 AND role_id = $2
		ORDER BY created_at ASC`,
		tenantID, roleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AttributePolicy
	for rows.Next() {
		var ap models.AttributePolicy
		var condJSON string
		if err := rows.Scan(
			&ap.ID, &ap.TenantID, &ap.RoleID, &ap.ResourceType,
			&ap.Action, &condJSON, &ap.Effect, &ap.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := unmarshalStringMap(condJSON, &ap.Conditions); err != nil {
			return nil, err
		}
		out = append(out, ap)
	}
	return out, rows.Err()
}

// DeletePolicy removes an attribute policy.
func (s *AccessControlStore) DeletePolicy(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM attribute_policies WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// ── internal helpers ──────────────────────────────────────────────────────────

func marshalStringSlice(ss []string) (string, error) {
	if ss == nil {
		return "[]", nil
	}
	b, err := json.Marshal(ss)
	if err != nil {
		return "", fmt.Errorf("marshal string slice: %w", err)
	}
	return string(b), nil
}

func unmarshalStringSlice(s string, out *[]string) error {
	if s == "" || s == "[]" {
		*out = []string{}
		return nil
	}
	var ss []string
	if err := json.Unmarshal([]byte(s), &ss); err != nil {
		return fmt.Errorf("unmarshal string slice: %w", err)
	}
	*out = ss
	return nil
}

func scanCustomRole(row interface{ Scan(...interface{}) error }) (*models.CustomRole, error) {
	var r models.CustomRole
	var permsJSON string
	err := row.Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Description,
		&r.IsSystem, &permsJSON, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := unmarshalStringSlice(permsJSON, &r.Permissions); err != nil {
		return nil, err
	}
	return &r, nil
}

func scanRoleAssignment(row interface{ Scan(...interface{}) error }) (*models.RoleAssignment, error) {
	var ra models.RoleAssignment
	var scopeType string
	err := row.Scan(
		&ra.ID, &ra.TenantID, &ra.UserID, &ra.RoleID,
		&ra.RoleName, &scopeType, &ra.ScopeID,
		&ra.AssignedBy, &ra.ExpiresAt, &ra.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	ra.ScopeType = models.ScopeType(scopeType)
	return &ra, nil
}
