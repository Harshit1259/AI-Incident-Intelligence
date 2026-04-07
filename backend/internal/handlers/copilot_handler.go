package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type CopilotHandler struct {
	incidentService *services.IncidentService
	copilotService  *services.CopilotService
}

func NewCopilotHandler(
	incidentService *services.IncidentService,
	copilotService *services.CopilotService,
) *CopilotHandler {
	return &CopilotHandler{
		incidentService: incidentService,
		copilotService:  copilotService,
	}
}

func (handler *CopilotHandler) Ask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	const prefix = "/api/v1/incidents/copilot/"
	incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))

	if incidentID == "" || incidentID == r.URL.Path || strings.Contains(incidentID, "/") {
		api.WriteError(w, http.StatusBadRequest, "invalid incident id")
		return
	}

	var request models.CopilotRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(request.Question) == "" {
		api.WriteError(w, http.StatusBadRequest, "question is required")
		return
	}

	detail, found := handler.incidentService.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	answer := handler.copilotService.Answer(detail, request.Question)
	api.WriteJSON(w, http.StatusOK, answer)
}
