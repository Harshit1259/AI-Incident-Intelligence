package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerTenantAdminRoutes wires all SaaS multi-tenancy management endpoints.
// All routes require JWT auth; the handler itself enforces admin role.
func registerTenantAdminRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.TenantAdminHandler,
) {
	// Tenant CRUD + lifecycle
	mux.Handle("/api/v1/admin/tenants", withAuth(h.Handle))
	mux.Handle("/api/v1/admin/tenants/", withAuth(h.Handle))

	// Tenant-scoped audit search
	mux.Handle("/api/v1/admin/audit", withAuth(h.Handle))
	mux.Handle("/api/v1/admin/audit/", withAuth(h.Handle))
}
