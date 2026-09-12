package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// IdentityHandler handles domain verifications, org hierarchy (projects/workspaces),
// custom RBAC roles, service accounts, and API keys.
type IdentityHandler struct {
	domainSvc    *services.DomainVerificationService
	rbacSvc      *services.RBACService
	projectStore *identityProjectStore
	saSvc        *services.ServiceAccountService
	apiKeySvc    *services.APIKeyService
	jwtSecret    string
}

// identityProjectStore is a thin wrapper — the actual store is accessed via services.
// We embed the org hierarchy store directly to avoid creating yet another service.
type identityProjectStore interface {
	ListProjects(tenantID string) ([]models.Project, error)
	CreateProject(p models.Project) error
	GetProject(id, tenantID string) (*models.Project, error)
	UpdateProject(p models.Project) error
	DeleteProject(id, tenantID string) error
	ListWorkspaces(projectID, tenantID string) ([]models.Workspace, error)
	CreateWorkspace(w models.Workspace) error
	GetWorkspace(id, tenantID string) (*models.Workspace, error)
	UpdateWorkspace(w models.Workspace) error
	DeleteWorkspace(id, tenantID string) error
}

// NewIdentityHandler creates a new IdentityHandler.
func NewIdentityHandler(
	domainSvc *services.DomainVerificationService,
	rbacSvc *services.RBACService,
	orgStore identityProjectStore,
	saSvc *services.ServiceAccountService,
	apiKeySvc *services.APIKeyService,
	jwtSecret string,
) *IdentityHandler {
	return &IdentityHandler{
		domainSvc:    domainSvc,
		rbacSvc:      rbacSvc,
		projectStore: &orgStore,
		saSvc:        saSvc,
		apiKeySvc:    apiKeySvc,
		jwtSecret:    jwtSecret,
	}
}

