package handlers

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

type ExplainHandler struct {
	incidentService *services.IncidentService
	explainService  *services.ExplainService
	remediation     *services.RemediationOrchestrator
}

func NewExplainHandler(
	incidentService *services.IncidentService,
	explainService *services.ExplainService,
) *ExplainHandler {
	return &ExplainHandler{
		incidentService: incidentService,
		explainService:  explainService,
	}
}

func (handler *ExplainHandler) Explain(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/incidents/explain/"
	incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))

	if incidentID == "" || incidentID == r.URL.Path || strings.Contains(incidentID, "/") {
		api.WriteError(w, http.StatusBadRequest, "invalid incident id")
		return
	}

	detail, found := handler.incidentService.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	// Use LLM-powered explain (falls back to template if LLM not configured)
	enrichedDetail, explanation := handler.explainService.Explain(detail)

	// Track AI feature adoption for Customer Health Score (SaaS F4).
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		audit.Log(claims.TenantID, claims.UserID, "incident.explain", "incident", incidentID, nil)
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"explanation": explanation,
		"detail":      enrichedDetail,
		// Provenance is surfaced at the top level as well as inside detail so a
		// client can decide how to render without walking the whole payload.
		// When has_causal_claim is false the UI must not show a root-cause
		// heading or a confidence percentage — there is nothing behind them.
		"provenance": enrichedDetail.Provenance,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Analyze — the explicit, attributable AI path
// ─────────────────────────────────────────────────────────────────────────────

// SetRemediationOrchestrator wires the orchestrator so a completed analysis can
// re-evaluate auto-remediation with a real RCA confidence in hand.
func (handler *ExplainHandler) SetRemediationOrchestrator(o *services.RemediationOrchestrator) {
	handler.remediation = o
}

// Analyze handles POST /api/v1/incidents/{id}/analyze.
//
// This is the only route that spends an LLM call on an incident. It is
// deliberately a POST rather than a GET: it costs money, it is not idempotent
// in any useful sense, and it produces a causal claim that is attributed to the
// caller. Opening an incident must never trigger it.
//
// Response mirrors Explain so the client can swap one for the other, but the
// provenance block will carry source="llm" plus the model, timestamp and actor.
func (handler *ExplainHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	incidentID := extractIncidentIDForAnalyze(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "invalid incident id")
		return
	}

	detail, found := handler.incidentService.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	// Attribute the analysis to the caller. An RCA is a claim somebody is
	// accountable for, so it carries a name.
	actor := "unknown"
	tenantID := ""
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		actor = claims.UserID
		tenantID = claims.TenantID
	}

	enriched, narrative := handler.explainService.Analyze(detail, actor)

	audit.Log(tenantID, actor, "incident.analyze", "incident", incidentID, map[string]any{
		"analysis_source": enriched.Provenance.Source,
		"model":           enriched.Provenance.Model,
	})

	// A completed analysis is the correct moment to re-evaluate automation:
	// there is now a real confidence in the CAUSE, which is what the
	// auto-execute gate requires. Without this, an analysed incident would
	// still be stuck at human-approval mode forever.
	if handler.remediation != nil && enriched.Provenance.HasCausalClaim {
		go handler.remediation.TriggerRemediation(enriched.Incident, enriched.Provenance.RCAConfidence)
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"explanation": narrative,
		"detail":      enriched,
		"provenance":  enriched.Provenance,
	})
}

// extractIncidentIDForAnalyze parses {id} out of /api/v1/incidents/{id}/analyze.
func extractIncidentIDForAnalyze(path string) string {
	const prefix = "/api/v1/incidents/"
	const suffix = "/analyze"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return strings.TrimSpace(id)
}
