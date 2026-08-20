package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// RBACService handles fine-grained permission checks (RBAC + ABAC).
type RBACService struct {
	acStore *store.AccessControlStore
}

// NewRBACService creates a new RBACService.
func NewRBACService(acStore *store.AccessControlStore) *RBACService {
	return &RBACService{acStore: acStore}
}

// GetUserPermissions collects all permissions for a user from their role assignments.
// For users with the built-in role (admin/operator/viewer) this returns the
// SystemRolePermissions set directly. Custom role assignments are also collected.
func (s *RBACService) GetUserPermissions(tenantID, userID string) []string {
	// Collect unique permissions.
	seen := make(map[string]struct{})
	var perms []string

	addPerm := func(p string) {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			perms = append(perms, p)
		}
	}

	// Get custom role assignments.
	assignments, err := s.acStore.GetAssignmentsByUser(tenantID, userID)
	if err != nil {
		return perms
	}

	for _, ra := range assignments {
		// Try custom role first.
		rolePerms, err := s.acStore.GetPermissionsForRole(ra.RoleID, tenantID)
		if err == nil {
			for _, p := range rolePerms {
				addPerm(p)
			}
			continue
		}
		// Fall back to system role name (ra.RoleID may be a system role name).
		if sysPerms, ok := models.SystemRolePermissions[ra.RoleID]; ok {
			for _, p := range sysPerms {
				addPerm(p)
			}
		}
	}

	return perms
}