// Handle dispatches all identity routes.
func (h *IdentityHandler) Handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	// ── Domain verifications ───────────────────────────────────────────────────
	case path == "/api/v1/domains":
		h.domainList(w, r)
	case path == "/api/v1/domains/initiate":
		h.domainInitiate(w, r)
	case strings.HasSuffix(path, "/verify") && strings.HasPrefix(path, "/api/v1/domains/"):
		domain := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/domains/"), "/verify")
		h.domainVerify(w, r, domain)
	case strings.HasPrefix(path, "/api/v1/domains/"):
		domain := strings.TrimPrefix(path, "/api/v1/domains/")
		if r.Method == http.MethodDelete {
			h.domainDelete(w, r, domain)
		} else {
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// ── Projects ───────────────────────────────────────────────────────────────
	case path == "/api/v1/projects":
		switch r.Method {
		case http.MethodGet:
			h.listProjects(w, r)
		case http.MethodPost:
			h.createProject(w, r)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasPrefix(path, "/api/v1/projects/") && strings.Contains(path, "/workspaces"):
		h.workspaceRoutes(w, r, path)
	case strings.HasPrefix(path, "/api/v1/projects/"):
		id := strings.TrimPrefix(path, "/api/v1/projects/")
		switch r.Method {
		case http.MethodGet:
			h.getProject(w, r, id)
		case http.MethodPut:
			h.updateProject(w, r, id)
		case http.MethodDelete:
			h.deleteProject(w, r, id)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// ── Workspaces ─────────────────────────────────────────────────────────────
	case strings.HasPrefix(path, "/api/v1/workspaces/"):
		id := strings.TrimPrefix(path, "/api/v1/workspaces/")
		switch r.Method {
		case http.MethodGet:
			h.getWorkspace(w, r, id)
		case http.MethodPut:
			h.updateWorkspace(w, r, id)
		case http.MethodDelete:
			h.deleteWorkspace(w, r, id)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// ── Custom Roles ───────────────────────────────────────────────────────────
	case path == "/api/v1/roles":
		switch r.Method {
		case http.MethodGet:
			h.listRoles(w, r)
		case http.MethodPost:
			h.createRole(w, r)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasSuffix(path, "/assign") && strings.HasPrefix(path, "/api/v1/roles/"):
		roleID := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/roles/"), "/assign")
		switch r.Method {
		case http.MethodPost:
			h.assignRole(w, r, roleID)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasPrefix(path, "/api/v1/roles/") && strings.Contains(path, "/assign/"):
		h.revokeRoleAssignment(w, r, path)
	case strings.HasPrefix(path, "/api/v1/roles/"):
		id := strings.TrimPrefix(path, "/api/v1/roles/")
		switch r.Method {
		case http.MethodGet:
			h.getRole(w, r, id)
		case http.MethodPut:
			h.updateRole(w, r, id)
		case http.MethodDelete:
			h.deleteRole(w, r, id)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// ── Service accounts ───────────────────────────────────────────────────────
	case path == "/api/v1/service-accounts":
		switch r.Method {
		case http.MethodGet:
			h.listServiceAccounts(w, r)
		case http.MethodPost:
			h.createServiceAccount(w, r)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasSuffix(path, "/token") && strings.HasPrefix(path, "/api/v1/service-accounts/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/service-accounts/"), "/token")
		h.issueServiceAccountToken(w, r, id)
	case strings.HasPrefix(path, "/api/v1/service-accounts/"):
		id := strings.TrimPrefix(path, "/api/v1/service-accounts/")
		switch r.Method {
		case http.MethodGet:
			h.getServiceAccount(w, r, id)
		case http.MethodPut:
			h.updateServiceAccount(w, r, id)
		case http.MethodDelete:
			h.deleteServiceAccount(w, r, id)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	// ── API keys ───────────────────────────────────────────────────────────────
	case path == "/api/v1/api-keys":
		switch r.Method {
		case http.MethodGet:
			h.listAPIKeys(w, r)
		case http.MethodPost:
			h.createAPIKey(w, r)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasSuffix(path, "/rotate") && strings.HasPrefix(path, "/api/v1/api-keys/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/api-keys/"), "/rotate")
		h.rotateAPIKey(w, r, id)
	case strings.HasPrefix(path, "/api/v1/api-keys/"):
		id := strings.TrimPrefix(path, "/api/v1/api-keys/")
		switch r.Method {
		case http.MethodDelete:
			h.revokeAPIKey(w, r, id)
		case http.MethodGet:
			h.getAPIKey(w, r, id)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	default:
		idWriteErr(w, http.StatusNotFound, "route not found")
	}
}

// ── Domain verification routes ─────────────────────────────────────────────────

func (h *IdentityHandler) domainList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	domains, err := h.domainSvc.ListByTenant(claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if domains == nil {
		domains = []models.DomainVerification{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

func (h *IdentityHandler) domainInitiate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	dv, err := h.domainSvc.InitiateVerification(claims.TenantID, body.Domain)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, dv)
}

func (h *IdentityHandler) domainVerify(w http.ResponseWriter, r *http.Request, domain string) {
	if r.Method != http.MethodPost {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	dv, err := h.domainSvc.VerifyDomain(claims.TenantID, domain)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, dv)
}

func (h *IdentityHandler) domainDelete(w http.ResponseWriter, r *http.Request, domain string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.domainSvc.DeleteDomain(claims.TenantID, domain); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Project routes ─────────────────────────────────────────────────────────────

func (h *IdentityHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ps, err := (*h.projectStore).ListProjects(claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ps == nil {
		ps = []models.Project{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"projects": ps})
}

func (h *IdentityHandler) createProject(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	p.ID = idNewID()
	p.TenantID = claims.TenantID
	p.CreatedBy = claims.UserID
	if err := (*h.projectStore).CreateProject(p); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, p)
}

func (h *IdentityHandler) getProject(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, err := (*h.projectStore).GetProject(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "project not found")
		return
	}
	idWriteJSON(w, http.StatusOK, p)
}

func (h *IdentityHandler) updateProject(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	existing, err := (*h.projectStore).GetProject(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "project not found")
		return
	}
	var update models.Project
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	update.ID = existing.ID
	update.TenantID = existing.TenantID
	update.CreatedBy = existing.CreatedBy
	if err := (*h.projectStore).UpdateProject(update); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, update)
}

func (h *IdentityHandler) deleteProject(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := (*h.projectStore).DeleteProject(id, claims.TenantID); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Workspace routes ───────────────────────────────────────────────────────────

func (h *IdentityHandler) workspaceRoutes(w http.ResponseWriter, r *http.Request, path string) {
	// /api/v1/projects/:project_id/workspaces[/:workspace_id]
	rest := strings.TrimPrefix(path, "/api/v1/projects/")
	parts := strings.SplitN(rest, "/workspaces", 2)
	projectID := parts[0]

	if len(parts) == 1 || parts[1] == "" || parts[1] == "/" {
		switch r.Method {
		case http.MethodGet:
			h.listWorkspaces(w, r, projectID)
		case http.MethodPost:
			h.createWorkspace(w, r, projectID)
		default:
			idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	// workspace sub-routes not dispatched here — see workspaces prefix above
	idWriteErr(w, http.StatusNotFound, "not found")
}

func (h *IdentityHandler) listWorkspaces(w http.ResponseWriter, r *http.Request, projectID string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ws, err := (*h.projectStore).ListWorkspaces(projectID, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ws == nil {
		ws = []models.Workspace{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"workspaces": ws})
}

func (h *IdentityHandler) createWorkspace(w http.ResponseWriter, r *http.Request, projectID string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var ws models.Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	ws.ID = idNewID()
	ws.TenantID = claims.TenantID
	ws.ProjectID = projectID
	ws.CreatedBy = claims.UserID
	if err := (*h.projectStore).CreateWorkspace(ws); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, ws)
}

func (h *IdentityHandler) getWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ws, err := (*h.projectStore).GetWorkspace(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "workspace not found")
		return
	}
	idWriteJSON(w, http.StatusOK, ws)
}

func (h *IdentityHandler) updateWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	existing, err := (*h.projectStore).GetWorkspace(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "workspace not found")
		return
	}
	var update models.Workspace
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	update.ID = existing.ID
	update.TenantID = existing.TenantID
	update.ProjectID = existing.ProjectID
	update.CreatedBy = existing.CreatedBy
	if err := (*h.projectStore).UpdateWorkspace(update); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, update)
}

func (h *IdentityHandler) deleteWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := (*h.projectStore).DeleteWorkspace(id, claims.TenantID); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Role routes ────────────────────────────────────────────────────────────────

func (h *IdentityHandler) listRoles(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	roles, err := h.rbacSvc.ListRoles(claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if roles == nil {
		roles = []models.CustomRole{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"roles": roles})
}

func (h *IdentityHandler) createRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	role, err := h.rbacSvc.CreateRole(claims.TenantID, body.Name, body.Description, body.Permissions, claims.UserID)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, role)
}

func (h *IdentityHandler) getRole(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	role, err := h.rbacSvc.GetRole(claims.TenantID, id)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "role not found")
		return
	}
	idWriteJSON(w, http.StatusOK, role)
}

func (h *IdentityHandler) updateRole(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	role, err := h.rbacSvc.UpdateRole(claims.TenantID, id, body.Name, body.Description, body.Permissions)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, role)
}

