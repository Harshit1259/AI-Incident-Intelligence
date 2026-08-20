package models

import "time"

// ── Fine-grained permission strings (resource:action format) ─────────────────

const (
	PermIncidentsRead     = "incidents:read"
	PermIncidentsWrite    = "incidents:write"
	PermIncidentsDelete   = "incidents:delete"
	PermSourcesRead       = "sources:read"
	PermSourcesWrite      = "sources:write"
	PermEventsRead        = "events:read"
	PermEventsWrite       = "events:write"
	PermUsersRead         = "users:read"
	PermUsersWrite        = "users:write"
	PermRolesManage       = "roles:manage"
	PermAPIKeysManage     = "apikeys:manage"
	PermServiceAcctManage = "service_accounts:manage"
	PermAuditRead         = "audit:read"
	PermTenantAdmin       = "tenant:admin"
	PermIngest            = "ingest:write"
	PermAnalyticsRead     = "analytics:read"
	PermPoliciesRead      = "policies:read"
	PermPoliciesWrite     = "policies:write"
	PermSSOManage         = "sso:manage"
	PermDomainsManage     = "domains:manage"
	PermProjectsManage    = "projects:manage"
	PermWorkspacesManage  = "workspaces:manage"
	PermSCIMProvision     = "scim:provision"
)

// AllPermissions is the canonical list used for role validation.
var AllPermissions = []string{
	PermIncidentsRead, PermIncidentsWrite, PermIncidentsDelete,
	PermSourcesRead, PermSourcesWrite,
	PermEventsRead, PermEventsWrite,
	PermUsersRead, PermUsersWrite,
	PermRolesManage, PermAPIKeysManage, PermServiceAcctManage,
	PermAuditRead, PermTenantAdmin, PermIngest, PermAnalyticsRead,
	PermPoliciesRead, PermPoliciesWrite,
	PermSSOManage, PermDomainsManage,
	PermProjectsManage, PermWorkspacesManage, PermSCIMProvision,
}

// SystemRolePermissions maps the 3 built-in roles to their permission sets.
var SystemRolePermissions = map[string][]string{
	RoleViewer: {
		PermIncidentsRead, PermSourcesRead, PermEventsRead,
		PermAnalyticsRead, PermAuditRead,
	},
	RoleOperator: {
		PermIncidentsRead, PermIncidentsWrite,
		PermSourcesRead, PermSourcesWrite,
		PermEventsRead, PermEventsWrite,
		PermAnalyticsRead, PermAuditRead,
		PermPoliciesRead, PermIngest,
	},
	RoleAdmin: AllPermissions,
}

// ── Scope types ───────────────────────────────────────────────────────────────

// ScopeType identifies the level at which a role assignment applies.
type ScopeType string

const (
	ScopeOrg       ScopeType = "org"
	ScopeProject   ScopeType = "project"
	ScopeWorkspace ScopeType = "workspace"
)

// ── Custom role ───────────────────────────────────────────────────────────────

// CustomRole is a tenant-defined role with an explicit permission set.
type CustomRole struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	Permissions []string  `json:"permissions"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ── Role assignment ───────────────────────────────────────────────────────────

// RoleAssignment binds a user to a role at a particular scope.
type RoleAssignment struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	UserID     string     `json:"user_id"`
	RoleID     string     `json:"role_id"`
	RoleName   string     `json:"role_name,omitempty"` // denormalised for display
	ScopeType  ScopeType  `json:"scope_type"`
	ScopeID    string     `json:"scope_id,omitempty"`
	AssignedBy string     `json:"assigned_by"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ── Attribute policy ──────────────────────────────────────────────────────────

// AttributePolicy is an ABAC rule that conditionally allows or denies
// a specific action on a resource type for a given role.
type AttributePolicy struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	RoleID       string            `json:"role_id"`
	ResourceType string            `json:"resource_type"`
	Action       string            `json:"action"`
	Conditions   map[string]string `json:"conditions"`
	Effect       string            `json:"effect"` // "allow" | "deny"
	CreatedAt    time.Time         `json:"created_at"`
}
