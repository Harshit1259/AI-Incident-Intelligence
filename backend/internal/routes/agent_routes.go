package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
)

func registerAgentRoutes(
	mux *http.ServeMux,
	withOps func(http.HandlerFunc) http.Handler,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	agentHandler *handlers.AgentHandler,
	enrollmentHandler *handlers.EnrollmentHandler,
	logExplorerHandler *handlers.LogExplorerHandler,
	agentAuth *middleware.AgentAuthenticator,
) {
	// ── Bootstrap (registration) ──────────────────────────────────────────
	// Uses X-Enrollment-Token or legacy X-Agent-Token; no JWT required.
	mux.Handle("/api/v1/agents/register", withOps(agentHandler.Register))

	// ── Steady-state ingest ───────────────────────────────────────────────
	// HMAC-signed per-agent requests; dev fallback via X-Agent-Token.
	mux.Handle("/api/v1/ingest/agent", withOps(agentAuth.Middleware(agentHandler.IngestBatch)))

	// ── Enrollment token management — operator+ ───────────────────────────
	// Must be registered before /api/v1/agents/ so the longer path wins.
	mux.Handle("/api/v1/agents/enrollment-tokens", withAuth(requireOperator(enrollmentHandler.HandleCollection)))
	mux.Handle("/api/v1/agents/enrollment-tokens/", withAuth(requireOperator(enrollmentHandler.HandleItem)))

	// ── Agent management — operator+ (contains host IPs and infra metadata) ─
	mux.Handle("/api/v1/agents", withAuth(requireOperator(agentHandler.ListAgents)))
	mux.Handle("/api/v1/agents/", withAuth(requireOperator(agentHandler.HandleAgentByID)))

	// ── Log explorer — operator+ ──────────────────────────────────────────
	mux.Handle("/api/v1/logs/stats", withAuth(requireOperator(logExplorerHandler.Stats)))
	mux.Handle("/api/v1/logs/stream", withAuth(requireOperator(logExplorerHandler.Stream)))
	mux.Handle("/api/v1/logs", withAuth(requireOperator(logExplorerHandler.Query)))
}
