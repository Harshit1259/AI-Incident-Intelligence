package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerDomainMemoryRoutes wires the AI domain memory endpoints.
// All routes require auth. Registered as always-active core routes.
func registerDomainMemoryRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.DomainMemoryHandler,
) {
	// Full memory context for an incident
	mux.Handle("/api/v1/memory/context/", withAuth(h.Handle))

	// Structured resolution recording
	mux.Handle("/api/v1/memory/resolve/", withAuth(h.Handle))

	// Remediation patterns
	mux.Handle("/api/v1/memory/remediations", withAuth(h.Handle))
	mux.Handle("/api/v1/memory/remediations/", withAuth(h.Handle))

	// Deploy signatures
	mux.Handle("/api/v1/memory/deploy-signatures", withAuth(h.Handle))
	mux.Handle("/api/v1/memory/deploy-signatures/", withAuth(h.Handle))

	// Runbook preferences
	mux.Handle("/api/v1/memory/runbook-preferences", withAuth(h.Handle))
	mux.Handle("/api/v1/memory/runbook-preferences/", withAuth(h.Handle))
}
