package handlers

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type ExplainHandler struct {
	incidentService *services.IncidentService
	explainService  *services.ExplainService
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

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"explanation": explanation,
		"detail":      enrichedDetail,
	})
}
