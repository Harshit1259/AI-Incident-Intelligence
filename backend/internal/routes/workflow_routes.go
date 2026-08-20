package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerWorkflowRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	withOps func(http.HandlerFunc) http.Handler,
	h *handlers.WorkflowHandler,
) {
	if h == nil {
		return
	}

	// ── Templates ─────────────────────────────────────────────────────────────
	mux.Handle("/api/v1/workflow/templates", withAuth(h.HandleTemplates))
	mux.Handle("/api/v1/workflow/templates/", withAuth(h.HandleTemplates))

	// ── Teams outgoing webhook (no JWT — Teams signs with HMAC, checked in handler) ──
	mux.Handle("/api/v1/teams/webhook", withOps(h.HandleTeamsWebhook))
}
