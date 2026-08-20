package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// PolicyHandler handles all automation policy engine endpoints.
type PolicyHandler struct {
	service *services.PolicyService
}

// NewPolicyHandler creates a new PolicyHandler.
func NewPolicyHandler(s *services.PolicyService) *PolicyHandler {
	return &PolicyHandler{service: s}
}

// HandlePolicies dispatches GET /api/v1/policies and POST /api/v1/policies.
func (h *PolicyHandler) HandlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listPolicies(w, r)
	case http.MethodPost:
		h.createPolicy(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandlePolicyByID dispatches GET/PUT/DELETE /api/v1/policies/{id}.
func (h *PolicyHandler) HandlePolicyByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getPolicy(w, r)
	case http.MethodPut:
		h.updatePolicy(w, r)
	case http.MethodDelete:
		h.deletePolicy(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleEvaluate handles POST /api/v1/policies/evaluate.
// Runs the full policy engine for the given context and writes a trail entry.
func (h *PolicyHandler) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var ctx models.PolicyEvalContext
	if err := json.NewDecoder(r.Body).Decode(&ctx); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if ctx.TenantID == "" {
		ctx.TenantID = middleware.TenantFromRequest(r)
	}
	if ctx.Actor == "" {
		ctx.Actor = getActor(r)
	}

	decision := h.service.Evaluate(ctx)
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"decision": decision,
		"context":  ctx,
	})
}

// HandleSimulate handles POST /api/v1/policies/simulate.
// Returns what the policy decision would be without writing a trail entry or
// mutating any circuit-breaker state.
func (h *PolicyHandler) HandleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var ctx models.PolicyEvalContext
	if err := json.NewDecoder(r.Body).Decode(&ctx); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if ctx.TenantID == "" {
		ctx.TenantID = middleware.TenantFromRequest(r)
	}

	decision := h.service.Simulate(ctx)
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"decision":  decision,
		"simulated": true,
		"context":   ctx,
	})
}

// HandleTrail handles GET /api/v1/policies/{id}/trail.
func (h *PolicyHandler) HandleTrail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	policyID := extractBetween(r.URL.Path, "policies", "trail")
	if policyID == "" {
		api.WriteError(w, http.StatusBadRequest, "policy id required")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	entries, err := h.service.GetTrailByPolicy(policyID, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to retrieve trail")
		return
	}
	if entries == nil {
		entries = []models.PolicyExecutionTrail{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"trail":     entries,
		"policy_id": policyID,
		"count":     len(entries),
	})
}

// HandleCircuitBreakers handles GET /api/v1/policies/circuit-breakers.
func (h *PolicyHandler) HandleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	states, err := h.service.GetCircuitBreakerStates(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to retrieve circuit breaker states")
		return
	}
	if states == nil {
		states = []models.CircuitBreakerState{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"circuit_breakers": states,
		"count":            len(states),
	})
}

// HandleResetCircuitBreaker handles POST /api/v1/policies/{id}/circuit-breaker/reset.
func (h *PolicyHandler) HandleResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	policyID := extractBetween(r.URL.Path, "policies", "circuit-breaker")
	if policyID == "" {
		api.WriteError(w, http.StatusBadRequest, "policy id required")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	if err := h.service.ResetCircuitBreaker(policyID, tenantID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to reset circuit breaker")
		return
	}
	actor := getActor(r)
	audit.Log(tenantID, actor, "policy.circuit_breaker.reset", "policy", policyID, nil)
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "reset", "policy_id": policyID})
}

// HandleTenantTrail handles GET /api/v1/policies/trail.
func (h *PolicyHandler) HandleTenantTrail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := h.service.GetTrailByTenant(tenantID, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to retrieve trail")
		return
	}
	if entries == nil {
		entries = []models.PolicyExecutionTrail{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"trail": entries,
		"count": len(entries),
	})
}

// ── private handlers ──────────────────────────────────────────────────────────

func (h *PolicyHandler) listPolicies(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	limit, offset := parsePagination(r, 50, 1000)
	policies, err := h.service.GetPolicies(tenantID, limit, offset)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}
	if policies == nil {
		policies = []models.ExecutionPolicy{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
		"count":    len(policies),
	})
}

func (h *PolicyHandler) createPolicy(w http.ResponseWriter, r *http.Request) {
	var p models.ExecutionPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if p.Name == "" {
		api.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	p.TenantID = tenantID

	if err := h.service.CreatePolicy(p); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to create policy")
		return
	}

	actor := getActor(r)
	audit.Log(tenantID, actor, "policy.created", "policy", p.ID, map[string]interface{}{
		"name":           p.Name,
		"service":        p.Service,
		"environment":    p.Environment,
		"severity":       p.Severity,
		"execution_mode": p.ExecutionMode,
		"priority":       p.Priority,
		"simulation":     p.SimulationMode,
		"dry_run":        p.DryRunMode,
	})

	api.WriteJSON(w, http.StatusCreated, p)
}

func (h *PolicyHandler) getPolicy(w http.ResponseWriter, r *http.Request) {
	id := policyIDFromPath(r.URL.Path)
	p, err := h.service.GetPolicyByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to retrieve policy")
		return
	}
	if p == nil {
		api.WriteError(w, http.StatusNotFound, "policy not found")
		return
	}
	api.WriteJSON(w, http.StatusOK, p)
}

func (h *PolicyHandler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	id := policyIDFromPath(r.URL.Path)

	existing, err := h.service.GetPolicyByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to retrieve policy")
		return
	}
	if existing == nil {
		api.WriteError(w, http.StatusNotFound, "policy not found")
		return
	}

	var p models.ExecutionPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p.ID = id
	p.TenantID = existing.TenantID
	p.CreatedAt = existing.CreatedAt

	if err := h.service.UpdatePolicy(p); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	actor := getActor(r)
	audit.Log(tenantID, actor, "policy.updated", "policy", id, map[string]interface{}{
		"name":        p.Name,
		"enabled":     p.Enabled,
		"simulation":  p.SimulationMode,
		"dry_run":     p.DryRunMode,
	})

	updated, _ := h.service.GetPolicyByID(id)
	api.WriteJSON(w, http.StatusOK, updated)
}

func (h *PolicyHandler) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id := policyIDFromPath(r.URL.Path)

	if err := h.service.DeletePolicy(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to delete policy")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	actor := getActor(r)
	audit.Log(tenantID, actor, "policy.deleted", "policy", id, nil)

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// ── path helpers ──────────────────────────────────────────────────────────────

// policyIDFromPath extracts the last non-empty segment from a URL path.
func policyIDFromPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// extractBetween extracts the segment between `before` and `after` in the path.
// e.g. /api/v1/policies/pol-123/trail → extractBetween(path, "policies", "trail") = "pol-123"
func extractBetween(path, before, after string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == before && i+2 < len(parts) && parts[i+2] == after {
			return parts[i+1]
		}
	}
	return ""
}

func getActor(r *http.Request) string {
	claims, ok := middleware.ClaimsFromContext(r)
	if ok && claims.UserID != "" {
		return claims.UserID
	}
	return "system"
}
