package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type IncidentHandler struct {
	incidentService *services.IncidentService
}

func NewIncidentHandler(incidentService *services.IncidentService) *IncidentHandler {
	return &IncidentHandler{incidentService: incidentService}
}

func (incidentHandler *IncidentHandler) ListIncidents(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()

	from, err := services.ParseOptionalTime(query.Get("from"))
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			// Changed 'w' to 'responseWriter'
			api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
			return
		}

		// Changed 'w' to 'responseWriter'
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to fetch incidents")
		return
	}

	to, err := services.ParseOptionalTime(query.Get("to"))
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			// Changed 'w' to 'responseWriter'
			api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
			return
		}

		// Changed 'w' to 'responseWriter'
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to fetch incidents")
		return
	}

	filter := models.IncidentListFilter{
		Status:    query.Get("status"),
		Severity:  query.Get("severity"),
		Service:   query.Get("service"),
		Search:    query.Get("search"),
		From:      from,
		To:        to,
		Page:      parseIntWithDefault(query.Get("page"), 1),
		PageSize:  parseIntWithDefault(query.Get("page_size"), 20),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
	}

	response, err := incidentHandler.incidentService.ListIncidents(filter)
	if err != nil {
		if isBadRequestError(err) {
			api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
			return
		}

		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to fetch incidents")
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, response)
}

func (incidentHandler *IncidentHandler) GetIncidentDetail(responseWriter http.ResponseWriter, request *http.Request) {
	incidentID, _, ok := parseIncidentActionPath(request.URL.Path)
	if !ok {
		api.WriteError(responseWriter, http.StatusBadRequest, "incident id is required")
		return
	}

	detail, found := incidentHandler.incidentService.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(responseWriter, http.StatusNotFound, "incident not found")
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, detail)
}

func (incidentHandler *IncidentHandler) UpdateIncidentStatus(responseWriter http.ResponseWriter, request *http.Request) {
	incidentID, action, ok := parseIncidentActionPath(request.URL.Path)
	if !ok || action == "" {
		api.WriteError(responseWriter, http.StatusBadRequest, "invalid request")
		return
	}

	incident, err := incidentHandler.incidentService.UpdateIncidentStatus(incidentID, action)
	if err != nil {
		if store.IsNotFoundError(err) {
			api.WriteError(responseWriter, http.StatusNotFound, "incident not found")
			return
		}

		if isBadRequestError(err) {
			api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
			return
		}

		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to update incident")
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, incident)
}

func parseIncidentActionPath(path string) (string, string, bool) {
	trimmedPath := strings.Trim(path, "/")
	parts := strings.Split(trimmedPath, "/")

	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "incidents" {
		return "", "", false
	}

	incidentID := strings.TrimSpace(parts[3])
	if incidentID == "" {
		return "", "", false
	}

	if len(parts) == 4 {
		return incidentID, "", true
	}

	return incidentID, strings.TrimSpace(parts[4]), true
}

func parseIntWithDefault(value string, fallback int) int {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return fallback
	}

	parsedValue, err := strconv.Atoi(trimmedValue)
	if err != nil {
		return fallback
	}

	return parsedValue
}

func isBadRequestError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	badRequestMessages := []string{
		"invalid status value",
		"invalid severity value",
		"invalid sort_by value",
		"invalid sort_order value",
		"search value too long",
		"service value too long",
		"from must be earlier than to",
		"invalid time value",
		"invalid action",
		"incident id is required",
	}

	for _, candidate := range badRequestMessages {
		if strings.Contains(message, candidate) {
			return true
		}
	}

	return false
}