// HasPermission checks whether a user has a specific permission, optionally
// evaluating ABAC attribute conditions.
func (s *RBACService) HasPermission(tenantID, userID, permission string, attrs map[string]string) bool {
	assignments, err := s.acStore.GetAssignmentsByUser(tenantID, userID)
	if err != nil {
		return false
	}

	for _, ra := range assignments {
		// Check custom role permissions.
		rolePerms, err := s.acStore.GetPermissionsForRole(ra.RoleID, tenantID)
		if err != nil {
			// Try as system role name.
			if sysPerms, ok := models.SystemRolePermissions[ra.RoleID]; ok {
				for _, p := range sysPerms {
					if p == permission {
						return true
					}
				}
			}
			continue
		}
		found := false
		for _, p := range rolePerms {
			if p == permission {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// Check ABAC policies for this role.
		policies, err := s.acStore.GetAttributePolicies(tenantID, ra.RoleID)
		if err != nil || len(policies) == 0 {
			return true // no ABAC restrictions, permission granted
		}

		allowed := evaluateABACPolicies(policies, permission, attrs)
		if allowed {
			return true
		}
	}
	return false
}

// SystemRoleToPermissions returns the permission set for a built-in role.
func (s *RBACService) SystemRoleToPermissions(role string) []string {
	if perms, ok := models.SystemRolePermissions[role]; ok {
		return perms
	}
	return nil
}

// CreateRole creates a new custom role for a tenant.
func (s *RBACService) CreateRole(tenantID, name, description string, perms []string, createdBy string) (*models.CustomRole, error) {
	// Validate permissions.
	valid := make(map[string]struct{})
	for _, p := range models.AllPermissions {
		valid[p] = struct{}{}
	}
	for _, p := range perms {
		if _, ok := valid[p]; !ok {
			return nil, fmt.Errorf("rbac: unknown permission %q", p)
		}
	}

	role := models.CustomRole{
		ID:          generateID(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		IsSystem:    false,
		Permissions: perms,
		CreatedBy:   createdBy,
	}
	if err := s.acStore.CreateRole(role); err != nil {
		return nil, fmt.Errorf("rbac: create role: %w", err)
	}
	return &role, nil
}

// UpdateRole updates the name, description, and permissions of a custom role.
func (s *RBACService) UpdateRole(tenantID, roleID, name, description string, perms []string) (*models.CustomRole, error) {
	role, err := s.acStore.GetRole(roleID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("rbac: role not found: %w", err)
	}
	if role.IsSystem {
		return nil, fmt.Errorf("rbac: cannot modify system role %q", role.Name)
	}
	role.Name = name
	role.Description = description
	role.Permissions = perms
	role.UpdatedAt = time.Now().UTC()
	if err := s.acStore.UpdateRole(*role); err != nil {
		return nil, fmt.Errorf("rbac: update role: %w", err)
	}
	return role, nil
}

// DeleteRole removes a custom role and all its assignments.
func (s *RBACService) DeleteRole(tenantID, roleID string) error {
	role, err := s.acStore.GetRole(roleID, tenantID)
	if err != nil {
		return fmt.Errorf("rbac: role not found: %w", err)
	}
	if role.IsSystem {
		return fmt.Errorf("rbac: cannot delete system role %q", role.Name)
	}
	return s.acStore.DeleteRole(roleID, tenantID)
}

// ListRoles returns all custom roles for a tenant.
func (s *RBACService) ListRoles(tenantID string) ([]models.CustomRole, error) {
	return s.acStore.ListRoles(tenantID)
}

// GetRole returns a single custom role.
func (s *RBACService) GetRole(tenantID, roleID string) (*models.CustomRole, error) {
	return s.acStore.GetRole(roleID, tenantID)
}

// AssignRole creates a role assignment for a user.
func (s *RBACService) AssignRole(tenantID, userID, roleID string, scope models.ScopeType, scopeID, assignedBy string, expiresAt *time.Time) error {
	ra := models.RoleAssignment{
		ID:         generateID(),
		TenantID:   tenantID,
		UserID:     userID,
		RoleID:     roleID,
		ScopeType:  scope,
		ScopeID:    scopeID,
		AssignedBy: assignedBy,
		ExpiresAt:  expiresAt,
	}
	return s.acStore.AssignRole(ra)
}

// RevokeAssignment removes a role assignment by its ID.
func (s *RBACService) RevokeAssignment(tenantID, assignmentID string) error {
	return s.acStore.DeleteAssignment(assignmentID, tenantID)
}

// RevokeUserRole removes all assignments of a role from a user.
func (s *RBACService) RevokeUserRole(tenantID, userID, roleID string) error {
	return s.acStore.DeleteAssignmentByUserRole(tenantID, userID, roleID)
}

// GetAssignmentsByUser returns all role assignments for a user.
func (s *RBACService) GetAssignmentsByUser(tenantID, userID string) ([]models.RoleAssignment, error) {
	return s.acStore.GetAssignmentsByUser(tenantID, userID)
}

// CreateAttributePolicy creates an ABAC policy.
func (s *RBACService) CreateAttributePolicy(tenantID, roleID, resourceType, action, effect string, conditions map[string]string) (*models.AttributePolicy, error) {
	if effect != "allow" && effect != "deny" {
		return nil, fmt.Errorf("rbac: effect must be 'allow' or 'deny'")
	}
	policy := models.AttributePolicy{
		ID:           generateID(),
		TenantID:     tenantID,
		RoleID:       roleID,
		ResourceType: resourceType,
		Action:       action,
		Conditions:   conditions,
		Effect:       effect,
	}
	if err := s.acStore.CreatePolicy(policy); err != nil {
		return nil, fmt.Errorf("rbac: create attribute policy: %w", err)
	}
	return &policy, nil
}

// GetAttributePolicies returns all ABAC policies for a role.
func (s *RBACService) GetAttributePolicies(tenantID, roleID string) ([]models.AttributePolicy, error) {
	return s.acStore.GetAttributePolicies(tenantID, roleID)
}

// ── ABAC evaluation ───────────────────────────────────────────────────────────

// evaluateABACPolicies checks whether a permission + attribute set is allowed
// by the policies. Returns true if any allow policy matches and no deny policy matches.
func evaluateABACPolicies(policies []models.AttributePolicy, permission string, attrs map[string]string) bool {
	// Check for explicit deny first (deny always wins).
	for _, p := range policies {
		if p.Effect == "deny" && matchesPolicy(p, permission, attrs) {
			return false
		}
	}
	// Check for any allow policy.
	for _, p := range policies {
		if p.Effect == "allow" && matchesPolicy(p, permission, attrs) {
			return true
		}
	}
	// Default: allow if no restrictive policies.
	return true
}

// matchesPolicy returns true when the policy's action matches the requested permission
// and all conditions are satisfied.
func matchesPolicy(p models.AttributePolicy, permission string, attrs map[string]string) bool {
	if p.Action != permission && p.Action != "*" {
		return false
	}
	// All conditions must match.
	for k, v := range p.Conditions {
		if attrs[k] != v {
			return false
		}
	}
	return true
}
