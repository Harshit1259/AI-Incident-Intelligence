package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// Note: /api/v1/config/schema is registered as a public (no-auth) route in
// RegisterRoutes directly, alongside /api/v1/health. This is intentional —
// operators need it before they have credentials.

func registerEnterpriseRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireAdmin func(http.HandlerFunc) http.HandlerFunc,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	policyHandler *handlers.PolicyHandler,
	configHandler *handlers.ConfigHandler,
	auditHandler *handlers.AuditHandler,
) {
	// ── Policy CRUD ───────────────────────────────────────────────────────────
	// GET  /api/v1/policies          — list all policies (operator+)
	// POST /api/v1/policies          — create policy (admin)
	mux.Handle("/api/v1/policies", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			requireOperator(policyHandler.HandlePolicies)(w, r)
		case http.MethodPost:
			requireAdmin(policyHandler.HandlePolicies)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// ── Policy engine — special action routes (must register before the /{id} wildcard) ──
	// POST /api/v1/policies/evaluate  — run evaluation and record trail (operator+)
	// POST /api/v1/policies/simulate  — simulate without side-effects (operator+)
	// GET  /api/v1/policies/trail     — tenant-wide execution trail (operator+)
	// GET  /api/v1/policies/circuit-breakers — all circuit-breaker states (operator+)
	mux.Handle("/api/v1/policies/evaluate", withAuth(requireOperator(policyHandler.HandleEvaluate)))
	mux.Handle("/api/v1/policies/simulate", withAuth(requireOperator(policyHandler.HandleSimulate)))
	mux.Handle("/api/v1/policies/trail", withAuth(requireOperator(policyHandler.HandleTenantTrail)))
	mux.Handle("/api/v1/policies/circuit-breakers", withAuth(requireOperator(policyHandler.HandleCircuitBreakers)))

	// ── Per-policy sub-routes ─────────────────────────────────────────────────
	// GET  /api/v1/policies/{id}/trail               — policy execution trail (operator+)
	// POST /api/v1/policies/{id}/circuit-breaker/reset — reset circuit breaker (admin)
	mux.Handle("/api/v1/policies/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case hasPathSuffix(path, "/trail"):
			requireOperator(policyHandler.HandleTrail)(w, r)
		case hasPathSuffix(path, "/circuit-breaker/reset"):
			requireAdmin(policyHandler.HandleResetCircuitBreaker)(w, r)
		default:
			// GET /{id}, PUT /{id}, DELETE /{id}
			switch r.Method {
			case http.MethodGet:
				requireOperator(policyHandler.HandlePolicyByID)(w, r)
			case http.MethodPut, http.MethodDelete:
				requireAdmin(policyHandler.HandlePolicyByID)(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}))

	// ── Config ────────────────────────────────────────────────────────────────
	mux.Handle("/api/v1/config", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			requireOperator(configHandler.HandleConfig)(w, r)
		case http.MethodPut:
			requireAdmin(configHandler.HandleConfig)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// ── Audit log ─────────────────────────────────────────────────────────────
	mux.Handle("/api/v1/audit", withAuth(requireOperator(auditHandler.HandleAudit)))
}

// hasPathSuffix checks if a URL path ends with the given suffix segment.
func hasPathSuffix(path, suffix string) bool {
	if len(suffix) == 0 {
		return false
	}
	// Trim trailing slash from path for comparison.
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
