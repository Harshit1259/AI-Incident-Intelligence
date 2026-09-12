package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerTenantAdminRoutes wires all SaaS multi-tenancy management endpoints.
// This is the management plane for the whole platform: it lists every tenant,
// creates them, and changes their state, plan and rate limits. It must be
// admin-only.
//
// The previous comment here claimed "the handler itself enforces admin role" —
// it did not. TenantAdminHandler takes the caller's claims but uses them only
// for audit entries, never for authorization, and the routes were registered
// with bare withAuth. The result: any authenticated user, down to a viewer,
// could list every tenant on the platform, create new ones, and rewrite another
// tenant's rate limits. The gate now lives on the routes, matching how
// registerAdminRoutes guards its own admin surface.
func registerTenantAdminRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireAdmin func(http.HandlerFunc) http.HandlerFunc,
	h *handlers.TenantAdminHandler,
) {
	// Tenant CRUD + lifecycle
	mux.Handle("/api/v1/admin/tenants", withAuth(requireAdmin(h.Handle)))
	mux.Handle("/api/v1/admin/tenants/", withAuth(requireAdmin(h.Handle)))

	// Tenant-scoped audit search
	mux.Handle("/api/v1/admin/audit", withAuth(requireAdmin(h.Handle)))
	mux.Handle("/api/v1/admin/audit/", withAuth(requireAdmin(h.Handle)))
}
