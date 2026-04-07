package handlers

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type ExplainHandler struct {
	incidentService *services.IncidentService
}

func NewExplainHandler(incidentService *services.IncidentService) *ExplainHandler {
	return &ExplainHandler{incidentService: incidentService}
}

func (handler *ExplainHandler) Explain(w http.ResponseWriter, r *http.Request) {
	// Expected path:
	// /api/v1/incidents/explain/{incident_id}

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

	explanation := services.BuildExplanation(detail)

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"explanation": explanation,
	})
}
