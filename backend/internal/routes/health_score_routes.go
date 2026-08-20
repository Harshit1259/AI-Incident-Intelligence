package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerHealthScoreRoutes wires the Customer Health Score admin endpoints.
// All routes are admin-only — the handler itself enforces the role check via
// middleware.RequireMinRole in the withAuth wrapper.
func registerHealthScoreRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.HealthScoreHandler,
) {
	// All-tenant health dashboard + single-tenant score
	mux.Handle("/api/v1/admin/health-scores", withAuth(h.Handle))
	mux.Handle("/api/v1/admin/health-scores/", withAuth(h.Handle))
}