func (h *IdentityHandler) deleteRole(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.rbacSvc.DeleteRole(claims.TenantID, id); err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityHandler) assignRole(w http.ResponseWriter, r *http.Request, roleID string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		UserID    string           `json:"user_id"`
		ScopeType models.ScopeType `json:"scope_type"`
		ScopeID   string           `json:"scope_id"`
		ExpiresAt *time.Time       `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if body.ScopeType == "" {
		body.ScopeType = models.ScopeOrg
	}
	if err := h.rbacSvc.AssignRole(claims.TenantID, body.UserID, roleID, body.ScopeType, body.ScopeID, claims.UserID, body.ExpiresAt); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, map[string]string{"status": "assigned"})
}

func (h *IdentityHandler) revokeRoleAssignment(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodDelete {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// path: /api/v1/roles/:role_id/assign/:user_id
	rest := strings.TrimPrefix(path, "/api/v1/roles/")
	parts := strings.SplitN(rest, "/assign/", 2)
	if len(parts) != 2 {
		idWriteErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	roleID := parts[0]
	userID := parts[1]
	if err := h.rbacSvc.RevokeUserRole(claims.TenantID, userID, roleID); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Service account routes ─────────────────────────────────────────────────────

func (h *IdentityHandler) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sas, err := h.saSvc.List(claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sas == nil {
		sas = []models.ServiceAccount{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"service_accounts": sas})
}

func (h *IdentityHandler) createServiceAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	sa, err := h.saSvc.Create(claims.TenantID, body.Name, body.Description, body.Role, claims.UserID)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idWriteJSON(w, http.StatusCreated, sa)
}

func (h *IdentityHandler) getServiceAccount(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sa, err := h.saSvc.GetByID(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "service account not found")
		return
	}
	idWriteJSON(w, http.StatusOK, sa)
}

func (h *IdentityHandler) updateServiceAccount(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := h.saSvc.SetEnabled(id, claims.TenantID, body.Enabled); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}

func (h *IdentityHandler) deleteServiceAccount(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.saSvc.Delete(id, claims.TenantID); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityHandler) issueServiceAccountToken(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TTLSeconds int `json:"ttl_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		idWriteErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.TTLSeconds <= 0 {
		body.TTLSeconds = 3600
	}

	sa, err := h.saSvc.GetByID(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "service account not found")
		return
	}
	token, err := h.saSvc.IssueJWT(*sa, h.jwtSecret, time.Duration(body.TTLSeconds)*time.Second)
	if err != nil {
		idWriteErr(w, http.StatusForbidden, err.Error())
		return
	}
	idWriteJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ── API key routes ─────────────────────────────────────────────────────────────

