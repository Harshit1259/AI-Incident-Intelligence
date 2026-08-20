package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerMarketplaceRoutes wires the Integration Hub endpoints.
// All routes require JWT auth (operator role or higher).
func registerMarketplaceRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.MarketplaceHandler,
) {
	// Catalog list
	mux.Handle("/api/v1/marketplace", withAuth(h.Handle))
	// All per-integration sub-routes: detail, sample, enable, disable, test, test-alert
	mux.Handle("/api/v1/marketplace/", withAuth(h.Handle))
}