func (h *IdentityHandler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := h.apiKeySvc.List(claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []models.APIKey{}
	}
	idWriteJSON(w, http.StatusOK, map[string]interface{}{"api_keys": keys})
}

func (h *IdentityHandler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name      string                 `json:"name"`
		OwnerID   string                 `json:"owner_id"`
		OwnerType models.APIKeyOwnerType `json:"owner_type"`
		Scopes    []string               `json:"scopes"`
		ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		idWriteErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if body.OwnerID == "" {
		body.OwnerID = claims.UserID
	}
	if body.OwnerType == "" {
		body.OwnerType = models.APIKeyOwnerUser
	}
	key, err := h.apiKeySvc.Create(claims.TenantID, body.OwnerID, body.OwnerType, body.Name, body.Scopes, body.ExpiresAt, claims.UserID)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// RawKey is set and returned once here.
	idWriteJSON(w, http.StatusCreated, key)
}

func (h *IdentityHandler) getAPIKey(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	key, err := h.apiKeySvc.GetByID(id, claims.TenantID)
	if err != nil {
		idWriteErr(w, http.StatusNotFound, "api key not found")
		return
	}
	key.RawKey = "" // never return the raw key on read
	idWriteJSON(w, http.StatusOK, key)
}

func (h *IdentityHandler) revokeAPIKey(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.apiKeySvc.Revoke(id, claims.TenantID, claims.UserID); err != nil {
		idWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *IdentityHandler) rotateAPIKey(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		idWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		idWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	newKey, err := h.apiKeySvc.Rotate(id, claims.TenantID, claims.UserID)
	if err != nil {
		idWriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// RawKey is set once here.
	idWriteJSON(w, http.StatusCreated, newKey)
}

// ── internal helpers ───────────────────────────────────────────────────────────

func idNewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func idWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func idWriteErr(w http.ResponseWriter, status int, msg string) {
	idWriteJSON(w, status, map[string]string{"error": msg})
}
